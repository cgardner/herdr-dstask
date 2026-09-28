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

	"github.com/cgardner/herdr-dstask/internal/store"
)

func project(name, priority string, tasks, resolved int) store.Project {
	return store.Project{Project: dstask.Project{Name: name, Priority: priority, Tasks: tasks, TasksResolved: resolved}}
}

func sampleProjects() []store.Project {
	late := project("harbor", "P1", 4, 1)
	late.OverdueTasks = 1
	busy := project("lighthouse", "P0", 10, 4)
	busy.ActiveTasks, busy.PausedTasks = 1, 1
	busy.Resolved = testNow.Add(-3 * 24 * time.Hour)
	old := project("archive", "P3", 3, 3)
	old.Resolved = testNow.Add(-90 * 24 * time.Hour)
	recent := project("launch", "P3", 2, 2)
	recent.Resolved = testNow.Add(-time.Hour)
	return []store.Project{
		old, project("atlas", "P2", 5, 0), late, busy, recent,
	}
}

func startProjects(t *testing.T, f *fake) Model {
	t.Helper()
	m := start(t, f)
	m.now = func() time.Time { return testNow }
	return drive(t, m, key("p"))
}

func names(ps []store.Project) string {
	var n []string
	for _, p := range ps {
		n = append(n, p.Name)
	}
	return strings.Join(n, ",")
}

func TestProjectsOpenSortedByUrgency(t *testing.T) {
	m := startProjects(t, &fake{open: tasks(), projects: sampleProjects()})
	if m.mode != modeProjects {
		t.Fatal("p should open the project view")
	}
	// Open projects by highest priority, then name. Finished ones hidden.
	if got := names(m.projects); got != "lighthouse,harbor,atlas" {
		t.Errorf("order = %s", got)
	}
	view := m.View()
	for _, want := range []string{"projects 3", "2 finished hidden", "LAST DONE", "40%", "4/10", "6 open", "▶ 1", "‖ 1", "1 late", "3d ago"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}
}

func TestTabShowsFinishedProjectsLast(t *testing.T) {
	m := startProjects(t, &fake{open: tasks(), projects: sampleProjects()})
	m = drive(t, m, key("tab"))
	// Finished projects follow, most recently finished first.
	if got := names(m.projects); got != "lighthouse,harbor,atlas,launch,archive" {
		t.Errorf("order = %s", got)
	}
	view := m.View()
	if !strings.Contains(view, "✓ finished") || !strings.Contains(view, "Jun 2026") || strings.Contains(view, "hidden") {
		t.Errorf("finished projects not shown right:\n%s", view)
	}
	m = drive(t, m, key("tab"))
	if len(m.projects) != 3 {
		t.Errorf("a second tab should hide them again")
	}
}

func TestEnterShowsTheProjectsTasks(t *testing.T) {
	m := startProjects(t, &fake{open: tasks(), projects: []store.Project{project("herdr", "P1", 1, 0)}})
	m = drive(t, m, key("enter"))
	if m.mode != modeList || m.filter != "project:herdr" {
		t.Fatalf("mode=%v filter=%q", m.mode, m.filter)
	}
	if len(m.visible) != 1 || m.visible[0].ID != 3 {
		t.Errorf("visible = %+v, want only #3", m.visible)
	}
	m = drive(t, m, key("esc"))
	if m.filter != "" || len(m.visible) != 3 {
		t.Errorf("esc should clear the project filter")
	}
}

func TestEnterFromTheResolvedListReturnsToOpenTasks(t *testing.T) {
	f := &fake{open: tasks(), projects: []store.Project{project("herdr", "P1", 1, 0)}}
	m := start(t, f)
	m = drive(t, m, key("tab"), key("p"), key("enter"))
	if m.showResolved || len(m.visible) != 1 {
		t.Errorf("showResolved=%v visible=%d", m.showResolved, len(m.visible))
	}
}

func TestContextHidingAProjectIsExplained(t *testing.T) {
	m := startProjects(t, &fake{open: tasks(), projects: []store.Project{project("elsewhere", "P2", 2, 0)}})
	m = drive(t, m, key("enter"))
	if !m.statusErr || !strings.Contains(m.status, "context +work hides") {
		t.Errorf("status = %q", m.status)
	}
}

