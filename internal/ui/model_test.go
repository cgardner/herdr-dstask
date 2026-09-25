package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/naggie/dstask"
)

// fake records every change the UI asks for.
type fake struct {
	open, resolved []dstask.Task
	calls          []string
	ignore         bool
	fail           error
	ed             *fakeEditor
}

func (f *fake) Open() ([]dstask.Task, error)     { return f.open, nil }
func (f *fake) Resolved() ([]dstask.Task, error) { return f.resolved, nil }
func (f *fake) ContextString() string            { return "+work" }
func (f *fake) SetIgnoreContext(v bool)          { f.ignore = v }
func (f *fake) record(s string) error            { f.calls = append(f.calls, s); return f.fail }
func (f *fake) Done(id int) error                { return f.record(fmt.Sprint("done ", id)) }
func (f *fake) Start(id int) error               { return f.record(fmt.Sprint("start ", id)) }
func (f *fake) Stop(id int) error                { return f.record(fmt.Sprint("stop ", id)) }
func (f *fake) Remove(id int) error              { return f.record(fmt.Sprint("remove ", id)) }
func (f *fake) Undo() error                      { return f.record("undo") }
func (f *fake) Add(in string) error              { return f.record("add " + in) }
func (f *fake) Modify(id int, s string) error    { return f.record(fmt.Sprintf("modify %d %s", id, s)) }
func (f *fake) Note(id int, s string) error      { return f.record(fmt.Sprintf("note %d %s", id, s)) }
func (f *fake) EditTask(id int) (Editor, error)  { return f.editor(fmt.Sprint("edit ", id)) }
func (f *fake) EditNotes(id int) (Editor, error) { return f.editor(fmt.Sprint("edit-notes ", id)) }

func (f *fake) editor(call string) (Editor, error) {
	if f.ed == nil {
		return nil, errors.New("no editor in tests")
	}
	f.calls = append(f.calls, call)
	return f.ed, nil
}

// fakeEditor records whether the UI saved or dropped the edit.
type fakeEditor struct {
	applied, discarded bool
	applyErr           error
}

func (e *fakeEditor) Command() *exec.Cmd { return exec.Command("true") }
func (e *fakeEditor) Apply() error       { e.applied = true; return e.applyErr }
func (e *fakeEditor) Discard()           { e.discarded = true }

func tasks() []dstask.Task {
	return []dstask.Task{
		{UUID: "a", ID: 3, Summary: "write the plugin", Project: "herdr", Priority: "P1", Status: "pending", Tags: []string{"work"}},
		{UUID: "b", ID: 7, Summary: "review the docs", Priority: "P2", Status: "active", Notes: "check the manifest"},
		{UUID: "c", ID: 9, Summary: "tidy the garage", Priority: "P3", Status: "paused", Tags: []string{"home"}},
	}
}

// drive feeds messages through Update and runs every command it returns, so
// background loads and actions complete before the test looks.
func drive(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		var cmd tea.Cmd
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
		m = settle(t, m, cmd, 0)
	}
	return m
}

func settle(t *testing.T, m Model, cmd tea.Cmd, depth int) Model {
	if cmd == nil || depth > 5 {
		return m
	}
	// A blink tick from the text input waits half a second before it returns,
	// and a test needs none of them.
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(50 * time.Millisecond):
		return m
	}
	switch msg.(type) {
	case loadedMsg, actionMsg:
		next, more := m.Update(msg)
		return settle(t, next.(Model), more, depth+1)
	}
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typed(s string) []tea.Msg {
	var msgs []tea.Msg
	for _, r := range s {
		msgs = append(msgs, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return msgs
}

func start(t *testing.T, f *fake) Model {
	t.Helper()
	m := New(f)
	m.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) }
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	return settle(t, m, m.Init(), 0)
}

func TestLoadListsTasksAndContext(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	if len(m.visible) != 3 || m.context != "+work" {
		t.Fatalf("visible=%d context=%q", len(m.visible), m.context)
	}
	view := m.View()
	for _, want := range []string{"write the plugin", "review the docs", "+work", "open 3"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}
}

func TestActionsTargetTheSelectedTask(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("j"), key("s"), key("j"), key("s"), key("d"))
	want := []string{"stop 7", "start 9", "done 9"}
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
}

