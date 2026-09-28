#!/usr/bin/env bash
# Run the plugin on an invented task repository, for screenshots.
#
# Screenshots for the documentation must not carry real tasks. This builds a
# throwaway repository with tools/demofixture and points the plugin at it, so
# the picture is genuine output of the real rendering path with nothing real
# in it. The repository lives in a temporary directory that is removed on exit.
set -euo pipefail
cd "$(dirname "$0")/.."

[ -x bin/herdr-dstask ] || make build >/dev/null

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

go run ./tools/demofixture "$FIXTURE"
DSTASK_GIT_REPO="$FIXTURE" DSTASK_CONTEXT="" ./bin/herdr-dstask "$@"