func TestProjectFilterWord(t *testing.T) {
	open := append(tasks(), dstask.Task{UUID: "x", ID: 11, Summary: "about herdr things", Status: "pending", Priority: "P2"})
	for filter, want := range map[string]string{
		"project:herdr":        "[3]",
		"project:HERDR":        "[3]",    // case does not matter
		"project:her":          "[]",     // a project matches whole, not in part
		"herdr":                "[3 11]", // a bare word still searches text
		"project:":             "[7 9 11]",
		"project:herdr plugin": "[3]",
	} {
		var got []int
		for _, task := range open {
			if matches(task, filter) {
				got = append(got, task.ID)
			}
		}
		if fmt.Sprint(got) != want {
			t.Errorf("%q: got %v, want %s", filter, got, want)
		}
	}
}

func TestProjectViewKeys(t *testing.T) {
	var many []store.Project
	for i := 0; i < 30; i++ {
		many = append(many, project(string(rune('a'+i%26))+strings.Repeat("x", i/26+1), "P2", 2, 1))
	}
	m := startProjects(t, &fake{open: tasks(), projects: many})
	rows := m.projectRows()
	if m.projectPages() != 2 || !strings.Contains(m.View(), "••") {
		t.Fatalf("pages=%d", m.projectPages())
	}
	m = drive(t, m, key("j"), key("l"))
	if m.pcursor != 1+rows || m.poffset != rows {
		t.Errorf("l: cursor=%d offset=%d", m.pcursor, m.poffset)
	}
	m = drive(t, m, key("l"), key("h"), key("h"), key("G"))
	if m.pcursor != 29 {
		t.Errorf("G: cursor=%d", m.pcursor)
	}
	m = drive(t, m, key("g"), key("ctrl+d"), key("ctrl+u"), key("k"))
	if m.pcursor != 0 {
		t.Errorf("cursor=%d, want 0", m.pcursor)
	}
	m = drive(t, m, key("?"))
	if m.mode != modeHelp || !strings.Contains(m.View(), "show or hide finished projects") {
		t.Fatalf("help from the project view")
	}
	m = drive(t, m, key("z"), key("r"))
	if m.mode != modeProjects {
		t.Errorf("help should return to the project view")
	}
	if drive(t, m, key("p")).mode != modeList || drive(t, m, key("esc")).mode != modeList {
		t.Errorf("p and esc should return to the list")
	}
	if _, cmd := m.Update(key("q")); cmd == nil || cmd() != tea.Quit() {
		t.Errorf("q should quit")
	}
}

func TestProjectsErrorAndEmpty(t *testing.T) {
	m := startProjects(t, &fake{open: tasks(), projectsErr: errors.New("cannot read resolved")})
	if !strings.Contains(m.View(), "cannot read resolved") {
		t.Errorf("load error not shown")
	}
	m = startProjects(t, &fake{open: tasks()})
	if !strings.Contains(m.View(), "no projects: give a task one") {
		t.Errorf("empty message missing")
	}
	m = startProjects(t, &fake{open: tasks(), projects: []store.Project{project("done", "P2", 1, 1)}})
	if !strings.Contains(m.View(), "no projects with open tasks") {
		t.Errorf("all-finished message missing")
	}
	if drive(t, m, key("enter")).mode != modeProjects {
		t.Errorf("enter with no project selected must do nothing")
	}
}

// A bar shows any progress, and it is full only when every task is done.
func TestProgressBarRounding(t *testing.T) {
	m := start(t, &fake{})
	cols := projectCols{name: 8, open: 7}
	for _, c := range []struct {
		p    store.Project
		want int
	}{
		{project("none", "P2", 5, 0), 0},
		{project("tiny", "P2", 100, 1), 1},
		{project("half", "P2", 10, 5), 10},
		{project("almost", "P2", 100, 99), barWidth - 1},
		{project("done", "P2", 3, 3), barWidth},
	} {
		row := m.projectRow(c.p, cols, false)
		if got := strings.Count(row, styleBarDone.Render(strings.Repeat("━", c.want))); c.want > 0 && got == 0 {
			t.Errorf("%s: want %d filled cells in %q", c.p.Name, c.want, row)
		}
	}
}