func TestFilterNarrowsAsYouType(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, append([]tea.Msg{key("/")}, typed("manifest")...)...)
	if len(m.visible) != 1 || m.visible[0].ID != 7 {
		t.Fatalf("filter on notes failed: %+v", m.visible)
	}
	m = drive(t, m, key("enter"))
	if m.mode != modeList || m.filter != "manifest" {
		t.Fatalf("mode=%v filter=%q", m.mode, m.filter)
	}
	m = drive(t, m, key("esc"))
	if m.filter != "" || len(m.visible) != 3 {
		t.Errorf("esc should clear the filter")
	}
}

func TestFilterMatchesTagsWithPlus(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, append([]tea.Msg{key("/")}, typed("+home")...)...)
	if len(m.visible) != 1 || m.visible[0].ID != 9 {
		t.Errorf("tag filter failed: %+v", m.visible)
	}
}

func TestModifyPromptSendsInput(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	msgs := append([]tea.Msg{key("m")}, typed("+urgent P0")...)
	m = drive(t, m, append(msgs, key("enter"))...)
	if len(f.calls) != 1 || f.calls[0] != "modify 3 +urgent P0" {
		t.Errorf("calls = %v", f.calls)
	}
	if !strings.Contains(m.View(), "modified #3") {
		t.Errorf("status line missing from view")
	}
}

func TestPromptEscCancels(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, append(append([]tea.Msg{key("n")}, typed("hello")...), key("esc"))...)
	if len(f.calls) != 0 || m.mode != modeList {
		t.Errorf("esc should cancel: calls=%v mode=%v", f.calls, m.mode)
	}
}

func TestRemoveNeedsConfirmation(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("x"), key("n"))
	if len(f.calls) != 0 {
		t.Fatalf("removed without a yes: %v", f.calls)
	}
	m = drive(t, m, key("x"), key("y"))
	if len(f.calls) != 1 || f.calls[0] != "remove 3" {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestResolvedTasksRefuseChanges(t *testing.T) {
	f := &fake{open: tasks(), resolved: []dstask.Task{{UUID: "z", Summary: "old", Status: "resolved"}}}
	m := start(t, f)
	m = drive(t, m, key("tab"))
	if !m.showResolved || len(m.visible) != 1 {
		t.Fatalf("tab should show resolved tasks")
	}
	m = drive(t, m, key("d"))
	if len(f.calls) != 0 || !m.statusErr {
		t.Errorf("a resolved task must not be changed: calls=%v", f.calls)
	}
}

func TestContextToggle(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("c"))
	if !f.ignore || !strings.Contains(m.View(), "context ignored") {
		t.Errorf("c should ignore the context")
	}
}

func TestErrorsReachTheStatusLine(t *testing.T) {
	f := &fake{open: tasks(), fail: errors.New("refusing to resolve task with incomplete tasklist")}
	m := start(t, f)
	m = drive(t, m, key("d"))
	if !m.statusErr || !strings.Contains(m.View(), "incomplete tasklist") {
		t.Errorf("error not shown: %q", m.status)
	}
}

func TestReloadKeepsTheSelection(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("j"), key("j")) // on #9
	f.open = []dstask.Task{tasks()[2], tasks()[0]}
	m = drive(t, m, key("r"))
	if got, _ := m.selected(); got.ID != 9 {
		t.Errorf("selection moved to #%d", got.ID)
	}
}

func TestDetailShowsNotesAndActsOnTheTask(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("j"), key("enter"))
	if m.mode != modeDetail {
		t.Fatal("enter should open the detail view")
	}
	view := m.View()
	for _, want := range []string{"#7 review the docs", "check the manifest", "active"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail lacks %q", want)
		}
	}
	m = drive(t, m, key("s"))
	if len(f.calls) != 1 || f.calls[0] != "stop 7" {
		t.Errorf("calls = %v", f.calls)
	}
	m = drive(t, m, key("esc"))
	if m.mode != modeList {
		t.Errorf("esc should return to the list")
	}
}

func TestEditorErrorsAreReported(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("e"))
	if !m.statusErr {
		t.Errorf("an editor that cannot open must be reported")
	}
}

func TestNarrowPaneDoesNotOverflow(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, tea.WindowSizeMsg{Width: 30, Height: 8})
	for i, line := range strings.Split(m.View(), "\n") {
		if w := len([]rune(stripSGR(line))); w > 30 {
			t.Errorf("line %d is %d wide: %q", i, w, line)
		}
	}
}

