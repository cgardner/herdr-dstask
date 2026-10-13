package e2e

import (
	"strings"
	"testing"
)

// shows checks that the screen lists every one of want and none of the other
// basic tasks.
func (s *session) shows(want ...string) {
	s.t.Helper()
	s.waitFor(want...)
	var others []string
	for _, task := range []string{"write the plugin", "review the docs", "tidy the garage", "fix the 3995 bug"} {
		found := false
		for _, w := range want {
			found = found || w == task
		}
		if !found {
			others = append(others, task)
		}
	}
	s.waitGone(others...)
}

func TestFilterNarrowsByWords(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.filter("the docs")
	s.shows("review the docs")
	s.waitHeader("‹the›", "docs ⌫", "open 1 of 4")
	s.quit()
}

func TestFilterNarrowsWhileTyping(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.press("/")
	s.waitPrompt()
	s.typeText("gar")
	s.waitFooter("› gar")
	s.shows("tidy the garage")
	s.press("enter")
	s.quit()
}

func TestFilterWords(t *testing.T) {
	t.Parallel()
	cases := []struct {
		filter string
		want   []string
	}{
		{"project:alpha", []string{"write the plugin", "review the docs"}},
		{"project:", []string{"tidy the garage"}},
		{"+WORK", []string{"write the plugin", "fix the 3995 bug"}},
		{"#2", []string{"review the docs"}},
		{"#1 #3", []string{"write the plugin", "tidy the garage"}},
		{"3995", []string{"fix the 3995 bug"}},
		{"P3", []string{"tidy the garage"}},
	}
	w := newWorld(t, basic...)
	for _, c := range cases {
		t.Run(c.filter, func(t *testing.T) {
			s := w.open()
			s.t = t
			s.filter(c.filter)
			s.shows(c.want...)
			s.press("esc")
			s.waitFor("write the plugin", "tidy the garage")
			s.quit()
		})
	}
}

func TestFilterPromptStartsFromTheFilter(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.filter("garage")
	s.press("/")
	s.waitFooter("› garage")
	s.press("esc")
	s.waitNoPrompt()
	s.quit()
}

// esc in the filter prompt closes it but keeps what was typed, because the
// filter applies as the user types. This pins the behavior as it is today.
func TestFilterPromptEscKeepsTheTypedText(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.press("/")
	s.waitPrompt()
	s.typeText("garage")
	s.waitFooter("› garage")
	s.press("esc")
	s.waitNoPrompt()
	s.shows("tidy the garage")
	s.waitHeader("garage ⌫")
	s.quit()
}

func TestFilterHashKeyOpensTheIDSearch(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.press("#")
	s.waitFooter("› #")
	s.typeText("3")
	s.press("enter")
	s.shows("tidy the garage")
	s.quit()
}

func TestFilterWithNoMatchShowsEveryTask(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.filter("zzz")
	s.waitFor(`no task matches "zzz", showing all tasks`)
	s.waitFor("write the plugin", "review the docs", "tidy the garage", "fix the 3995 bug")
	s.quit()
}

func TestActiveOnly(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.seed("start", "1")
	s := w.open()
	s.press("A")
	s.shows("write the plugin")
	s.waitHeader("▶ active only")
	s.press("A")
	s.waitFor("tidy the garage")
	s.waitUntil("header drops the active filter", func(string) bool { return !strings.Contains(s.header(), "active only") })
	s.quit()
}

func TestActiveOnlyWithNoActiveTask(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.press("A")
	s.waitFor("no active tasks (s starts one, A shows all), showing all tasks")
	s.quit()
}

func TestBackspaceDropsOneFilterAtATime(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.seed("start", "1")
	s := w.open()
	s.press("A")
	s.filter("project:alpha plugin")
	s.waitHeader("‹▶ active only›", "‹project:alpha›", "plugin ⌫")

	s.press("backspace")
	s.waitHeader("project:alpha ⌫")
	s.waitUntil("plugin is dropped", func(string) bool { return !strings.Contains(s.header(), "plugin") })

	s.press("backspace")
	s.waitHeader("▶ active only ⌫")
	s.waitUntil("project:alpha is dropped", func(string) bool { return !strings.Contains(s.header(), "project:alpha") })
	s.shows("write the plugin")

	s.press("backspace")
	s.waitFor("write the plugin", "review the docs", "tidy the garage", "fix the 3995 bug")
	s.waitUntil("no filter is left", func(string) bool { return !strings.Contains(s.header(), "⌫") })

	// With no filter, backspace does nothing, and the plugin keeps running.
	before := s.screen()
	s.press("backspace")
	s.settle()
	if s.screen() != before {
		t.Errorf("backspace with no filter changed the screen:\n%s", s.dump())
	}
	s.quit()
}

func TestBackspaceKeepsTheSelectedTask(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.filter("the garage")
	s.shows("tidy the garage")
	s.press("backspace")
	s.waitFor("write the plugin")
	s.waitSelected("tidy the garage")
	s.quit()
}

func TestEscClearsMarksThenFiltersThenQuits(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.seed("start", "1")
	s := w.open()
	s.press("A")
	s.filter("plugin")
	s.press("space")
	s.waitHeader("1 marked")

	s.press("esc")
	s.waitUntil("marks cleared", func(string) bool { return !strings.Contains(s.header(), "marked") })
	s.waitHeader("plugin ⌫", "▶ active only")

	s.press("esc")
	s.waitUntil("text filter cleared", func(string) bool { return !strings.Contains(s.header(), "plugin") })
	s.waitHeader("▶ active only")

	s.press("esc")
	s.waitUntil("active filter cleared", func(string) bool { return !strings.Contains(s.header(), "active only") })

	s.press("esc")
	s.waitExit()
}

func TestTabToResolvedClearsTheFilters(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.seed("done", "3")
	s := w.open()
	s.filter("plugin")
	s.shows("write the plugin")
	s.press("tab")
	s.waitHeader("resolved 1")
	s.waitFor("tidy the garage")
	s.waitUntil("filter cleared", func(string) bool { return !strings.Contains(s.header(), "plugin") })
	s.press("tab")
	s.waitHeader("open 3")
	s.quit()
}
