package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/naggie/dstask"
)

func space() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}} }

func TestSpaceMarksAndMovesDown(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, space(), space())
	if !m.marked["a"] || !m.marked["b"] || len(m.marked) != 2 {
		t.Fatalf("marked = %v", m.marked)
	}
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 after two marks", m.cursor)
	}
	view := m.View()
	if !strings.Contains(view, "2 marked") || strings.Count(view, "✓") != 2 {
		t.Errorf("view should show two marks:\n%s", view)
	}
	// On the last row, space marks and stays.
	m = drive(t, m, space(), space())
	if m.cursor != 2 || m.marked["c"] {
		t.Errorf("space on the last row should toggle in place: cursor=%d marked=%v", m.cursor, m.marked)
	}
	// space again unmarks.
	m = drive(t, m, key("k"), space())
	if m.marked["b"] {
		t.Errorf("a second space should unmark")
	}
}

func TestBulkChangesActOnEveryMarkedTask(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, space(), key("j"), space()) // mark #3 and #9
	m = drive(t, m, key("d"))
	if len(f.calls) != 1 || f.calls[0] != "done 3,9" {
		t.Errorf("calls = %v", f.calls)
	}
	if len(m.marked) != 0 || !strings.Contains(m.View(), "resolved 2 tasks") {
		t.Errorf("a successful bulk change should clear the marks and report: %v", m.marked)
	}
}

func TestBulkModifyAndNoteUseOnePrompt(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("*"))
	m = drive(t, m, key("m"))
	if !strings.Contains(m.View(), "modify 3 tasks") {
		t.Errorf("the prompt should name the count:\n%s", m.View())
	}
	m = drive(t, m, append(typed("+sprint"), key("enter"))...)
	m = drive(t, m, key("*"), key("n"))
	m = drive(t, m, append(typed("same line"), key("enter"))...)
	want := "modify 3,7,9 +sprint|note 3,7,9 same line"
	if got := strings.Join(f.calls, "|"); got != want {
		t.Errorf("calls = %s", got)
	}
}

func TestBulkStartStartsTheIdleAndStopsWhenAllActive(t *testing.T) {
	f := &fake{open: tasks()} // #7 is active, #3 and #9 are not
	m := start(t, f)
	m = drive(t, m, key("*"), key("s"))
	m = drive(t, m, key("j"), space(), key("s")) // only #7, which is active
	want := "start 3,9|stop 7"
	if got := strings.Join(f.calls, "|"); got != want {
		t.Errorf("calls = %s, want %s", got, want)
	}
}

func TestBulkRemoveAsksWithTheCount(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, space(), space(), key("x"))
	if !strings.Contains(m.View(), "remove 2 tasks?") {
		t.Fatalf("confirm should name the count:\n%s", m.View())
	}
	m = drive(t, m, key("n"))
	if len(f.calls) != 0 || len(m.marked) != 2 || !strings.Contains(m.View(), "kept 2 tasks") {
		t.Errorf("no should keep the tasks and the marks: calls=%v marked=%d", f.calls, len(m.marked))
	}
	drive(t, m, key("x"), key("y"))
	if len(f.calls) != 1 || f.calls[0] != "remove 3,7" {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestOneMarkedTaskIsNamedByID(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("j"), key("j"), space(), key("g"), key("x"))
	// The cursor is on #3, but the mark is on #9, and the mark wins.
	if !strings.Contains(m.View(), "remove #9?") {
		t.Errorf("confirm = %q", m.footer())
	}
}

func TestFailedBulkChangeKeepsTheMarks(t *testing.T) {
	f := &fake{open: tasks(), fail: errFake}
	m := start(t, f)
	m = drive(t, m, key("*"), key("d"))
	if len(m.marked) != 3 || !m.statusErr {
		t.Errorf("a failed change should keep the marks for another try: %d", len(m.marked))
	}
}

func TestEscClearsMarksBeforeFilters(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("A"), key("*"))
	m = drive(t, m, key("esc"))
	if len(m.marked) != 0 || !m.activeOnly {
		t.Errorf("first esc clears the marks only: marked=%d activeOnly=%v", len(m.marked), m.activeOnly)
	}
}

func TestStarTogglesTheVisibleTasksOnly(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, space())  // mark #3
	m = drive(t, m, key("A")) // only #7 is visible
	m = drive(t, m, key("*"))
	if !m.marked["a"] || !m.marked["b"] || len(m.marked) != 2 {
		t.Fatalf("* should mark the visible #7 and keep #3: %v", m.marked)
	}
	if !strings.Contains(m.View(), "2 marked (1 hidden by the filter)") {
		t.Errorf("header should warn about the hidden mark:\n%s", m.View())
	}
	m = drive(t, m, key("*"))
	if !m.marked["a"] || m.marked["b"] {
		t.Errorf("a second * unmarks the visible tasks only: %v", m.marked)
	}
}

func TestMarksOnTasksThatLeaveTheListAreDropped(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("*"))
	f.open = tasks()[1:] // #3 was resolved elsewhere
	m = drive(t, m, key("r"))
	if m.marked["a"] || len(m.marked) != 2 {
		t.Errorf("marked = %v", m.marked)
	}
}

func TestResolvedTasksCannotBeMarked(t *testing.T) {
	f := &fake{open: tasks(), resolved: []dstask.Task{{UUID: "z", Summary: "old", Status: "resolved"}}}
	m := start(t, f)
	m = drive(t, m, space(), key("tab"))
	if len(m.marked) != 0 {
		t.Errorf("tab should clear the marks")
	}
	for _, k := range []tea.KeyMsg{space(), key("*")} {
		if mm := drive(t, m, k); len(mm.marked) != 0 || !mm.statusErr {
			t.Errorf("%v in the resolved list should refuse", k)
		}
	}
}

func TestEditorKeysStaySingleTask(t *testing.T) {
	f := &fake{open: tasks(), ed: &fakeEditor{}}
	m := start(t, f)
	m = drive(t, m, key("*"))
	for _, k := range []string{"e", "N"} {
		if mm := drive(t, m, key(k)); !mm.statusErr || len(f.calls) != 0 {
			t.Errorf("%s with marks should explain, not edit: calls=%v", k, f.calls)
		}
	}
}

// The task view is about one task, so a change there ignores the marks.
func TestTaskViewChangesIgnoreMarks(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("*"), key("enter"), key("d"))
	if len(f.calls) != 1 || f.calls[0] != "done 3" {
		t.Errorf("calls = %v", f.calls)
	}
	if len(m.marked) != 3 {
		t.Errorf("a single change should keep the marks")
	}
}

func TestMarksAreNotSaved(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("*"))
	if v := m.State(); v.Task == "" {
		t.Fatalf("state should still name the selected task")
	}
	r := New(&fake{open: tasks()}).Restore(m.State())
	if len(r.marked) != 0 {
		t.Errorf("marks are for one session only")
	}
}

var errFake = errorString("refusing to resolve task with incomplete tasklist")

type errorString string

func (e errorString) Error() string { return string(e) }
