#!/bin/sh
set -eu
umask 077
version=${MODEX_VERSION:-}
prefix=${HOME:?}/.local/bin
while [ "$#" -gt 0 ]; do
    case "$1" in
        --version) [ "$#" -ge 2 ] || exit 2; version=$2; shift 2 ;;
        --prefix) [ "$#" -ge 2 ] || exit 2; prefix=$2; shift 2 ;;
        *) echo 'Usage: install.sh --version v0.1.0-alpha.1 [--prefix /absolute/bin]' >&2; exit 2 ;;
    esac
done
case "$version" in v[0-9]*) ;; *) echo 'An explicit release version (v...) is required.' >&2; exit 2 ;; esac
case "$version" in *[!a-zA-Z0-9.+-]*) echo 'Invalid version' >&2; exit 2 ;; esac
case "$prefix" in /*) ;; *) echo 'Prefix must be absolute' >&2; exit 2 ;; esac
case "$(uname -s)" in Linux) platform=linux ;; Darwin) platform=darwin ;; *) echo 'Use Windows SSH client ZIP or install the host inside WSL2.' >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo 'Unsupported CPU' >&2; exit 1 ;; esac
for tool in curl tar mktemp awk cp chmod mv; do command -v "$tool" >/dev/null || { echo "Missing $tool" >&2; exit 1; }; done
if command -v sha256sum >/dev/null; then hash=sha256sum
elif command -v shasum >/dev/null; then hash=shasum
else echo 'sha256sum or shasum is required' >&2; exit 1; fi
work=$(mktemp -d "${TMPDIR:-/tmp}/modex-install.XXXXXXXX")
staged=
cleanup() { rm -rf "$work"; if [ -n "$staged" ]; then rm -rf "$staged"; fi; }
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
asset="modex_${version#v}_${platform}_${arch}.tar.gz"
base="https://github.com/warmdev17/modex/releases/download/$version"
curl --fail --location --proto '=https' --tlsv1.2 "$base/$asset" -o "$work/archive.tar.gz"
curl --fail --location --proto '=https' --tlsv1.2 "$base/checksums.txt" -o "$work/checksums.txt"
expected=$(awk -v name="$asset" '$2 == name {print $1; count++} END {if (count != 1) exit 1}' "$work/checksums.txt")
[ "${#expected}" = 64 ] || { echo 'Invalid checksum entry' >&2; exit 1; }
case "$expected" in *[!0-9a-fA-F]*) echo 'Invalid checksum' >&2; exit 1 ;; esac
if [ "$hash" = sha256sum ]; then actual=$(sha256sum "$work/archive.tar.gz" | awk '{print $1}')
else actual=$(shasum -a 256 "$work/archive.tar.gz" | awk '{print $1}'); fi
[ "$actual" = "$expected" ] || { echo 'Checksum mismatch; nothing installed' >&2; exit 1; }
for file in modex modex-host; do
    tar -xOzf "$work/archive.tar.gz" "$file" > "$work/$file"
    [ -s "$work/$file" ] || { echo "Invalid binary $file" >&2; exit 1; }
    [ ! -d "$prefix/$file" ] || { echo "Destination is a directory: $file" >&2; exit 1; }
done
mkdir -p "$prefix"
staged=$(mktemp -d "$prefix/.modex-install.XXXXXXXX")
for file in modex modex-host; do cp "$work/$file" "$staged/$file"; chmod 755 "$staged/$file"; done
for file in modex modex-host; do mv -f "$staged/$file" "$prefix/$file"; done
printf 'Installed %s in %s. Existing services were not restarted.\n' "$version" "$prefix"
case ":$PATH:" in *":$prefix:"*) ;; *) printf 'Add this directory to PATH: %s\n' "$prefix" ;; esac
printf 'Configure Codex and SSH, then: modex service install --workspace /absolute/project\n'
