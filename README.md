# herdr-dstask

A [Herdr](https://herdr.dev) plugin that puts [dstask](https://github.com/naggie/dstask)
in the control plane. It opens a popup with a list of your tasks. From the
popup you can view a task and change it without a switch to a shell.

The plugin reads and writes the task repository through the dstask Go
library, `github.com/naggie/dstask`. It does not run the `dstask` binary. The
repository, the context and the commit history are the same ones that the
CLI uses.

## Install

```sh
make link        # build, then link this working copy into the running Herdr
```

To open the list with a key, add a binding to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+t"
type = "plugin_action"
command = "cgardner.herdr-dstask.open"
description = "dstask tasks"
```

To test on a copy of your tasks, open the `dstask: tasks (sandbox copy)`
action, or run `make sandbox`. It copies `~/.dstask` to
`$TMPDIR/herdr-dstask-sandbox` one time and removes the git remotes from the
copy. Thus changes and `dstask sync` in the sandbox cannot reach your real
tasks. `make sandbox FRESH=1` copies the repository again.

```toml
[[keys.command]]
key = "prefix+alt+t"
type = "plugin_action"
command = "cgardner.herdr-dstask.open-sandbox"
description = "dstask tasks (sandbox copy)"
```

`make run` starts the UI in the current terminal, outside Herdr.
`make list` prints the open tasks and does not start the UI.

## Keys

| key | action |
|---|---|
| `j` `k` `g` `G` `ctrl+d` `ctrl+u` | move |
| `enter` | view the task: all fields and the notes |
| `/` | filter by words in the summary, project, tags or notes |
| `tab` | switch between open and resolved tasks |
| `c` | switch between the dstask context and all tasks |
| `a` | add a task, in dstask syntax: `+tag project:x P1 summary` |
| `s` | start the task, or stop it if it is active |
| `d` | resolve |
| `m` | modify: `+tag -tag project:x P1 due:friday` |
| `n` | append a line to the notes |
| `N` | edit the notes in `$EDITOR` |
| `e` | edit the whole task as YAML in `$EDITOR` |
| `x` | remove, after a confirmation |
| `u` | undo the last change (`git revert`, the same as `dstask undo`) |
| `?` | help |
| `q` `esc` | quit |

## Environment

The plugin uses the same variables as dstask: `DSTASK_GIT_REPO` for the
repository (default `~/.dstask`) and `DSTASK_CONTEXT` for a context override.
`$EDITOR` sets the editor, and dstask uses `vim` when it is empty.

## Limits

- The plugin does not sync. `dstask sync` is still necessary to push and pull.
- A resolved task has no ID in dstask, so the plugin cannot change it. Undo
  can restore a task that you resolved by mistake.
- Templates are not supported yet.
- The dstask library calls `os.Exit` when it cannot write a task file to disk.
  If that occurs, the popup closes. The terminal can stay in the alternate
  screen, and `reset` repairs it.

## Toward Herdr integration

The UI talks to one `Backend` interface, and `internal/store` is the only
package that knows dstask. Thus a Herdr link can go in a new package with no
change to the store. Some possible next steps:

- Start an agent on a task: a key that opens a pane and prompts an agent with
  the summary and notes of the task.
- Link a task to a workspace: record the task UUID in the pane metadata, and
  show the active task of each agent in the sidebar.
- Filter by workspace: use the Herdr workspace or repository name as a
  project filter when the popup opens.
- Resolve a task when its agent reaches `done`.
