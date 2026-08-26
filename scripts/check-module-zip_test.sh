#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd "$script_dir/.." && pwd)"
temporary_directory="$(mktemp -d "${TMPDIR:-/tmp}/weave-module-zip-test.XXXXXX")"
trap 'rm -rf "$temporary_directory"' EXIT

coordinate="fake.example/module@v0.0.1"
zip_path="$temporary_directory/module.zip"
license_entry="$coordinate/LICENSE"
touch "$zip_path"

mkdir -p "$temporary_directory/bin"
fake_go="$temporary_directory/bin/go"
fake_unzip="$temporary_directory/bin/unzip"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf '\''{\n  "Zip": "%s"\n}\n'\'' "$FAKE_MODULE_ZIP"' \
  >"$fake_go"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -e' \
  'case "$1" in' \
  '  -Z1)' \
  '    printf '\''%s\n'\'' "$FAKE_LICENSE_ENTRY"' \
  '    index=0' \
  '    while (( index < 8192 )); do' \
  '      printf '\''fake.example/module@v0.0.1/files/%08d-padding-entry\n'\'' "$index"' \
  '      ((index += 1))' \
  '    done' \
  '    ;;' \
  '  -p)' \
  '    command cat "$FAKE_LICENSE_SOURCE"' \
  '    ;;' \
  '  *)' \
  '    exit 2' \
  '    ;;' \
  'esac' \
  >"$fake_unzip"
chmod +x "$fake_go" "$fake_unzip"

PATH="$temporary_directory/bin:$PATH" \
GO_COMMAND="$fake_go" \
FAKE_MODULE_ZIP="$zip_path" \
FAKE_LICENSE_ENTRY="$license_entry" \
FAKE_LICENSE_SOURCE="$repository_root/LICENSE" \
  "$script_dir/check-module-zip.sh" "$coordinate"

printf 'Module zip regression check passed with LICENSE before trailing entries.\n'
