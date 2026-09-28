// Package store reads and changes tasks through the dstask library,
// github.com/naggie/dstask, and not through the dstask binary.
//
// The library is written for the dstask CLI, and three of its habits are
// hostile to a full-screen terminal UI. Each change here works around one:
//
//   - The Command* functions print to stdout and ask for confirmation when
//     stdout is a terminal, which it is in a Herdr pane. The store uses the
//     lower-level TaskSet API instead and repeats only the small amount of
//     logic each command adds on top of it.
//   - RunCmd, which GitCommit uses, connects git to os.Stdout and os.Stderr,
//     and LoadTaskSet logs to stderr. Output written there would land on top
//     of the UI, so every library call runs inside quiet(), which points both
//     at a buffer for the duration.
//   - The Must* helpers call os.Exit. The store calls the variants that
//     return an error wherever the library offers one. SavePendingChanges
//     offers none, so a disk failure while writing a task still exits.
package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/naggie/dstask"
	yaml "gopkg.in/yaml.v2"
)

// Store is one dstask repository.
type Store struct {
	conf dstask.Config

	// IgnoreContext lists every task, as "--" does on the CLI.
	IgnoreContext bool
}

// New opens the repository that dstask itself would use, honoring
// DSTASK_GIT_REPO and DSTASK_CONTEXT.
func New() (*Store, error) {
	conf := dstask.NewConfig()
	if _, err := os.Stat(conf.Repo + "/.git"); err != nil {
		return nil, fmt.Errorf("no dstask repository at %s (run dstask once to create it)", conf.Repo)
	}
	return &Store{conf: conf}, nil
}

// NewAt opens a repository at an explicit path, and otherwise behaves as New
// does: DSTASK_CONTEXT still applies. Tests use it.
func NewAt(repo string) *Store {
	conf := dstask.Config{
		CtxFromEnvVar: os.Getenv("DSTASK_CONTEXT"),
		Repo:          repo,
		StateFile:     repo + "/.git/dstask/state.bin",
		IDsFile:       repo + "/.git/dstask/ids.bin",
	}
	return &Store{conf: conf}
}

// Repo is the repository path.
func (s *Store) Repo() string { return s.conf.Repo }

// SetIgnoreContext switches between the active context and every task.
func (s *Store) SetIgnoreContext(v bool) { s.IgnoreContext = v }

// Context is the context the listings apply, following the same precedence
// as the CLI: DSTASK_CONTEXT, then the saved state, and nothing when the
// store ignores context.
func (s *Store) Context() dstask.Query {
	if s.IgnoreContext {
		return dstask.Query{}
	}
	if s.conf.CtxFromEnvVar != "" {
		return dstask.ParseQuery(strings.Fields(s.conf.CtxFromEnvVar)...)
	}
	var ctx dstask.Query
	quiet(func() error {
		ctx = dstask.LoadState(s.conf.StateFile).Context
		return nil
	})
	return ctx
}

// ContextString renders the active context, for example "+work project:x".
func (s *Store) ContextString() string { return s.Context().String() }

// Open lists the unresolved tasks in the context, in the CLI's "next" order.
func (s *Store) Open() ([]dstask.Task, error) {
	ctx := s.Context()
	var tasks []dstask.Task
	err := quiet(func() error {
		ts, err := dstask.LoadTaskSet(s.conf.Repo, s.conf.IDsFile, false)
		if err != nil {
			return err
		}
		ts.Filter(ctx)
		ts.SortByCreated(dstask.Ascending)
		ts.SortByPriority(dstask.Ascending)
		tasks = ts.Tasks()
		return nil
	})
	return tasks, err
}

