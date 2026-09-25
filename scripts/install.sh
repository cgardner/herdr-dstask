#!/usr/bin/env bash
# Herdr plugin build hook. It compiles from source, so installing needs the Go
# toolchain until release binaries exist.
set -euo pipefail
cd "$(dirname "$0")/.."

binary="herdr-dstask"
version="$(sed -n 's/^version = "\(.*\)"/\1/p' herdr-plugin.toml | head -1)"

if ! command -v go >/dev/null; then
  echo "${binary}: the Go toolchain is needed to build from source" >&2
  exit 1
fi
mkdir -p bin
go build -trimpath \
  -ldflags "-s -w -X github.com/cgardner/herdr-dstask/internal/cli.version=${version}" \
  -o "bin/${binary}" .
echo "${binary}: built v${version}"
