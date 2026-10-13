//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// projectWorld has alpha (one of three done), beta (none of one done) and
// gamma (finished).
func projectWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld(t,
		"P1 project:alpha alpha one",   // 1
		"P2 project:alpha alpha two",   // 2
		"P2 project:alpha alpha three", // 3
		"P3 project:beta beta one",     // 4
		"P2 project:gamma gamma one",   // 5
	)
	for _, id := range []int{3, 5} {
		if err := w.store().Done(id); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// openProjects opens the plugin and the project view.
func (w *world) openProjects() *session {
	w.t.Helper()
	s := w.open()
	s.waitFor("alpha one")
	s.press("p")
	s.waitFor("PROGRESS")
	return s
}

func TestProjectViewCounts(t *testing.T) {
	t.Parallel()
	s := projectWorld(t).openProjects()
	s.waitHeader("projects 2", "1 finished hidden", "sort urgency")
	s.waitFor("alpha", "1/3", "beta", "0/1")
	s.waitGone("gamma")
	s.waitSelected("alpha")
	s.press("tab")
	s.waitFor("gamma", "1/1", "hide finished")
	s.quit()
}

func TestProjectViewSorts(t *testing.T) {
	t.Parallel()
	s := projectWorld(t).openProjects()
	s.press("s")
	s.waitHeader("sort progress")
	s.press("S")
	s.waitHeader("sort urgency")
	s.quit()
}

func TestProjectFilterAndBackspace(t *testing.T) {
	t.Parallel()
	s := projectWorld(t).openProjects()
	s.prompt("/", "al ha")
	s.waitHeader("projects 1 of 2", "‹al›", "ha ⌫")
	s.waitGone("beta")

	s.press("backspace")
	s.waitHeader("al ⌫")
	s.waitUntil("ha is dropped", func(string) bool { return !strings.Contains(s.header(), "ha") })

	s.press("backspace")
	s.waitFor("beta")
	s.waitUntil("no filter is left", func(string) bool { return !strings.Contains(s.header(), "⌫") })

	// With no filter, backspace goes back to the task list.
	s.press("backspace")
	s.waitHeader("open 3")
	s.quit()
}

func TestProjectEscClearsTheFilterThenLeaves(t *testing.T) {
	t.Parallel()
	s := projectWorld(t).openProjects()
	s.prompt("/", "beta")
	s.waitGone("alpha")
	s.press("esc")
	s.waitFor("alpha", "beta")
	s.waitFor("PROGRESS")
	s.press("esc")
	s.waitHeader("open 3")
	s.quit()
}

func TestProjectEnterShowsItsTasks(t *testing.T) {
	t.Parallel()
	s := projectWorld(t).openProjects()
	s.waitSelected("alpha")
	s.press("enter")
	s.waitHeader("open 2 of 3", "project:alpha ⌫")
	s.waitFor("alpha one", "alpha two")
	s.waitGone("beta one")
	s.press("backspace")
	s.waitFor("beta one")
	s.quit()
}

func TestProjectViewPGoesBack(t *testing.T) {
	t.Parallel()
	s := projectWorld(t).openProjects()
	s.press("p")
	s.waitHeader("open 3")
	s.quit()
}
