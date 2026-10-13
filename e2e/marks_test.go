package e2e

import (
	"strings"
	"testing"
)

func TestMarkedTasksChangeInOneCommit(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	before := w.commits()
	s := w.open()
	s.waitSelected("write the plugin")
	s.press("space") // marks #1 and moves down
	s.waitSelected("review the docs")
	s.press("space")
	s.waitHeader("2 marked")
	if n := strings.Count(s.screen(), "✓"); n != 2 {
		t.Errorf("%d rows carry a mark, want 2:\n%s", n, s.dump())
	}
	s.press("d")
	s.waitFor("resolved 2 tasks")
	s.waitGone("write the plugin", "review the docs")
	s.quit()
	if got := w.commits() - before; got != 1 {
		t.Errorf("a bulk change made %d commits, want 1", got)
	}
}

func TestMarkedTasksStartTogether(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.press("space", "space")
	s.waitHeader("2 marked")
	s.press("s")
	s.waitFor("started 2 tasks")
	s.quit()
}

func TestMarkEveryTaskAndHideSomeWithAFilter(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.waitFor("write the plugin")
	s.press("*")
	s.waitHeader("4 marked")
	s.filter("garage")
	s.waitHeader("4 marked (3 hidden by the filter)")
	s.press("*") // every task shown is marked, so * unmarks them all
	s.waitUntil("marks cleared", func(string) bool { return !strings.Contains(s.header(), "4 marked") })
	s.quit()
}

func TestEditRefusesMarkedTasks(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.press("space")
	s.waitHeader("1 marked")
	s.press("e")
	s.waitFor("e and N edit one task")
	s.quit()
}
