#!/usr/bin/env bash
# Herdr plugin build hook.
#
# A GitHub install runs this. It fetches the prebuilt binary for the running
# platform and falls back to compiling from source, so installing needs the Go
# toolchain only when no release asset matches.
set -euo pipefail
cd "$(dirname "$0")/.."

repo="cgardner/herdr-dstask"
binary="herdr-dstask"
version="$(sed -n 's/^version = "\(.*\)"/\1/p' herdr-plugin.toml | head -1)"

# These names must match the PLATFORMS list in the Makefile, which is what the
# release workflow builds.
case "$(uname -s)-$(uname -m)" in
  Darwin-arm64)              target=darwin-arm64 ;;
  Darwin-x86_64)             target=darwin-amd64 ;;
  Linux-x86_64)              target=linux-amd64 ;;
  Linux-aarch64|Linux-arm64) target=linux-arm64 ;;
  *)                         target="" ;;
esac

mkdir -p bin
url="https://github.com/${repo}/releases/download/v${version}/${binary}-${target}"

if [ -n "$target" ] && curl -fsSL "$url" -o "bin/${binary}.tmp"; then
  mv "bin/${binary}.tmp" "bin/${binary}"
  chmod +x "bin/${binary}"
  echo "${binary}: installed the prebuilt ${target} binary for v${version}"
elif command -v go >/dev/null; then
  echo "${binary}: no prebuilt binary for v${version}; building from source" >&2
  rm -f "bin/${binary}.tmp"
  go build -trimpath \
    -ldflags "-s -w -X github.com/${repo}/internal/cli.version=${version}" \
    -o "bin/${binary}" .
else
  echo "${binary}: no prebuilt binary for ${target:-$(uname -s)-$(uname -m)} and no Go toolchain" >&2
  exit 1
fi
