#!/bin/sh
# Build both commands before replacing any installed executable.
set -eu

modex_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
modex_prefix=${PREFIX:-"$HOME/.local"}
modex_host=${HOST_BINARY:-/tmp/modex-host}
case "$modex_prefix:$modex_host" in *'
'*) printf '%s\n' 'Build paths must be single-line.' >&2; exit 1 ;; esac
case "$modex_prefix" in /*) ;; *) printf '%s\n' 'PREFIX must be absolute.' >&2; exit 1 ;; esac
case "$modex_host" in /*) ;; *) printf '%s\n' 'HOST_BINARY must be absolute.' >&2; exit 1 ;; esac
modex_bin=$modex_prefix/bin
modex_stage=
modex_build=$(mktemp -d "${TMPDIR:-/tmp}/modex-build.XXXXXX")
trap 'rm -rf "$modex_build"; if [ -n "$modex_stage" ]; then rm -f "$modex_stage"; fi' EXIT
trap 'exit 1' HUP INT TERM

cd "$modex_root"
go build -o "$modex_build/modex-host" ./cmd/modex-host
go build -o "$modex_build/modex" ./cmd/modex

modex_install() {
    if [ -d "$2" ]; then printf '%s\n' 'Executable destination is a directory.' >&2; exit 1; fi
    mkdir -p "$(dirname -- "$2")"
    modex_stage=$(mktemp "$2.XXXXXX")
    cp "$1" "$modex_stage"
    chmod 755 "$modex_stage"
    mv -f "$modex_stage" "$2"
    modex_stage=
}
modex_install "$modex_build/modex-host" "$modex_bin/modex-host"
modex_install "$modex_build/modex" "$modex_bin/modex"
if [ "$modex_host" != "$modex_bin/modex-host" ]; then
    modex_install "$modex_build/modex-host" "$modex_host"
fi

modex_quote() {
    printf "'"
    printf '%s' "$1" | sed "s/'/'\\\\''/g"
    printf "'"
}
modex_append_path() {
    mkdir -p "$(dirname -- "$1")"
    if ! [ -f "$1" ] || ! grep -Fqx -- "$modex_line" "$1"; then
        printf '\n# Modex local commands\n%s\n' "$modex_line" >> "$1"
    fi
}
case ":${PATH:-}:" in
    *":$modex_bin:"*) ;;
    *)
        modex_shell=${SHELL:-sh}
        case "${modex_shell##*/}" in
            fish)
                modex_line="fish_add_path --path $(modex_quote "$modex_bin")"
                modex_append_path "${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/modex-path.fish"
                ;;
            zsh)
                modex_line="export PATH=$(modex_quote "$modex_bin"):\$PATH"
                modex_append_path "${ZDOTDIR:-$HOME}/.zshrc"
                modex_append_path "${ZDOTDIR:-$HOME}/.zprofile"
                ;;
            *)
                modex_line="export PATH=$(modex_quote "$modex_bin"):\$PATH"
                modex_append_path "$HOME/.profile"
                if [ "${modex_shell##*/}" = bash ]; then modex_append_path "$HOME/.bashrc"; fi
                ;;
        esac
        printf '%s\n' 'PATH configured. Open a new terminal to use modex.'
        ;;
esac
printf 'Installed %s and %s\n' "$modex_bin/modex" "$modex_bin/modex-host"
printf 'Phone host binary: %s\n' "$modex_host"
printf '%s\n' 'Running supervisors were not restarted. Use modex to open the session manager.'