// Resolved lists resolved tasks in the context, newest first.
func (s *Store) Resolved() ([]dstask.Task, error) {
	ctx := s.Context()
	var tasks []dstask.Task
	err := quiet(func() error {
		ts, err := dstask.LoadTaskSet(s.conf.Repo, s.conf.IDsFile, true)
		if err != nil {
			return err
		}
		ts.UnHide()
		ts.Filter(ctx)
		ts.FilterByStatus(dstask.STATUS_RESOLVED)
		ts.SortByResolved(dstask.Descending)
		tasks = ts.Tasks()
		return nil
	})
	return tasks, err
}

// Project is one project's progress. The counts come from dstask's own
// TaskSet.GetProjects, the same numbers that `dstask show-projects` prints,
// plus three that it does not give.
type Project struct {
	dstask.Project

	// ActiveTasks, PausedTasks and OverdueTasks count open tasks only.
	ActiveTasks, PausedTasks, OverdueTasks int
}

// OpenTasks is the number of tasks not yet resolved.
func (p Project) OpenTasks() int { return p.Tasks - p.TasksResolved }

// Done is the fraction of tasks resolved, from 0 to 1.
func (p Project) Done() float64 {
	if p.Tasks == 0 {
		return 0
	}
	return float64(p.TasksResolved) / float64(p.Tasks)
}

// Projects lists every project with its progress. It loads resolved tasks,
// which the task listings do not, so it is slower: about 200 ms for 750
// tasks. Like `dstask show-projects`, it ignores the context, because a
// project's progress needs all of its tasks.
func (s *Store) Projects() ([]Project, error) {
	var out []Project
	now := time.Now()
	err := quiet(func() error {
		ts, err := dstask.LoadTaskSet(s.conf.Repo, s.conf.IDsFile, true)
		if err != nil {
			return err
		}
		index := map[string]int{}
		for _, p := range ts.GetProjects() {
			index[p.Name] = len(out)
			out = append(out, Project{Project: *p})
		}
		for _, t := range ts.AllTasks() {
			i, ok := index[t.Project]
			if !ok || t.Status == dstask.STATUS_RESOLVED {
				continue
			}
			switch t.Status {
			case dstask.STATUS_ACTIVE:
				out[i].ActiveTasks++
			case dstask.STATUS_PAUSED:
				out[i].PausedTasks++
			}
			if !t.Due.IsZero() && t.Due.Before(now) {
				out[i].OverdueTasks++
			}
		}
		return nil
	})
	return out, err
}

// Done resolves a task.
func (s *Store) Done(id int) error {
	return s.change(id, "Resolved %s", func(t *dstask.Task) error {
		t.Status = dstask.STATUS_RESOLVED
		t.Resolved = time.Now()
		return nil
	})
}

// Start marks a task active.
func (s *Store) Start(id int) error {
	return s.change(id, "Started %s", func(t *dstask.Task) error {
		t.Status = dstask.STATUS_ACTIVE
		return nil
	})
}

// Stop pauses an active task. The CLI's stop writes "paused", not "pending".
func (s *Store) Stop(id int) error {
	return s.change(id, "Stopped %s", func(t *dstask.Task) error {
		t.Status = dstask.STATUS_PAUSED
		return nil
	})
}

// Remove deletes a task. The caller must confirm first.
func (s *Store) Remove(id int) error {
	return s.change(id, "Removed: %s", func(t *dstask.Task) error {
		t.Deleted = true
		return nil
	})
}

// Modify applies dstask modifiers such as "+tag -tag project:x P1 due:friday",
// parsed by the library's own ParseQuery.
func (s *Store) Modify(id int, modifiers string) error {
	q := parse(dstask.CMD_MODIFY, modifiers)
	if len(q.IDs) > 0 {
		// ParseQuery reads every leading number as a task ID. Refusing it
		// keeps the change on the task the user selected.
		return fmt.Errorf("%d reads as a task id, not a modifier", q.IDs[0])
	}
	if !q.HasOperators() {
		return errors.New("no modifiers given (for example +tag -tag project:x P1 due:friday)")
	}
	return s.change(id, "Modified %s", func(t *dstask.Task) error {
		notes := t.Notes
		t.Modify(q)
		// Task.Modify appends a newline to non-empty notes even when the
		// query carries no note, so every modify would grow the notes.
		if q.Note == "" {
			t.Notes = notes
		}
		return nil
	})
}

