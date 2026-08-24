#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd "$script_dir/.." && pwd)"
go_command="${GO_COMMAND:-go}"
gofmt_command="${GOFMT_COMMAND:-gofmt}"

"$script_dir/check-repository.sh"

unformatted="$(
  find "$repository_root" \
    -path "$repository_root/.git" -prune -o \
    -path "$repository_root/_ci" -prune -o \
    -type f -name '*.go' -print0 |
    xargs -0 "$gofmt_command" -l
)"
if [[ -n "$unformatted" ]]; then
  printf 'Unformatted Go files:\n%s\n' "$unformatted" >&2
  exit 1
fi

while IFS= read -r module_file; do
  module_directory="$(dirname "$module_file")"
  relative_directory="${module_directory#"$repository_root"/}"
  printf 'Testing %s...\n' "$relative_directory"
  (
    cd "$module_directory"
    "$go_command" test ./...
    "$go_command" vet ./...
  )
done < <(
  find "$repository_root" \
    -path "$repository_root/.git" -prune -o \
    -path "$repository_root/_ci" -prune -o \
    -mindepth 2 -maxdepth 2 -type f -name go.mod -print |
    LC_ALL=C sort
)

printf 'All available Adapter module checks passed.\n'
