#!/usr/bin/env bash

set -euo pipefail

go_command="${GO_COMMAND:-go}"
coordinate="${1:-}"

fail() {
  printf 'Module zip check failed: %s\n' "$1" >&2
  exit 1
}

[[ "$coordinate" == *@v* ]] || fail "usage: $0 module-path@version"
command -v unzip >/dev/null 2>&1 || fail "unzip is required"

if ! download_json="$("$go_command" mod download -json "$coordinate" 2>&1)"; then
  fail "go mod download failed: $download_json"
fi
zip_path="$(
  printf '%s\n' "$download_json" |
    sed -n 's/^[[:space:]]*"Zip": "\([^"]*\)",*$/\1/p' |
    head -n 1
)"
[[ -n "$zip_path" && -f "$zip_path" ]] || fail "go mod download did not return a module zip"

module_path="${coordinate%@*}"
version="${coordinate##*@}"
license_entry="$module_path@$version/LICENSE"
if ! unzip -Z1 "$zip_path" | grep -Fqx "$license_entry"; then
  fail "$license_entry is missing"
fi

expected_license_sha256="c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4"
if command -v sha256sum >/dev/null 2>&1; then
  license_sha256="$(unzip -p "$zip_path" "$license_entry" | sha256sum | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  license_sha256="$(unzip -p "$zip_path" "$license_entry" | shasum -a 256 | awk '{print $1}')"
else
  fail "sha256sum or shasum is required"
fi
[[ "$license_sha256" == "$expected_license_sha256" ]] ||
  fail "$license_entry is not the expected complete Apache-2.0 text"

printf 'Module zip contains the expected Apache-2.0 LICENSE: %s\n' "$coordinate"