// Note appends a line to the task's notes, as `dstask N note text` does. The
// text is not parsed, so it can hold anything.
func (s *Store) Note(id int, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("empty note")
	}
	return s.change(id, "Edit note %s", func(t *dstask.Task) error {
		if t.Notes == "" {
			t.Notes = text
		} else {
			t.Notes += "\n" + text
		}
		return nil
	})
}

// Add creates a pending task from dstask syntax, so "+work P1 fix it" is a P1
// task tagged work. The context applies, as it does on the CLI.
func (s *Store) Add(input string) error {
	q := parse(dstask.CMD_ADD, input)
	if len(q.IDs) > 0 {
		return fmt.Errorf("%d reads as a task id; templates are not supported yet", q.IDs[0])
	}
	if q.Template > 0 {
		return errors.New("templates are not supported yet")
	}
	if q.Text == "" {
		return errors.New("a task needs a summary")
	}
	ctx := s.Context()
	q = q.Merge(ctx)
	return quiet(func() error {
		ts, err := dstask.LoadTaskSet(s.conf.Repo, s.conf.IDsFile, false)
		if err != nil {
			return err
		}
		task, err := ts.LoadTask(dstask.Task{
			WritePending: true,
			Status:       dstask.STATUS_PENDING,
			Summary:      q.Text,
			Tags:         q.Tags,
			Project:      q.Project,
			Priority:     q.Priority,
			Due:          q.Due,
			Notes:        q.Note,
		})
		if err != nil {
			return err
		}
		ts.SavePendingChanges()
		return dstask.GitCommit(s.conf.Repo, "Added %s", task)
	})
}

// parse reads input with the library's own parser, after the command word
// the CLI would have seen. Without it ParseQuery takes the first word that
// names a command as the command, so "edit the docs" would lose "edit".
func parse(cmd, input string) dstask.Query {
	return dstask.ParseQuery(append([]string{cmd}, strings.Fields(input)...)...)
}

// Undo reverts the last commit in the repository, as `dstask undo` does.
func (s *Store) Undo() error {
	return quiet(func() error {
		return dstask.RunGitCmd(s.conf.Repo, "revert", "--no-gpg-sign", "--no-edit", "HEAD~1..")
	})
}

// change loads the open tasks, applies fn to one of them, and saves and
// commits the result with a message in the CLI's words.
func (s *Store) change(id int, message string, fn func(*dstask.Task) error) error {
	if id <= 0 {
		return errors.New("resolved tasks have no id, so dstask cannot address them")
	}
	return quiet(func() error {
		ts, err := dstask.LoadTaskSet(s.conf.Repo, s.conf.IDsFile, false)
		if err != nil {
			return err
		}
		task, err := ts.GetByID(id)
		if err != nil {
			return err
		}
		if err := fn(&task); err != nil {
			return err
		}
		if err := ts.UpdateTask(task); err != nil {
			return err
		}
		ts.SavePendingChanges()
		return dstask.GitCommit(s.conf.Repo, message, task)
	})
}

// Edit is a task written to a temporary file for the user's editor. The
// editor needs the terminal, so the UI runs Cmd and then calls Apply.
type Edit struct {
	store *Store
	uuid  string
	id    int
	path  string
	notes bool
	Cmd   *exec.Cmd
}

// EditTask writes the task as YAML, the format `dstask edit` uses.
func (s *Store) EditTask(id int) (*Edit, error) { return s.edit(id, false) }

// EditNotes writes only the notes, as `dstask note` with no text does.
func (s *Store) EditNotes(id int) (*Edit, error) { return s.edit(id, true) }

