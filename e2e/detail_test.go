//go:build e2e

package e2e

import (
	"testing"
)

func TestDetailShowsTheFields(t *testing.T) {
	t.Parallel()
	s := newWorld(t, "P1 +work +urgent project:alpha write the plugin").open()
	s.waitSelected("write the plugin")
	s.press("enter")
	s.waitFor("#1 write the plugin", "status", "pending", "priority", "P1", "project", "alpha", "urgent · work", "no notes")
	s.press("esc")
	s.waitHeader("open 1")
	s.quit()
}

func TestDetailBackKeys(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	for _, k := range []string{"esc", "h", "left", "backspace", "q"} {
		s.waitSelected("write the plugin")
		s.press("enter")
		s.waitFor("#1 write the plugin")
		s.press(k)
		s.waitHeader("open 4")
	}
	s.quit()
}

func TestDetailClosesWhenTheTaskIsResolved(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.press("enter")
	s.waitFor("#1 write the plugin")
	s.press("d")
	s.waitFor("resolved #1")
	s.waitHeader("open 3")
	s.waitGone("write the plugin")
	s.quit()
}

func TestEditNotesInTheEditor(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.editor(`printf 'written in the editor\n' >> "$1"`)
	s := w.open()
	s.press("enter")
	s.waitFor("#1 write the plugin", "no notes")
	s.press("N")
	s.waitFor("written in the editor")
	s.press("esc") // q in the task view goes back, so leave it first
	s.quit()
}

func TestEditTheWholeTaskInTheEditor(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.editor(`sed -i.bak 's/^summary: .*/summary: renamed in the editor/' "$1"`)
	s := w.open()
	s.waitSelected("write the plugin")
	s.press("e")
	s.waitFor("renamed in the editor")
	s.waitGone("write the plugin")
	s.quit()
}
