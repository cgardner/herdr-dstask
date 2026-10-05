package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cgardner/herdr-dstask/internal/state"
)

func TestStateRoundTrip(t *testing.T) {
	f := &fake{open: tasks(), projects: sampleProjects()}
	m := start(t, f)
	m = drive(t, m, key("j"), key("A"), key("c")) // select #7, active only, ignore context
	m = drive(t, m, key("p"), key("s"), key("tab"), key("j"))
	v := m.State()
	want := state.View{
		View: "projects", ActiveOnly: true, IgnoreContext: true, Task: "b",
		ProjectSort: "progress", ShowFinished: true, Project: v.Project,
	}
	if v != want || v.Project == "" {
		t.Fatalf("state = %+v", v)
	}

	// A new popup restores the view, the filters and both selections.
	f2 := &fake{open: tasks(), projects: sampleProjects()}
	r := New(f2).Restore(v)
	r = drive(t, r, windowSize())
	r = settle(t, r, r.Init(), 0) // a batch of both loads
	if r.mode != modeProjects || r.psort != sortProgress || !r.showFinished || !f2.ignore {
		t.Errorf("restored mode=%v sort=%v finished=%v ignore=%v", r.mode, r.psort, r.showFinished, f2.ignore)
	}
	if p, _ := r.selectedProject(); p.Name != v.Project {
		t.Errorf("project selection = %q, want %q", p.Name, v.Project)
	}
	r = drive(t, r, key("p"))
	if got, _ := r.selected(); got.UUID != "b" || !r.activeOnly {
		t.Errorf("task selection = %q activeOnly=%v", got.UUID, r.activeOnly)
	}
}

func TestRestoreSelectsTheSavedTaskByUUID(t *testing.T) {
	r := New(&fake{open: tasks()}).Restore(state.View{View: "list", Task: "c"})
	r = drive(t, r, windowSize())
	r = settle(t, r, r.Init(), 0)
	if got, _ := r.selected(); got.ID != 9 {
		t.Errorf("selected #%d, want #9", got.ID)
	}
	// A task that is gone leaves the cursor at the top.
	r = New(&fake{open: tasks()}).Restore(state.View{Task: "gone"})
	r = settle(t, drive(t, r, windowSize()), r.Init(), 0)
	if r.cursor != 0 {
		t.Errorf("cursor = %d", r.cursor)
	}
}

func TestRestoreIgnoresWhatCannotBeTrue(t *testing.T) {
	r := New(&fake{}).Restore(state.View{ShowResolved: true, ActiveOnly: true, ProjectSort: "nonsense"})
	if r.activeOnly {
		t.Errorf("the resolved list has no active tasks, so active-only must not restore with it")
	}
	if r.psort != sortUrgency {
		t.Errorf("an unknown sort should fall back to urgency, got %v", r.psort)
	}
}

// A task view, a prompt or the help screen is a short step: the state names
// the list behind it.
func TestStateNamesTheListBehindAShortStep(t *testing.T) {
	f := &fake{open: tasks(), projects: sampleProjects()}
	m := start(t, f)
	for _, c := range []struct {
		keys []string
		view string
	}{
		{[]string{"enter"}, "list"},
		{[]string{"?"}, "list"},
		{[]string{"/"}, "list"},
		{[]string{"p", "?"}, "projects"},
		{[]string{"p", "/"}, "projects"},
	} {
		mm := m
		for _, k := range c.keys {
			mm = drive(t, mm, key(k))
		}
		if got := mm.State().View; got != c.view {
			t.Errorf("%v: view %q, want %q", c.keys, got, c.view)
		}
	}
}

// Before the first load arrives, the state keeps the restored selection, so
// a popup closed at once does not forget it.
func TestStateKeepsASelectionThatHasNotLoaded(t *testing.T) {
	r := New(&fake{}).Restore(state.View{View: "projects", Task: "t", Project: "p"})
	if v := r.State(); v.Task != "t" || v.Project != "p" {
		t.Errorf("state = %+v", v)
	}
}

func windowSize() tea.Msg { return tea.WindowSizeMsg{Width: 100, Height: 20} }
