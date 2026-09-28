#!/usr/bin/env bash
# Make docs/images/screenshot.png and docs/images/social-preview.png.
#
# It must run inside Herdr, because Herdr is the terminal emulator: it runs
# `make demo` in a pane of a fixed width, reads the finished screen back with
# `herdr pane read --format ansi`, and scripts/screenshot.py paints that screen.
# The demo shows invented tasks only. The capture tab is closed on exit.
#
# Needs rsvg-convert (librsvg) and the FiraCode Nerd Font Mono font.
set -euo pipefail
cd "$(dirname "$0")/.."

[ "${HERDR_ENV:-}" = 1 ] || { echo "screenshot: run this inside a Herdr pane" >&2; exit 1; }
command -v rsvg-convert >/dev/null || { echo "screenshot: rsvg-convert is needed" >&2; exit 1; }
make build >/dev/null

WIDTH=115 # wide enough for every key hint in the footer
work="$(mktemp -d)"
json() { python3 -c "import json,sys; print(json.load(sys.stdin)$1)"; }

tab_json="$(herdr tab create --workspace "$HERDR_WORKSPACE_ID" --cwd "$PWD" --label screenshot --no-focus)"
tab="$(json '["result"]["tab"]["tab_id"]' <<<"$tab_json")"
left="$(json '["result"]["root_pane"]["pane_id"]' <<<"$tab_json")"
trap 'herdr tab close "$tab" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT

pane="$(herdr pane split "$left" --direction right --cwd "$PWD" --no-focus | json '["result"]["pane"]["pane_id"]')"
width() { herdr pane layout --pane "$pane" | python3 -c "
import json, sys
panes = json.load(sys.stdin)['result']['layout']['panes']
print(next(p['rect']['width'] for p in panes if p['pane_id'] == '$pane'))"; }
# Grow the pane a little at a time until it reaches the width.
for _ in $(seq 40); do
  [ "$(width)" -ge "$WIDTH" ] && break
  herdr pane resize --pane "$pane" --direction left --amount 0.01 >/dev/null
done

herdr pane run "$pane" "bash scripts/demo.sh" >/dev/null
herdr pane wait-output "$pane" --match "build a cold frame" --timeout 60000 >/dev/null
herdr pane send-text "$pane" j >/dev/null # select the active task
sleep 0.5
herdr pane read "$pane" --source visible --format ansi >"$work/capture.ansi"

python3 scripts/screenshot.py "$work/capture.ansi" "$work"
mkdir -p docs/images
for name in screenshot social-preview; do
  rsvg-convert -z 2 "$work/$name.svg" -o "docs/images/$name.png"
  echo "wrote docs/images/$name.png"
done
