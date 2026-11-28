#!/bin/sh
# Build and install meatproxy, with an `mp` alias if none exists.
#
#   ./install.sh                       # install to a default PATH directory
#   MEATPROXY_INSTALL_DIR=~/bin ./install.sh
set -eu

cd "$(dirname "$0")"

if ! command -v go >/dev/null 2>&1; then
	echo "error: go is required (Go 1.24+)" >&2
	exit 1
fi

on_path() {
	printf '%s' "$PATH" | tr ':' '\n' | grep -Fxq "$1"
}

if [ -n "${MEATPROXY_INSTALL_DIR:-}" ]; then
	destdir="$MEATPROXY_INSTALL_DIR"
	mkdir -p "$destdir"
else
	# Prefer a standard system PATH directory, then any writable PATH dir.
	destdir=""
	for candidate in /usr/local/bin /opt/homebrew/bin "$HOME/.local/bin" "$HOME/go/bin"; do
		if [ -d "$candidate" ] && [ -w "$candidate" ] && on_path "$candidate"; then
			destdir="$candidate"
			break
		fi
	done
	if [ -z "$destdir" ]; then
		for candidate in $(printf '%s' "$PATH" | tr ':' '\n'); do
			if [ -d "$candidate" ] && [ -w "$candidate" ]; then
				destdir="$candidate"
				break
			fi
		done
	fi
	if [ -z "$destdir" ]; then
		destdir="$HOME/go/bin"
	fi
	mkdir -p "$destdir"
fi

bin="$destdir/meatproxy"
go build -o "$bin" .
echo "installed: $bin"

# Refresh the standard Go bin too, so old installs on PATH get the new binary.
gobin="$HOME/go/bin/meatproxy"
if [ "$gobin" != "$bin" ] && [ -d "$(dirname "$gobin")" ]; then
	cp "$bin" "$gobin"
	echo "refreshed: $gobin"
fi
if ! on_path "$destdir"; then
	echo "add $destdir to your PATH to use 'meatproxy' directly"
fi

# Add an `mp` alias to the shell's rc file, unless one already exists.
rc=""
case "$(basename "${SHELL:-/bin/sh}")" in
	zsh) rc="$HOME/.zshrc" ;;
	bash) rc="$HOME/.bashrc" ;;
esac
if [ -n "$rc" ] && ! grep -Eq '^[[:space:]]*alias[[:space:]]+mp=' \
	"$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" 2>/dev/null; then
	touch "$rc"
	printf '\n# meatproxy\nalias mp='"'"'meatproxy'"'"'\n' >> "$rc"
	echo "added alias mp=meatproxy to $rc (restart your shell or run: source $rc)"
fi
