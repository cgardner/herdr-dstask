package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naggie/dstask"
)

// repo makes an empty dstask repository in a temporary directory. The tests
// never touch the user's own repository.
func repo(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"config", "commit.gpgsign", "false"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	t.Setenv("DSTASK_CONTEXT", "")
	return NewAt(dir)
}

func mustAdd(t *testing.T, s *Store, input string) dstask.Task {
	t.Helper()
	if err := s.Add(input); err != nil {
		t.Fatalf("add %q: %v", input, err)
	}
	tasks, err := s.Open()
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if strings.Contains(input, task.Summary) {
			return task
		}
	}
	t.Fatalf("added %q but it is not listed", input)
	return dstask.Task{}
}

func gitLog(t *testing.T, s *Store) string {
	t.Helper()
	out, err := exec.Command("git", "-C", s.Repo(), "log", "--format=%s").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestAddParsesDstaskSyntaxAndCommits(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "+work project:herdr P1 wire the plugin")
	if task.Summary != "wire the plugin" || task.Project != "herdr" || task.Priority != "P1" {
		t.Errorf("task decoded wrong: %+v", task)
	}
	if len(task.Tags) != 1 || task.Tags[0] != "work" {
		t.Errorf("tags = %v, want [work]", task.Tags)
	}
	if task.ID != 1 {
		t.Errorf("id = %d, want 1", task.ID)
	}
	if !strings.Contains(gitLog(t, s), "Added 1: wire the plugin") {
		t.Errorf("commit message missing:\n%s", gitLog(t, s))
	}
}

func TestAddRefusesALeadingNumber(t *testing.T) {
	s := repo(t)
	if err := s.Add("5 minutes of work"); err == nil {
		t.Fatal("expected an error")
	}
}

// "edit" and "note" are dstask commands. In a summary they are only words.
func TestAddKeepsCommandWordsInTheSummary(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "edit the notes")
	if task.Summary != "edit the notes" {
		t.Errorf("summary = %q", task.Summary)
	}
}

func TestAddNeedsASummary(t *testing.T) {
	s := repo(t)
	if err := s.Add("+work P1"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestStartStopDone(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "ship it")

	steps := []struct {
		fn   func(int) error
		want string
	}{
		{s.Start, dstask.STATUS_ACTIVE},
		{s.Stop, dstask.STATUS_PAUSED},
	}
	for _, step := range steps {
		if err := step.fn(task.ID); err != nil {
			t.Fatal(err)
		}
		open, _ := s.Open()
		if open[0].Status != step.want {
			t.Errorf("status = %s, want %s", open[0].Status, step.want)
		}
	}

	if err := s.Done(task.ID); err != nil {
		t.Fatal(err)
	}
	if open, _ := s.Open(); len(open) != 0 {
		t.Errorf("resolved task still open: %+v", open)
	}
	resolved, err := s.Resolved()
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].Resolved.IsZero() {
		t.Errorf("resolved listing wrong: %+v", resolved)
	}
	log := gitLog(t, s)
	for _, want := range []string{"Started 1: ship it", "Stopped 1: ship it", "Resolved 1: ship it"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
}

func TestModifyAppliesOperatorsAndKeepsNotes(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "+old tidy up")
	if err := s.Note(task.ID, "first line"); err != nil {
		t.Fatal(err)
	}
	if err := s.Modify(task.ID, "+new -old project:home P0"); err != nil {
		t.Fatal(err)
	}
	open, _ := s.Open()
	got := open[0]
	if got.Project != "home" || got.Priority != "P0" {
		t.Errorf("modify not applied: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "new" {
		t.Errorf("tags = %v, want [new]", got.Tags)
	}
	// dstask's Task.Modify adds a newline to the notes on every call.
	if got.Notes != "first line" {
		t.Errorf("notes = %q, want them unchanged", got.Notes)
	}
}

func TestModifyRefusesAnotherTaskID(t *testing.T) {
	s := repo(t)
	a := mustAdd(t, s, "first")
	mustAdd(t, s, "second")
	if err := s.Modify(a.ID, "2 +oops"); err == nil {
		t.Fatal("a leading number must be refused")
	}
	if err := s.Modify(a.ID, "just words"); err == nil {
		t.Fatal("input without operators must be refused")
	}
}

func TestNoteKeepsTextThatLooksLikeSyntax(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "call back")
	for _, line := range []string{"5 minutes", "+not-a-tag P0 project:nope"} {
		if err := s.Note(task.ID, line); err != nil {
			t.Fatal(err)
		}
	}
	open, _ := s.Open()
	if want := "5 minutes\n+not-a-tag P0 project:nope"; open[0].Notes != want {
		t.Errorf("notes = %q, want %q", open[0].Notes, want)
	}
	if len(open[0].Tags) != 0 || open[0].Priority != "P2" {
		t.Errorf("note text was parsed as syntax: %+v", open[0])
	}
}

