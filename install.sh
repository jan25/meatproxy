#!/bin/sh
# Build and install meatproxy on PATH.
#
#   ./install.sh                       # install to $HOME/go/bin (or first writable PATH dir)
#   MEATPROXY_INSTALL_DIR=~/bin ./install.sh
set -eu

cd "$(dirname "$0")"

if ! command -v go >/dev/null 2>&1; then
	echo "error: go is required (Go 1.24+)" >&2
	exit 1
fi

if [ -n "${MEATPROXY_INSTALL_DIR:-}" ]; then
	destdir="$MEATPROXY_INSTALL_DIR"
	mkdir -p "$destdir"
else
	destdir="$HOME/go/bin"
	mkdir -p "$destdir"
	# Prefer a writable directory already on PATH so `meatproxy` works immediately.
	if ! printf '%s' "$PATH" | tr ':' '\n' | grep -Fxq "$destdir"; then
		for candidate in $(printf '%s' "$PATH" | tr ':' '\n'); do
			if [ -d "$candidate" ] && [ -w "$candidate" ]; then
				destdir="$candidate"
				break
			fi
		done
	fi
fi

bin="$destdir/meatproxy"
go build -o "$bin" .
echo "installed: $bin"
if ! printf '%s' "$PATH" | tr ':' '\n' | grep -Fxq "$destdir"; then
	echo "add $destdir to your PATH to use 'meatproxy' directly"
fi
