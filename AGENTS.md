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

## Emoji in the list

The priority column is a colored circle, 🔴 🟠 🟡 🔵 for P0 to P3. Each is one
code point that terminals draw two columns wide, and Herdr agrees. Do not use
an emoji that needs a variation selector, such as ⬆️ or ⚠️. Terminals do not
agree on its width, and the columns after it move. `TestRowsStayAlignedAcross
Priorities` catches a mark of the wrong width.
