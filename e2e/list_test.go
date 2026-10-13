package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// basic is the task list most tests start from. IDs follow the order.
var basic = []string{
	"P1 +work project:alpha write the plugin", // 1
	"P2 +docs project:alpha review the docs",  // 2
	"P3 +home tidy the garage",                // 3
	"P2 +work project:beta fix the 3995 bug",  // 4
}

func TestListShowsOpenTasksInPriorityOrder(t *testing.T) {
	t.Parallel()
	w := newWorld(t, "P3 +home tidy the garage", "P0 project:alpha fix the outage", "P2 review the docs")
	s := w.open()
	s.waitFor("fix the outage", "review the docs", "tidy the garage")
	scr := s.screen()
	if !(strings.Index(scr, "fix the outage") < strings.Index(scr, "review the docs") &&
		strings.Index(scr, "review the docs") < strings.Index(scr, "tidy the garage")) {
		t.Errorf("the list follows dstask next, priority first:\n%s", s.dump())
	}
	if h := s.header(); !strings.Contains(h, "open 3") {
		t.Errorf("header counts the open tasks: %q", h)
	}
	for _, col := range []string{"ID", "PROJECT", "TAGS", "SUMMARY"} {
		if !strings.Contains(s.line(1), col) {
			t.Errorf("column heading %s missing: %q", col, s.line(1))
		}
	}
	s.waitSelected("fix the outage")
	s.quit()
}

func TestListWithNoTasks(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	s := w.open()
	s.waitFor("no tasks")
	s.quit()
}

func TestListMovesTheSelection(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.waitSelected("write the plugin")
	s.press("j")
	s.waitSelected("review the docs")
	s.press("down")
	s.waitSelected("fix the 3995 bug")
	s.press("k")
	s.waitSelected("review the docs")
	s.press("G")
	s.waitSelected("tidy the garage")
	s.press("g")
	s.waitSelected("write the plugin")
	s.quit()
}

func TestListPages(t *testing.T) {
	t.Parallel()
	var tasks []string
	for i := 1; i <= 20; i++ {
		tasks = append(tasks, fmt.Sprintf("P2 numbered task %02d", i))
	}
	s := newWorld(t, tasks...).open()
	s.waitFor("numbered task 01")
	s.settle()
	if strings.Contains(s.screen(), "numbered task 20") {
		t.Fatalf("20 tasks do not fit on one page of a 20-row pane:\n%s", s.dump())
	}
	s.press("l")
	s.waitFor("numbered task 20")
	s.waitGone("numbered task 01")
	s.press("h")
	s.waitFor("numbered task 01")
	s.press("G")
	s.waitSelected("numbered task 20")
	s.quit()
}

func TestListQuitKeys(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	for _, k := range []string{"q", "ctrl+c", "esc"} {
		s := w.open()
		s.waitFor("write the plugin")
		s.press(k)
		s.waitExit()
	}
}

func TestListFollowsAResize(t *testing.T) {
	t.Parallel()
	s := newWorld(t, "P2 a summary that is far too long to fit in a narrow pane of sixty columns").open()
	s.waitFor("sixty columns")
	s.resize(60, rows)
	s.waitGone("sixty columns")
	s.waitFor("…", "dstask")
	for i, l := range strings.Split(s.screen(), "\n") {
		if w := ansi.StringWidth(l); w > 60 {
			t.Errorf("row %d is %d columns wide in a 60-column pane: %q", i, w, l)
		}
	}
	s.quit()
}

// The help screen is longer than the 20-row pane of the other tests, so this
// test uses a taller one.
func TestHelpScreen(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.cols, w.rows = cols, 50
	s := w.open()
	s.waitFor("write the plugin")
	s.press("?")
	s.waitFor("dstask keys", "backspace", "drop the last filter")
	s.press("j") // any key leaves the help screen
	s.waitFor("write the plugin")
	s.waitGone("dstask keys")
	s.quit()
}
