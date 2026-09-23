#!/bin/sh
# Installs the latest geoimg release for macOS or Linux.
#
#   curl -fsSL https://github.com/osint-builders/geoimg/releases/latest/download/install.sh | sh
#
# Environment:
#   GEOIMG_VERSION      tag to install (default: latest)
#   GEOIMG_INSTALL_DIR  target directory (default: /usr/local/bin if writable, else ~/.local/bin)
set -eu

REPO="osint-builders/geoimg"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "geoimg: unsupported OS $(uname -s); download a build from https://github.com/$REPO/releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "geoimg: unsupported CPU $(uname -m)" >&2; exit 1 ;;
esac

if [ -n "${GEOIMG_VERSION:-}" ]; then
  base="https://github.com/$REPO/releases/download/$GEOIMG_VERSION"
else
  base="https://github.com/$REPO/releases/latest/download"
fi
archive="geoimg_${os}_${arch}.tar.gz"

dir="${GEOIMG_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading $archive"
curl -fsSL "$base/$archive" -o "$tmp/$archive"
curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

expected="$(grep " $archive\$" "$tmp/SHA256SUMS" | cut -d' ' -f1)"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$archive" | cut -d' ' -f1)"
else
  actual="$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)"
fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
  echo "geoimg: checksum mismatch for $archive" >&2
  exit 1
fi

tar -xzf "$tmp/$archive" -C "$tmp"
mkdir -p "$dir"
install -m 0755 "$tmp/geoimg_${os}_${arch}/geoimg" "$dir/geoimg"
echo "installed $("$dir/geoimg" -version) to $dir/geoimg"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "note: add $dir to your PATH" ;;
esac
