#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd "$script_dir/.." && pwd)"
go_command="${GO_COMMAND:-go}"

fail() {
  printf 'Repository check failed: %s\n' "$1" >&2
  exit 1
}

[[ ! -e "$repository_root/go.mod" ]] || fail "the repository root must not contain go.mod"
[[ -f "$repository_root/go.work" ]] || fail "go.work is missing"
[[ -f "$repository_root/LICENSE" ]] || fail "LICENSE is missing"
[[ ! -L "$repository_root/LICENSE" ]] || fail "LICENSE must be a regular file"
[[ -x "$repository_root/scripts/check-module-zip.sh" ]] ||
  fail "scripts/check-module-zip.sh must be executable"
[[ -x "$repository_root/scripts/check-module-zip_test.sh" ]] ||
  fail "scripts/check-module-zip_test.sh must be executable"

expected_license_sha256="c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4"
if command -v sha256sum >/dev/null 2>&1; then
  license_sha256="$(sha256sum "$repository_root/LICENSE" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  license_sha256="$(shasum -a 256 "$repository_root/LICENSE" | awk '{print $1}')"
else
  fail "sha256sum or shasum is required"
fi
[[ "$license_sha256" == "$expected_license_sha256" ]] || fail "LICENSE is not the expected complete Apache-2.0 text"

module_directories=()
while IFS= read -r module_file; do
  relative_file="${module_file#"$repository_root"/}"
  module_directories+=("./${relative_file%/go.mod}")
done < <(
  find "$repository_root" \
    -path "$repository_root/.git" -prune -o \
    -path "$repository_root/_ci" -prune -o \
    -mindepth 2 -maxdepth 2 -type f -name go.mod -print |
    LC_ALL=C sort
)

(( ${#module_directories[@]} > 0 )) || fail "no Adapter modules were found"

workspace_directories=()
while IFS= read -r workspace_directory; do
  [[ -n "$workspace_directory" ]] && workspace_directories+=("$workspace_directory")
done < <(
  "$go_command" work edit -json "$repository_root/go.work" |
    sed -n 's/^[[:space:]]*"DiskPath": "\([^"]*\)".*$/\1/p' |
    LC_ALL=C sort
)

module_list="$(printf '%s\n' "${module_directories[@]}" | LC_ALL=C sort)"
workspace_list="$(printf '%s\n' "${workspace_directories[@]}" | LC_ALL=C sort)"
[[ "$workspace_list" == "$module_list" ]] || fail "go.work must contain every and only existing Adapter module"

for module_directory in "${module_directories[@]}"; do
  module_file="$repository_root/${module_directory#./}/go.mod"
  expected_module_path="github.com/imbrooklyn/weave-adapters/${module_directory#./}"
  module_path="$(
    "$go_command" mod edit -json "$module_file" |
      sed -n 's/^[[:space:]]*"Path": "\([^"]*\)".*$/\1/p' |
      head -n 1
  )"
  [[ "$module_path" == "$expected_module_path" ]] || fail "$module_file has unexpected module path $module_path"

  go_version="$(
    "$go_command" mod edit -json "$module_file" |
      sed -n 's/^[[:space:]]*"Go": "\([^"]*\)".*$/\1/p'
  )"
  [[ "$go_version" == "1.27" || "$go_version" == "1.27.0" ]] || fail "$module_file must declare Go 1.27"

  if grep -Eq '^[[:space:]]*replace([[:space:](]|$)' "$module_file"; then
    fail "$module_file must not contain replace directives"
  fi
done

printf 'Repository metadata checks passed for %d module(s).\n' "${#module_directories[@]}"