func (s *Store) edit(id int, notes bool) (*Edit, error) {
	var task dstask.Task
	err := quiet(func() error {
		ts, err := dstask.LoadTaskSet(s.conf.Repo, s.conf.IDsFile, false)
		if err != nil {
			return err
		}
		task, err = ts.GetByID(id)
		return err
	})
	if err != nil {
		return nil, err
	}
	var data []byte
	ext := "yml"
	if notes {
		data, ext = []byte(task.Notes), "md"
	} else if data, err = yaml.Marshal(&task); err != nil {
		return nil, fmt.Errorf("marshal task %d: %w", id, err)
	}
	f, err := os.CreateTemp("", dstask.MakeTempFilename(task.ID, task.Summary, ext))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	editor := strings.Fields(os.Getenv("EDITOR"))
	if len(editor) == 0 {
		editor = []string{"vim"}
	}
	return &Edit{
		store: s, uuid: task.UUID, id: id, path: f.Name(), notes: notes,
		Cmd: exec.Command(editor[0], append(editor[1:], f.Name())...),
	}, nil
}

// Apply reads the edited file back and saves it. It removes the file whether
// or not the edit succeeds.
func (e *Edit) Apply() error {
	defer os.Remove(e.path)
	data, err := os.ReadFile(e.path)
	if err != nil {
		return err
	}
	message := "Edited %s"
	if e.notes {
		message = "Edit note %s"
	}
	return e.store.change(e.id, message, func(t *dstask.Task) error {
		// The ID can point at another task if the list changed while the
		// editor was open. The UUID cannot.
		if t.UUID != e.uuid {
			return fmt.Errorf("task %d changed while the editor was open; the edit is in %s", e.id, e.keep())
		}
		if e.notes {
			t.Notes = strings.TrimRight(string(data), "\n")
			return nil
		}
		edited := *t
		if err := yaml.Unmarshal(data, &edited); err != nil {
			return fmt.Errorf("the edit is not valid YAML: %w (kept in %s)", err, e.keep())
		}
		*t = edited
		return nil
	})
}

// keep copies the edited file somewhere the deferred removal will not reach,
// so a failed edit is not lost.
func (e *Edit) keep() string {
	kept := e.path + ".kept"
	if data, err := os.ReadFile(e.path); err == nil && os.WriteFile(kept, data, 0o600) == nil {
		return kept
	}
	return e.path
}

// Discard removes the temporary file without saving.
func (e *Edit) Discard() { os.Remove(e.path) }

var quietMu sync.Mutex

// quiet runs fn with os.Stdout, os.Stderr and the standard logger pointed at a
// buffer, and with stdin closed so git can never wait for input. On failure the
// captured output is added to the error, because git explains itself there.
//
// Swapping the os variables is safe for the UI: Bubble Tea keeps its own
// handles to the terminal from startup and never reads these variables again.
func quiet(fn func() error) error {
	quietMu.Lock()
	defer quietMu.Unlock()

	sink, err := os.CreateTemp("", "herdr-dstask-*.log")
	if err != nil {
		return err
	}
	defer os.Remove(sink.Name())
	defer sink.Close()
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer devnull.Close()

	stdout, stderr, stdin := os.Stdout, os.Stderr, os.Stdin
	logOut := log.Writer()
	os.Stdout, os.Stderr, os.Stdin = sink, sink, devnull
	log.SetOutput(sink)
	defer func() {
		os.Stdout, os.Stderr, os.Stdin = stdout, stderr, stdin
		log.SetOutput(logOut)
	}()

	if err := fn(); err != nil {
		if _, serr := sink.Seek(0, io.SeekStart); serr == nil {
			var buf bytes.Buffer
			buf.ReadFrom(sink)
			if out := strings.TrimSpace(buf.String()); out != "" {
				return fmt.Errorf("%w: %s", err, lastLine(out))
			}
		}
		return err
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Command is the editor process, for the UI to run on the terminal.
func (e *Edit) Command() *exec.Cmd { return e.Cmd }