func stripSGR(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			in = true
		case in && (r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'):
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestActiveFilterShowsOnlyStartedTasks(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("A"))
	if len(m.visible) != 1 || m.visible[0].ID != 7 {
		t.Fatalf("visible = %+v, want only #7", m.visible)
	}
	if !strings.Contains(m.View(), "active only") {
		t.Errorf("header should name the active filter")
	}
	m = drive(t, m, key("A"))
	if m.activeOnly || len(m.visible) != 3 {
		t.Errorf("a second A should show every task")
	}
}

func TestActiveFilterCombinesWithTextFilter(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("A"), key("/"))
	m = drive(t, m, append(typed("garage"), key("enter"))...)
	if len(m.visible) != 0 || !strings.Contains(m.View(), "no task matches") {
		t.Fatalf("paused #9 must stay hidden: %+v", m.visible)
	}
	// esc clears the text filter first and keeps the active filter.
	m = drive(t, m, key("esc"))
	if m.filter != "" || !m.activeOnly || len(m.visible) != 1 {
		t.Fatalf("filter=%q activeOnly=%v visible=%d", m.filter, m.activeOnly, len(m.visible))
	}
	m = drive(t, m, key("esc"))
	if m.activeOnly || len(m.visible) != 3 {
		t.Errorf("the second esc should clear the active filter")
	}
}

func TestActiveFilterSurvivesReload(t *testing.T) {
	f := &fake{open: tasks()}
	m := start(t, f)
	m = drive(t, m, key("A"))
	f.open = append(tasks(), dstask.Task{UUID: "d", ID: 11, Summary: "new work", Status: "active"})
	m = drive(t, m, key("r"))
	if len(m.visible) != 2 {
		t.Errorf("after reload, visible = %d, want the 2 active tasks", len(m.visible))
	}
}

func TestActiveFilterEmptyMessage(t *testing.T) {
	open := tasks()
	open[1].Status = "pending"
	m := start(t, &fake{open: open})
	m = drive(t, m, key("A"))
	if !strings.Contains(m.View(), "no active tasks") {
		t.Errorf("empty active list should say so")
	}
}

func TestActiveFilterIsRefusedForResolvedTasks(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("A"), key("tab"))
	if m.activeOnly {
		t.Fatal("tab should clear the active filter")
	}
	m = drive(t, m, key("A"))
	if m.activeOnly || !m.statusErr {
		t.Errorf("A in the resolved list should be refused")
	}
}

func TestFilterByID(t *testing.T) {
	open := append(tasks(), dstask.Task{UUID: "t", ID: 72, Summary: "fix SAL-3 and 9 others", Status: "pending", Priority: "P2"})
	cases := []struct {
		filter string
		want   []int
	}{
		{"#7", []int{7}},
		{"#72", []int{72}},
		{"#3 #9", []int{3, 9}},     // several IDs match any of them
		{"#3 #9 garage", []int{9}}, // text words narrow the IDs
		{"#999", nil},              // no such task
		{"9", []int{72}},           // a bare number is a text search
		{"#", []int{3, 7, 9, 72}},  // a lone # hides nothing
		{"#abc", nil},              // not an id, so a text word that matches nothing
		{"#0", nil},                // resolved tasks have no id to find
	}
	for _, c := range cases {
		var got []int
		for _, task := range open {
			if matches(task, c.filter) {
				got = append(got, task.ID)
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%q: got %v, want %v", c.filter, got, c.want)
		}
	}
}

func TestHashKeyOpensTheIDSearch(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("#"))
	if m.mode != modePrompt || m.prompt != promptFilter || m.input.Value() != "#" {
		t.Fatalf("mode=%v value=%q", m.mode, m.input.Value())
	}
	if len(m.visible) != 3 {
		t.Errorf("a lone # should not hide tasks while typing")
	}
	m = drive(t, m, append(typed("9"), key("enter"))...)
	if len(m.visible) != 1 || m.visible[0].ID != 9 || m.filter != "#9" {
		t.Errorf("visible=%+v filter=%q", m.visible, m.filter)
	}
	if got, _ := m.selected(); got.ID != 9 {
		t.Errorf("the found task should be selected, got #%d", got.ID)
	}
}
