package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/naggie/dstask"
)

var testNow = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

func TestHelpOpensAndAnyKeyCloses(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("?"))
	if m.mode != modeHelp || !strings.Contains(m.View(), "dstask keys") {
		t.Fatalf("mode=%v", m.mode)
	}
	for _, want := range []string{"show only active tasks", "undo the last change"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("help lacks %q", want)
		}
	}
	m = drive(t, m, key("z"))
	if m.mode != modeList {
		t.Errorf("any key should close the help")
	}
	// Help from the detail view returns to the detail view.
	m = drive(t, m, key("enter"), key("?"), key("z"))
	if m.mode != modeDetail {
		t.Errorf("help should return to the detail view, got %v", m.mode)
	}
}

func TestPromptLineNamesTheTarget(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	for k, want := range map[string]string{"m": "modify #3", "n": "note #3", "a": "add", "/": "filter"} {
		mm := drive(t, m, key(k))
		if !strings.Contains(mm.View(), want) {
			t.Errorf("%s: prompt lacks %q", k, want)
		}
	}
}

func TestAddPromptSendsInput(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	drive(t, m, append(append([]tea.Msg{key("a")}, typed("+work P1 write docs")...), key("enter"))...)
	if len(f.calls) != 1 || f.calls[0] != "add +work P1 write docs" {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestNotePromptSendsInput(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	drive(t, m, append(append([]tea.Msg{key("n")}, typed("5 minutes")...), key("enter"))...)
	if len(f.calls) != 1 || f.calls[0] != "note 3 5 minutes" {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestUndoFromListAndDetail(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("u"), key("enter"), key("u"))
	if strings.Join(f.calls, ",") != "undo,undo" {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestDetailKeys(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("enter"))
	for _, k := range []string{"j", "k", "G", "g", "ctrl+d"} {
		m = drive(t, m, key(k))
		if m.mode != modeDetail {
			t.Fatalf("%s left the detail view", k)
		}
	}
	m = drive(t, m, key("a"))
	if m.mode != modePrompt || m.back != modeDetail {
		t.Fatalf("a in the detail view should open the add prompt")
	}
	if !strings.Contains(m.View(), "#3 write the plugin") {
		t.Errorf("the detail view should stay behind the prompt")
	}
	m = drive(t, m, key("esc"), key("h"))
	if m.mode != modeList {
		t.Errorf("h should return to the list")
	}
	if _, cmd := drive(t, m, key("enter")).Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Errorf("ctrl+c in the detail view should quit")
	}
}

func TestRemoveFromDetailReturnsToTheList(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("enter"), key("x"))
	if !strings.Contains(m.View(), "remove #3") {
		t.Fatalf("confirm line missing")
	}
	m = drive(t, m, key("y"))
	if m.mode != modeList || len(f.calls) != 1 || f.calls[0] != "remove 3" {
		t.Errorf("mode=%v calls=%v", m.mode, f.calls)
	}
}

func TestDetailClosesWhenTheTaskGoes(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("enter"))
	f.open = tasks()[1:]
	m = drive(t, m, key("r"))
	if m.mode != modeList {
		t.Errorf("the detail view of a removed task should close")
	}
}

func TestDetailFollowsAChangedTask(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("enter"))
	f.open = tasks()
	f.open[0].Summary = "renamed"
	m = drive(t, m, key("d"))
	if m.mode != modeDetail || !strings.Contains(m.View(), "renamed") {
		t.Errorf("the detail view should show the reloaded task")
	}
}