func TestRemoveAndUndo(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "mistake")
	if err := s.Remove(task.ID); err != nil {
		t.Fatal(err)
	}
	if open, _ := s.Open(); len(open) != 0 {
		t.Fatalf("removed task still open")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if open, _ := s.Open(); len(open) != 1 {
		t.Fatalf("undo did not restore the task")
	}
}

func TestChangeRefusesIDZero(t *testing.T) {
	s := repo(t)
	if err := s.Done(0); err == nil {
		t.Fatal("expected an error")
	}
}

func TestUnknownIDReportsTheLibraryError(t *testing.T) {
	s := repo(t)
	err := s.Done(42)
	if err == nil || !strings.Contains(err.Error(), "42") {
		t.Fatalf("err = %v, want one naming the id", err)
	}
}

func TestContextFiltersListingsAndApplies(t *testing.T) {
	s := repo(t)
	mustAdd(t, s, "+home fix the gate")
	t.Setenv("DSTASK_CONTEXT", "+work")
	s.conf.CtxFromEnvVar = "+work"

	if err := s.Add("write the report"); err != nil {
		t.Fatal(err)
	}
	open, _ := s.Open()
	if len(open) != 1 || open[0].Summary != "write the report" {
		t.Fatalf("context listing wrong: %+v", open)
	}
	if len(open[0].Tags) != 1 || open[0].Tags[0] != "work" {
		t.Errorf("context not applied to the new task: %v", open[0].Tags)
	}
	if got := s.ContextString(); got != "+work" {
		t.Errorf("context = %q, want +work", got)
	}

	s.SetIgnoreContext(true)
	if all, _ := s.Open(); len(all) != 2 {
		t.Errorf("ignoring context lists %d tasks, want 2", len(all))
	}
}

func TestEditRoundTrip(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "edit me")

	ed, err := s.EditTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(ed.path)
	edited := strings.Replace(string(data), "summary: edit me", "summary: edited", 1)
	if edited == string(data) {
		t.Fatalf("YAML has no summary line:\n%s", data)
	}
	os.WriteFile(ed.path, []byte(edited), 0o600)
	if err := ed.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ed.path); !os.IsNotExist(err) {
		t.Errorf("temporary file left behind")
	}
	open, _ := s.Open()
	if open[0].Summary != "edited" {
		t.Errorf("summary = %q, want edited", open[0].Summary)
	}
}

func TestEditNotesRoundTrip(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "notes")
	ed, err := s.EditNotes(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(ed.path) != ".md" {
		t.Errorf("notes file %s should end in .md", ed.path)
	}
	os.WriteFile(ed.path, []byte("- [x] done\n\n"), 0o600)
	if err := ed.Apply(); err != nil {
		t.Fatal(err)
	}
	open, _ := s.Open()
	if open[0].Notes != "- [x] done" {
		t.Errorf("notes = %q", open[0].Notes)
	}
}

