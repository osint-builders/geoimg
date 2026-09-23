#!/usr/bin/env bash
# Cross-compiles geoimg for every supported platform and packages release
# archives plus a SHA256SUMS file into dist/.
#
#   scripts/dist.sh [version]      (default: git describe, else "dev")
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
TARGETS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"

rm -rf dist && mkdir -p dist
for target in $TARGETS; do
  os="${target%/*}" arch="${target#*/}"
  name="geoimg_${os}_${arch}"
  bin="geoimg"; [ "$os" = windows ] && bin="geoimg.exe"
  echo "building $name"
  mkdir -p "dist/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -tags nodynamic \
    -ldflags "-s -w -X main.version=$VERSION" -o "dist/$name/$bin" ./cmd/geoimg
  cp README.md "dist/$name/"
  if [ "$os" = windows ]; then
    (cd dist && zip -qr "$name.zip" "$name")
  else
    tar -C dist -czf "dist/$name.tar.gz" "$name"
  fi
  rm -rf "dist/$name"
done
cp scripts/install.sh scripts/install.ps1 dist/
(cd dist && sha256sum ./*.tar.gz ./*.zip | sed 's| \./| |' > SHA256SUMS)
ls -lh dist