func TestProjectColumnsAlign(t *testing.T) {
	m := startProjects(t, &fake{open: tasks(), projects: sampleProjects()})
	m = drive(t, m, key("tab"))
	lines := strings.Split(m.View(), "\n")
	header := stripSGR(lines[1])
	at := lipgloss.Width(header[:strings.Index(header, "LAST DONE")])
	for _, line := range lines[2:7] {
		plain := stripSGR(line)
		for _, age := range []string{"3d ago", "1h ago", "Jun 2026"} {
			if i := strings.Index(plain, age); i >= 0 {
				if col := lipgloss.Width(plain[:i]); col != at {
					t.Errorf("%q at column %d, LAST DONE at %d:\n%s", age, col, at, plain)
				}
			}
		}
	}
}

func TestShortAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		5 * time.Minute:      "5m ago",
		5 * time.Hour:        "5h ago",
		10 * 24 * time.Hour:  "10d ago",
		100 * 24 * time.Hour: "Jun 2026",
	} {
		if got := shortAge(testNow.Add(-d), testNow); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

// sortProjects has open projects that differ in every sort key, plus one
// finished project that must stay last in every order.
func sortProjects() []store.Project {
	mk := func(name, priority string, tasks, resolved, daysAgo int) store.Project {
		p := project(name, priority, tasks, resolved)
		if daysAgo > 0 {
			p.Resolved = testNow.Add(-time.Duration(daysAgo) * 24 * time.Hour)
		}
		return p
	}
	return []store.Project{
		mk("alpha", "P2", 10, 9, 20), // 90% done, 1 open
		mk("bravo", "P0", 10, 2, 1),  // 20% done, 8 open, newest
		mk("charlie", "P1", 4, 2, 5), // 50% done, 2 open
		mk("delta", "P1", 3, 0, 0),   // 0% done, 3 open, never finished one
		mk("echo", "P2", 2, 2, 2),    // finished
	}
}

func TestProjectSortOrders(t *testing.T) {
	m := startProjects(t, &fake{open: tasks(), projects: sortProjects()})
	m = drive(t, m, key("tab")) // show the finished project too
	for _, c := range []struct {
		sort  string
		order string
		mark  string
	}{
		{"urgency", "bravo,charlie,delta,alpha,echo", ""},
		{"progress", "alpha,charlie,bravo,delta,echo", "PROGRESS ▾"},
		{"open", "bravo,delta,charlie,alpha,echo", "OPEN ▾"},
		{"last done", "bravo,charlie,alpha,delta,echo", "LAST DONE ▾"},
	} {
		if m.psort.String() != c.sort {
			t.Fatalf("sort = %s, want %s", m.psort, c.sort)
		}
		if got := names(m.projects); got != c.order {
			t.Errorf("%s: order = %s, want %s", c.sort, got, c.order)
		}
		view := m.View()
		if !strings.Contains(view, "sort ") || !strings.Contains(view, c.sort) {
			t.Errorf("%s: header does not name the order", c.sort)
		}
		if c.mark != "" && !strings.Contains(view, c.mark) {
			t.Errorf("%s: header lacks %q", c.sort, c.mark)
		}
		if c.mark == "" && strings.Contains(view, "▾") {
			t.Errorf("urgency has no column to mark")
		}
		m = drive(t, m, key("s"))
	}
	if m.psort != sortUrgency {
		t.Errorf("s should wrap round to urgency, got %s", m.psort)
	}
	m = drive(t, m, key("S"))
	if m.psort != sortLastDone {
		t.Errorf("S should go back to last done, got %s", m.psort)
	}
}

// A new order returns to the first project, but a reload keeps the place.
func TestProjectSortReturnsToTheTop(t *testing.T) {
	f := &fake{open: tasks(), projects: sortProjects()}
	m := startProjects(t, f)
	m = drive(t, m, key("j"), key("j"))
	if p, _ := m.selectedProject(); p.Name != "delta" {
		t.Fatalf("setup: on %s", p.Name)
	}
	m = drive(t, m, key("r"))
	if p, _ := m.selectedProject(); p.Name != "delta" {
		t.Errorf("reload moved the cursor to %s", p.Name)
	}
	m = drive(t, m, key("s"))
	if m.pcursor != 0 {
		t.Errorf("a new order should start at the top, cursor=%d", m.pcursor)
	}
	// The order survives a trip to the task list and back.
	m = drive(t, m, key("p"), key("p"))
	if m.psort != sortProgress {
		t.Errorf("sort = %s after leaving the view, want progress", m.psort)
	}
}
