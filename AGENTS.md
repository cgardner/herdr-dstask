# AGENTS.md

A Herdr plugin: a terminal UI for dstask. Go, Bubble Tea, and the dstask
library. Run `make help` for the targets. `README.md` has the keys and the
limits. This file has only what the code and the README cannot tell you.

## Use the dstask library, not the binary

The plugin imports `github.com/naggie/dstask` and does not run `dstask`. Do
not add a code path that runs the binary or parses its output. The library
has three habits that are dangerous in a full-screen UI. `internal/store`
works around each one, and a new store method must do the same:

- **The `Command*` functions are for the CLI.** They print to stdout, and
  they ask for confirmation when stdout is a terminal. In a Herdr pane it is
  a terminal. Use the `TaskSet` API: `LoadTaskSet`, `GetByID`, `UpdateTask`,
  `SavePendingChanges`, `GitCommit`. Copy the small logic that the command
  adds, and use the commit message of the CLI (`"Resolved %s"`, and so on).
- **`RunCmd` connects git to `os.Stdout`, and `LoadTaskSet` logs to
  stderr.** Put every library call inside `quiet()`. It points `os.Stdout`,
  `os.Stderr`, `os.Stdin` and the standard logger at a temporary file. Bubble
  Tea keeps its own terminal handles, so the UI is not affected.
- **The `Must*` helpers call `os.Exit`.** Use the variant that returns an
  error. `SavePendingChanges` has no such variant.

## Traps in the library

- `ParseQuery` takes the first word that names a command as the command.
  Thus "edit the docs" loses "edit". `parse()` puts the command word first,
  as the CLI does.
- `ParseQuery` reads every leading number as a task ID. `Modify` and `Add`
  refuse input that starts with a number. `Note` never parses its text.
- `Task.Modify` adds a newline to non-empty notes on every call. `Modify`
  puts the notes back when the query has no note.
- An ID-less modify in the CLI changes every task in the context, with no
  question when stdout is not a terminal. The store requires an ID for every
  change.
- A resolved task has ID 0. The CLI can address a task only by its ID, so the
  UI refuses changes to resolved tasks.
- `String()` on a task gives `"<id>: <summary>"`. The commit messages use it.

## Tests never touch ~/.dstask

`internal/store` tests make a new git repository in `t.TempDir()` and open it
with `NewAt`. `internal/ui` tests use a fake `Backend`. For a manual test of
the TUI, set `DSTASK_GIT_REPO` to a scratch repository.

## Testing the TUI in Herdr

Keys sent fast with `herdr pane send-keys` can arrive together, and Bubble
Tea reads `esc` then `k` as `alt+k`. Send one key for each call, with a
short pause.

## Coverage floor

`make cover` fails below 90% of statements, and `make ci` runs it. Coverage
uses `-coverpkg` over `./internal/...` and the root, so a call from one
package into another counts.

Code that reaches outside the process sits behind a package variable that a
test replaces: `cli.openStore` and `cli.runProgram`. The editor callback is
`ui.editorDone`, a named function, because a test cannot reach a closure
inside `tea.ExecProcess`. Keep a new outside call behind the same pattern.
Only `main` and a few error returns for disk failures are not covered.

## Colors follow the terminal theme

Every foreground color is an ANSI palette index from 0 to 15, so the terminal
theme decides the real color. Herdr gives a plugin no way to read its own
theme: the API schema and the snapshot carry no color data. The terminal
palette is thus the only theme the pane can follow. If the terminal and Herdr
use the same theme, the colors match. `TestColorsArePaletteIndexes` fails on a
fixed 256-color or hex value.

The selection background is the exception. It must match Herdr's own
overlays, so it comes from `selection_bg` in the Herdr config, as in
herdr-switcher-plus.

The priority column is a one-column dot: `●` for P0 and P1, `○` for P2, `·`
for P3, colored by priority. Do not use emoji. They carry their own colors,
which ignore the theme.

## Releasing

The setup is the same as in herdr-switcher-plus. A conventional commit on
`main` opens a release pull request through release-please. It bumps
`version` in `herdr-plugin.toml` and writes `CHANGELOG.md`. Merging it tags
the release, and the build job in the same workflow cross-compiles the four
platforms, writes `SHA256SUMS` and attaches them.

- The build runs inside `release-please.yml`, not in a workflow that listens
  for the tag. GitHub does not start a workflow from an event that
  `GITHUB_TOKEN` creates, so a tag that release-please pushes never fires
  `on: push: tags`. `release.yml` covers a tag pushed by hand. Both call
  `build-release.yml`.
- Three places carry the version and all three must agree: the git tag,
  `version` in `herdr-plugin.toml`, and the download URL that
  `scripts/install.sh` builds from that version. The build fails a tag that
  disagrees with the manifest.
- The manifest starts empty with `initial-version` set to 0.1.0. A manifest
  that names a version before any release makes release-please skip it.
- The `PLATFORMS` list in the `Makefile` and the `uname` cases in
  `scripts/install.sh` name the same four targets. Change both or neither.
- GitHub blocks Actions from opening pull requests unless the repository
  allows it, under Settings, Actions, General, Workflow permissions.
- Do not edit `CHANGELOG.md` by hand. Before the first release the file must
  not exist. On a first release, release-please keeps any existing file below
  its new section and changes its `# Changelog` title to `## Changelog`, so
  even a file with only the title leaves a stray heading at the end.
  herdr-switcher-plus still carries one. From the second release on, the
  sections go under the title correctly.
