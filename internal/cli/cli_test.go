package cli

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/cgardner/herdr-dstask/internal/state"
	"github.com/cgardner/herdr-dstask/internal/store"
	"github.com/cgardner/herdr-dstask/internal/ui"
)

// scratch makes an empty dstask repository and points the store seam at it.
// The tests never open ~/.dstask.
func scratch(t *testing.T) *store.Store {
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
	s := store.NewAt(dir)
	prev := openStore
	openStore = func() (*store.Store, error) { return s, nil }
	t.Cleanup(func() { openStore = prev })
	return s
}

func stubProgram(t *testing.T, err error) *int {
	t.Helper()
	calls := 0
	prev := runProgram
	runProgram = func(m ui.Model) (ui.Model, error) { calls++; return m, err }
	t.Cleanup(func() { runProgram = prev })
	stubState(t, state.View{})
	return &calls
}

// stubState replaces the state seams, so a test never reads or writes the
// real state folder. It returns the views that the run saved.
func stubState(t *testing.T, saved state.View) *[]state.View {
	t.Helper()
	var writes []state.View
	prevLoad, prevSave := loadState, saveState
	loadState = func(string) state.View { return saved }
	saveState = func(v state.View) error { writes = append(writes, v); return nil }
	t.Cleanup(func() { loadState, saveState = prevLoad, prevSave })
	return &writes
}

func run(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := run("--version")
	if code != 0 || strings.TrimSpace(out) != version {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	if code, _, errOut := run("--nope"); code != 2 || !strings.Contains(errOut, "nope") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestListPrintsOpenTasks(t *testing.T) {
	s := scratch(t)
	s.Add("+work P1 write the tests")
	code, out, _ := run("--list")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	for _, want := range []string{"1", "P1", "pending", "write the tests", "work"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q: %q", want, out)
		}
	}
}

func TestAllIgnoresTheContext(t *testing.T) {
	s := scratch(t)
	s.Add("+home fix the gate")
	t.Setenv("DSTASK_CONTEXT", "+work")
	withContext := store.NewAt(s.Repo())
	openStore = func() (*store.Store, error) { return withContext, nil }
	if _, out, _ := run("--list"); strings.Contains(out, "gate") {
		t.Errorf("the +work context should hide a +home task: %q", out)
	}
	if _, out, _ := run("--list", "--all"); !strings.Contains(out, "gate") {
		t.Errorf("--all should show every task: %q", out)
	}
}

func TestStoreErrorsAreReported(t *testing.T) {
	prev := openStore
	openStore = func() (*store.Store, error) { return nil, errors.New("no dstask repository") }
	t.Cleanup(func() { openStore = prev })
	if code, _, errOut := run(); code != 1 || !strings.Contains(errOut, "no dstask repository") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestListReportsALoadError(t *testing.T) {
	prev := openStore
	openStore = func() (*store.Store, error) { return store.NewAt("/nonexistent/dstask"), nil }
	t.Cleanup(func() { openStore = prev })
	// A missing status directory is not an error for dstask, so a missing
	// repository lists nothing rather than failing.
	if code, out, _ := run("--list"); code != 0 || out != "" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestRunStartsTheUI(t *testing.T) {
	scratch(t)
	calls := stubProgram(t, nil)
	if code, _, _ := run(); code != 0 || *calls != 1 {
		t.Errorf("code=%d calls=%d", code, *calls)
	}
}

func TestUIErrorsAreReported(t *testing.T) {
	scratch(t)
	stubProgram(t, errors.New("no terminal"))
	if code, _, errOut := run(); code != 1 || !strings.Contains(errOut, "no terminal") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestBackendAdaptsTheEditors(t *testing.T) {
	s := scratch(t)
	s.Add("edit me later")
	b := backend{s}
	for _, open := range []func(int) (ui.Editor, error){b.EditTask, b.EditNotes} {
		ed, err := open(1)
		if err != nil || ed == nil || ed.Command() == nil {
			t.Fatalf("ed=%v err=%v", ed, err)
		}
		ed.Discard()
	}
	if ed, err := b.EditTask(99); err == nil || ed != nil {
		t.Errorf("an unknown id should fail with no editor, got %v", ed)
	}
}

func TestTheViewIsRestoredAndSaved(t *testing.T) {
	s := scratch(t)
	var started ui.Model
	prev := runProgram
	runProgram = func(m ui.Model) (ui.Model, error) { started = m; return m, nil }
	t.Cleanup(func() { runProgram = prev })
	writes := stubState(t, state.View{View: "projects", Filter: "project:atlas", ProjectSort: "open"})

	if code, _, _ := run(); code != 0 {
		t.Fatalf("code = %d", code)
	}
	got := started.State()
	if got.View != "projects" || got.Filter != "project:atlas" || got.ProjectSort != "open" {
		t.Errorf("the UI did not start from the saved view: %+v", got)
	}
	if len(*writes) != 1 {
		t.Fatalf("want one save, got %d", len(*writes))
	}
	w := (*writes)[0]
	if w.Repo != s.Repo() || w.View != "projects" || w.Filter != "project:atlas" {
		t.Errorf("saved %+v", w)
	}
}

func TestTheAllFlagWinsOverTheSavedView(t *testing.T) {
	scratch(t)
	var started ui.Model
	prev := runProgram
	runProgram = func(m ui.Model) (ui.Model, error) { started = m; return m, nil }
	t.Cleanup(func() { runProgram = prev })
	stubState(t, state.View{IgnoreContext: false})
	run("--all")
	if !started.State().IgnoreContext {
		t.Errorf("--all should ignore the context even when the saved view did not")
	}
}

func TestNothingIsSavedWhenTheUIFails(t *testing.T) {
	scratch(t)
	prev := runProgram
	runProgram = func(m ui.Model) (ui.Model, error) { return m, errors.New("no terminal") }
	t.Cleanup(func() { runProgram = prev })
	writes := stubState(t, state.View{})
	run()
	if len(*writes) != 0 {
		t.Errorf("a failed run must not save a view")
	}
}