func TestDetailBodyShowsEveryField(t *testing.T) {
	m := start(t, &fake{})
	m.now = func() time.Time { return testNow }
	task := dstask.Task{
		UUID: "u-1", ID: 0, Summary: "done thing", Status: dstask.STATUS_RESOLVED,
		Priority: dstask.PRIORITY_CRITICAL, Project: "p", Tags: []string{"a", "b"},
		Created:  testNow.Add(-72 * time.Hour),
		Resolved: testNow.Add(-5 * time.Hour),
		Due:      testNow.Add(-24 * time.Hour),
		Notes:    "line one\nline two",
	}
	body := m.detailBody(task)
	for _, want := range []string{"resolved", "P0", "+a", "+b", "due", "3d ago", "5h ago", "u-1", "line two"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
	m.target = task
	m.mode = modeDetail
	m.refreshDetail()
	if v := m.View(); !strings.Contains(v, " done thing") || strings.Contains(v, "#0") {
		t.Errorf("a resolved task has no id to show: %q", v[:40])
	}
}

func TestAgeUnits(t *testing.T) {
	for d, want := range map[time.Duration]string{
		10 * time.Minute: "10m ago",
		30 * time.Hour:   "30h ago",
		96 * time.Hour:   "4d ago",
	} {
		if got := age(testNow.Add(-d), testNow); !strings.Contains(got, want) {
			t.Errorf("age(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestDueIsRedOnlyWhenLateAndOpen(t *testing.T) {
	m := start(t, &fake{})
	m.now = func() time.Time { return testNow }
	late := dstask.Task{Status: "pending", Due: testNow.Add(-time.Hour)}
	if _, st := m.due(late); st.GetForeground() != styleOverdue.GetForeground() {
		t.Errorf("a late open task should be red")
	}
	late.Status = dstask.STATUS_RESOLVED
	if _, st := m.due(late); st.GetForeground() == styleOverdue.GetForeground() {
		t.Errorf("a resolved task is never late")
	}
	if label, _ := m.due(dstask.Task{}); label != "" {
		t.Errorf("no due date, no label")
	}
}

func TestStatusMarks(t *testing.T) {
	for status, want := range map[string]string{
		dstask.STATUS_ACTIVE: "▶", dstask.STATUS_PAUSED: "‖", dstask.STATUS_RESOLVED: "✓",
		dstask.STATUS_DELEGATED: "→", dstask.STATUS_DEFERRED: "z", dstask.STATUS_PENDING: " ",
	} {
		if got, _ := statusMark(status); got != want {
			t.Errorf("%s: %q, want %q", status, got, want)
		}
	}
}

func TestStylesFollowPriorityAndStatus(t *testing.T) {
	cases := []struct {
		task dstask.Task
		want lipgloss.Style
	}{
		{dstask.Task{Status: "active"}, styleActive},
		{dstask.Task{Status: "resolved"}, styleDim},
		{dstask.Task{Status: "pending", Priority: "P3"}, styleLow},
	}
	for _, c := range cases {
		if summaryStyle(c.task).GetForeground() != c.want.GetForeground() {
			t.Errorf("%+v: wrong summary style", c.task)
		}
	}
	for p, want := range map[string]lipgloss.Style{"P0": styleCritical, "P1": styleHigh, "P3": styleLow} {
		if priorityStyle(p).GetForeground() != want.GetForeground() {
			t.Errorf("%s: wrong priority style", p)
		}
	}
}

func TestLongListScrolls(t *testing.T) {
	var many []dstask.Task
	for i := 1; i <= 40; i++ {
		many = append(many, dstask.Task{UUID: fmt.Sprint(i), ID: i, Summary: fmt.Sprintf("task %02d", i), Status: "pending", Priority: "P2"})
	}
	m := start(t, &fake{open: many})
	m = drive(t, m, key("G"))
	if m.cursor != 39 || !strings.Contains(m.View(), "task 40") || strings.Contains(m.View(), "task 01") {
		t.Fatalf("G should scroll to the end: cursor=%d offset=%d", m.cursor, m.offset)
	}
	m = drive(t, m, key("ctrl+u"), key("g"))
	if m.cursor != 0 || m.offset != 0 {
		t.Errorf("g should return to the top: cursor=%d offset=%d", m.cursor, m.offset)
	}
	m = drive(t, m, key("ctrl+d"))
	if m.cursor != m.listRows()/2 {
		t.Errorf("ctrl+d moved to %d", m.cursor)
	}
	m = drive(t, m, key("k"), key("k"), key("k"), key("k"), key("k"), key("k"), key("k"), key("k"), key("k"))
	if m.cursor != 0 {
		t.Errorf("k should stop at the top, got %d", m.cursor)
	}
}

func TestQuitKeys(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	for _, msg := range []tea.KeyMsg{key("q"), key("esc"), {Type: tea.KeyCtrlC}} {
		if _, cmd := m.Update(msg); cmd == nil || cmd() != tea.Quit() {
			t.Errorf("%v should quit", msg)
		}
	}
}

func TestLoadErrorIsShown(t *testing.T) {
	m := start(t, &fake{})
	next, _ := m.Update(loadedMsg{err: errors.New("cannot read repo")})
	if v := next.(Model).View(); !strings.Contains(v, "cannot read repo") {
		t.Errorf("load error not shown")
	}
}

func TestLoadingAndEmptyMessages(t *testing.T) {
	m := New(&fake{})
	if !strings.Contains(m.View(), "loading") {
		t.Errorf("a new model should say it is loading")
	}
	m = start(t, &fake{})
	if !strings.Contains(m.View(), "no tasks") {
		t.Errorf("an empty list should say so")
	}
}

func TestEditorSavesOnCleanExit(t *testing.T) {
	ed := &fakeEditor{}
	f := &fake{open: tasks(), ed: ed}
	m := start(t, f)
	for _, k := range []string{"e", "N"} {
		if _, cmd := m.Update(key(k)); cmd == nil {
			t.Fatalf("%s should hand the terminal to the editor", k)
		}
	}
	if strings.Join(f.calls, ",") != "edit 3,edit-notes 3" {
		t.Errorf("calls = %v", f.calls)
	}

	msg := editorDone(ed, "edited #3")(nil).(actionMsg)
	if !ed.applied || msg.err != nil || msg.ok != "edited #3" {
		t.Errorf("clean exit should apply: %+v", msg)
	}
}

func TestEditorFailureDiscards(t *testing.T) {
	ed := &fakeEditor{}
	msg := editorDone(ed, "edited")(errors.New("exit status 1")).(actionMsg)
	if !ed.discarded || ed.applied || msg.err == nil || !strings.Contains(msg.err.Error(), "nothing saved") {
		t.Errorf("a failed editor must discard: %+v", msg)
	}
}

func TestFitLeavesAnUnsizedLineAlone(t *testing.T) {
	if fit("abc", 0) != "abc" || fill("abc", 0, lipgloss.NewStyle()) != "abc" {
		t.Errorf("a zero width means no limit")
	}
}
