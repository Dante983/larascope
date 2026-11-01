#!/bin/sh
set -eu

REPO=Dante983/larascope
DEFAULT_BASE_URL="https://github.com/${REPO}/releases"

valid_version() {
	printf '%s\n' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$'
}

fetch() {
	url=$1
	destination=$2
	if command -v curl >/dev/null 2>&1; then
		case "$url" in
			http://127.0.0.1|http://127.0.0.1:*|http://127.0.0.1/*) curl -fsSL "$url" -o "$destination" ;;
			*) curl -fsSL --proto '=https' --tlsv1.2 "$url" -o "$destination" ;;
		esac
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$destination" "$url"
	else
		echo "error: curl or wget is required" >&2
		exit 1
	fi
}

main() {
	INSTALL_DIR=${LARASCOPE_INSTALL_DIR:-"$HOME/.local/bin"}
	BASE_URL=${LARASCOPE_BASE_URL:-"$DEFAULT_BASE_URL"}
	case "$BASE_URL" in
		*@*) echo "error: LARASCOPE_BASE_URL must not contain credentials" >&2; exit 1 ;;
		https://*|http://127.0.0.1|http://127.0.0.1:*|http://127.0.0.1/*) ;;
		*) echo "error: LARASCOPE_BASE_URL must use HTTPS" >&2; exit 1 ;;
	esac

	if [ -n "${LARASCOPE_VERSION:-}" ]; then
		tag=$LARASCOPE_VERSION
		if ! valid_version "$tag"; then
			echo "error: invalid LARASCOPE_VERSION: $tag" >&2
			exit 1
		fi
	else
		if ! command -v curl >/dev/null 2>&1; then
			echo "error: curl is required to resolve the latest release; set LARASCOPE_VERSION" >&2
			exit 1
		fi
		case "$BASE_URL" in
			http://127.0.0.1|http://127.0.0.1:*|http://127.0.0.1/*)
				if ! url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE_URL/latest"); then
					echo "error: unable to find the latest Larascope release" >&2
					exit 1
				fi
				;;
			*)
				if ! url=$(curl -fsSLI --proto '=https' --tlsv1.2 -o /dev/null -w '%{url_effective}' "$BASE_URL/latest"); then
					echo "error: unable to find the latest Larascope release" >&2
					exit 1
				fi
				;;
		esac
		tag=${url##*/tag/}
		if [ -z "$tag" ] || ! valid_version "$tag"; then
			echo "error: latest Larascope release is unavailable or has an invalid tag" >&2
			exit 1
		fi
	fi

	case $(uname -s) in
		Linux) os=linux ;;
		Darwin) os=darwin ;;
		*) echo "error: unsupported OS (Windows: use install.ps1)" >&2; exit 1 ;;
	esac
	case $(uname -m) in
		x86_64|amd64) arch=amd64 ;;
		aarch64|arm64) arch=arm64 ;;
		*) echo "error: unsupported architecture" >&2; exit 1 ;;
	esac

	ver_no_v=${tag#v}
	archive="larascope_${ver_no_v}_${os}_${arch}.tar.gz"
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM HUP
	fetch "$BASE_URL/download/$tag/$archive" "$tmp/$archive"
	fetch "$BASE_URL/download/$tag/checksums.txt" "$tmp/checksums.txt"

	matching=$(awk -v file="$archive" '$2 == file { print $1 }' "$tmp/checksums.txt")
	count=$(printf '%s\n' "$matching" | sed '/^$/d' | wc -l | tr -d ' ')
	if [ "$count" -ne 1 ]; then
		echo "error: checksum for $archive is missing or ambiguous" >&2
		exit 1
	fi
	expected=$matching
	if command -v sha256sum >/dev/null 2>&1; then
		actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
	elif command -v shasum >/dev/null 2>&1; then
		actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
	else
		echo "error: sha256sum or shasum is required for verification" >&2
		exit 1
	fi
	if [ "$expected" != "$actual" ]; then
		echo "error: checksum mismatch for $archive" >&2
		echo "expected: $expected" >&2
		echo "actual:   $actual" >&2
		exit 1
	fi

	mkdir "$tmp/x"
	tar -xzf "$tmp/$archive" -C "$tmp/x" larascope
	if [ ! -f "$tmp/x/larascope" ]; then
		echo "error: archive does not contain larascope" >&2
		exit 1
	fi
	if ! mkdir -p "$INSTALL_DIR" || [ ! -w "$INSTALL_DIR" ]; then
		echo "error: install directory is not writable; set LARASCOPE_INSTALL_DIR" >&2
		exit 1
	fi
	cp "$tmp/x/larascope" "$INSTALL_DIR/.larascope.new.$$"
	chmod 755 "$INSTALL_DIR/.larascope.new.$$"
	mv -f "$INSTALL_DIR/.larascope.new.$$" "$INSTALL_DIR/larascope"
	echo "Installed Larascope to $INSTALL_DIR/larascope"
	"$INSTALL_DIR/larascope" --version
	case ":${PATH}:" in
		*":${INSTALL_DIR}:"*) ;;
		*) echo "Add it to PATH: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
	esac
}

main