func TestEditKeepsInvalidYAML(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "broken")
	ed, _ := s.EditTask(task.ID)
	os.WriteFile(ed.path, []byte("summary: [unclosed"), 0o600)
	err := ed.Apply()
	if err == nil || !strings.Contains(err.Error(), ".kept") {
		t.Fatalf("err = %v, want one naming the kept file", err)
	}
	kept := ed.path + ".kept"
	defer os.Remove(kept)
	if _, statErr := os.Stat(kept); statErr != nil {
		t.Errorf("kept file missing: %v", statErr)
	}
}

// The library logs and git prints. None of it may reach the real stdout,
// which in the plugin is the terminal the UI draws on.
func TestQuietCapturesOutput(t *testing.T) {
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	s := repo(t)
	mustAdd(t, s, "silent")
	os.Stdout = stdout
	w.Close()
	var buf [512]byte
	n, _ := r.Read(buf[:])
	if n > 0 {
		t.Errorf("library wrote to stdout: %q", buf[:n])
	}
}

func TestNewOpensTheConfiguredRepository(t *testing.T) {
	s := repo(t)
	t.Setenv("DSTASK_GIT_REPO", s.Repo())
	got, err := New()
	if err != nil || got.Repo() != s.Repo() {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestNewRefusesAMissingRepository(t *testing.T) {
	t.Setenv("DSTASK_GIT_REPO", filepath.Join(t.TempDir(), "missing"))
	if _, err := New(); err == nil || !strings.Contains(err.Error(), "no dstask repository") {
		t.Fatalf("err = %v", err)
	}
}

// git explains a failure on stderr. quiet captures it, so the error names the
// cause and not only an exit status.
func TestGitFailuresCarryGitsExplanation(t *testing.T) {
	s := repo(t)
	mustAdd(t, s, "only commit")
	err := s.Undo() // HEAD~1 does not exist in a repository with one commit
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), ": ") || strings.HasSuffix(err.Error(), "exit status 128") {
		t.Errorf("error lacks git's explanation: %v", err)
	}
}

func TestEditCommandUsesTheEditor(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "pick an editor")
	t.Setenv("EDITOR", "nano -w")
	ed, err := s.EditTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer ed.Discard()
	args := ed.Command().Args
	if args[0] != "nano" || args[1] != "-w" || args[2] != ed.path {
		t.Errorf("args = %v", args)
	}

	t.Setenv("EDITOR", "")
	ed2, _ := s.EditNotes(task.ID)
	defer ed2.Discard()
	if ed2.Command().Args[0] != "vim" {
		t.Errorf("empty EDITOR should fall back to vim, as dstask does")
	}
}

func TestDiscardRemovesTheFile(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "never mind")
	ed, _ := s.EditTask(task.ID)
	ed.Discard()
	if _, err := os.Stat(ed.path); !os.IsNotExist(err) {
		t.Errorf("file still present")
	}
	if open, _ := s.Open(); open[0].Summary != "never mind" {
		t.Errorf("discard changed the task")
	}
}

func TestEditOfAnUnknownIDFails(t *testing.T) {
	s := repo(t)
	if _, err := s.EditTask(5); err == nil {
		t.Fatal("expected an error")
	}
}

// If the list changes while the editor is open, the ID can point at another
// task. Apply must refuse rather than overwrite that task.
func TestEditRefusesWhenTheIDMovedToAnotherTask(t *testing.T) {
	s := repo(t)
	task := mustAdd(t, s, "first")
	ed, _ := s.EditTask(task.ID)
	if err := s.Remove(task.ID); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, s, "second") // takes ID 1
	err := ed.Apply()
	if err == nil || !strings.Contains(err.Error(), "changed while the editor was open") {
		t.Fatalf("err = %v", err)
	}
	os.Remove(ed.path + ".kept")
	if open, _ := s.Open(); open[0].Summary != "second" {
		t.Errorf("the other task was overwritten: %+v", open[0])
	}
}
