#!/usr/bin/env bash
# Open the plugin popup on a copy of the dstask repository, so a test can
# change tasks freely. The copy keeps .git/dstask, which holds the task IDs,
# so the IDs match the real repository. Its remotes are removed, so a
# `dstask sync` inside the sandbox cannot push to the real remote.
#
# Usage: scripts/sandbox.sh [--fresh]   (--fresh copies the repository again)
set -euo pipefail
cd "$(dirname "$0")/.."

src="${DSTASK_GIT_REPO:-$HOME/.dstask}"
dest="${HERDR_DSTASK_SANDBOX:-${TMPDIR:-/tmp}/herdr-dstask-sandbox}"
herdr="${HERDR_BIN_PATH:-herdr}"

if [ "${1:-}" = "--fresh" ] || [ ! -d "$dest/.git" ]; then
  rm -rf "$dest"
  # rsync, not cp: git's fsmonitor socket cannot be copied, and cp warns.
  rsync -a --exclude='*.ipc' "$src/" "$dest/"
  for remote in $(git -C "$dest" remote); do
    git -C "$dest" remote remove "$remote"
  done
  echo "sandbox: copied $src to $dest"
else
  echo "sandbox: reusing $dest (pass --fresh to copy again)"
fi

if ! "$herdr" plugin pane open --plugin cgardner.herdr-dstask --entrypoint tasks \
  --focus --env "DSTASK_GIT_REPO=$dest" >/dev/null; then
  echo "sandbox: could not open the Herdr pane; outside Herdr, run:" >&2
  echo "  DSTASK_GIT_REPO=$dest $(pwd)/bin/herdr-dstask" >&2
  exit 1
fi
