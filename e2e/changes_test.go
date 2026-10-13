package e2e

import (
	"strings"
	"testing"
)

func TestStartAndStop(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	before := w.commits()
	s := w.open()
	s.waitSelected("write the plugin")
	s.press("s")
	s.waitFor("started #1")
	s.waitSelected("▶")
	s.press("s")
	s.waitFor("stopped #1")
	s.waitSelected("‖")
	s.quit()
	if got := w.commits() - before; got != 2 {
		t.Errorf("start and stop made %d commits, want 2", got)
	}
}

func TestDoneResolvesTheTaskInOneCommit(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	before := w.commits()
	s := w.open()
	s.waitSelected("write the plugin")
	s.press("d")
	s.waitFor("resolved #1")
	s.waitGone("write the plugin")
	s.quit()
	if got := w.commits() - before; got != 1 {
		t.Errorf("done made %d commits, want 1", got)
	}
	if c := w.lastCommit(); !strings.HasPrefix(c, "Resolved") {
		t.Errorf("the commit message follows the CLI: %q", c)
	}
}

func TestAddATask(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	s := w.open()
	s.prompt("a", "P0 +new project:gamma brand new task")
	s.waitFor("added a task", "brand new task")
	s.waitSelected("write the plugin")
	scr := s.screen()
	if strings.Index(scr, "brand new task") > strings.Index(scr, "write the plugin") {
		t.Errorf("a P0 task comes first:\n%s", s.dump())
	}
	s.quit()
	out, _, _ := w.run("--list")
	if !strings.Contains(out, "brand new task") || !strings.Contains(out, "gamma") {
		t.Errorf("the repository has the new task: %q", out)
	}
}

func TestModifyATask(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.waitSelected("write the plugin")
	s.prompt("m", "+urgent P0")
	s.waitFor("modified #1")
	s.press("enter")
	s.waitFor("#1 write the plugin", "urgent · work", "P0")
	s.press("esc") // q in the task view goes back, so leave it first
	s.quit()
}

func TestNoteATask(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.waitSelected("write the plugin")
	s.prompt("n", "remember the milk")
	s.waitFor("noted #1")
	s.press("enter")
	s.waitFor("#1 write the plugin", "remember the milk")
	s.press("esc") // q in the task view goes back, so leave it first
	s.quit()
}

func TestRemoveAsksFirst(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.waitSelected("write the plugin")
	s.press("x")
	s.waitFor(`remove #1 "write the plugin"? y to remove`)
	s.press("n")
	s.waitFor("kept #1")
	s.waitFor("write the plugin")
	s.press("x")
	s.waitFor("y to remove")
	s.press("y")
	s.waitFor("removed #1")
	s.waitGone("write the plugin")
	s.quit()
}

func TestUndoTheLastChange(t *testing.T) {
	t.Parallel()
	s := newWorld(t, basic...).open()
	s.waitSelected("write the plugin")
	s.press("d")
	s.waitGone("write the plugin")
	s.press("u")
	s.waitFor("undid the last change", "write the plugin")
	s.quit()
}

func TestResolvedTasksRefuseChanges(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.seed("done", "3")
	s := w.open()
	s.press("tab")
	s.waitHeader("resolved 1")
	s.waitSelected("tidy the garage")
	s.press("d")
	s.waitFor("resolved tasks have no id")
	s.press("A")
	s.waitFor("resolved tasks are never active")
	s.press("space")
	s.waitFor("cannot be marked")
	s.quit()
}
