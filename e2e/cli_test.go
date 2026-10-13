//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	t.Parallel()
	out, _, code := newWorld(t).run("--version")
	if code != 0 || strings.TrimSpace(out) == "" {
		t.Errorf("--version: code %d, output %q", code, out)
	}
}

func TestListFlagPrintsTheOpenTasks(t *testing.T) {
	t.Parallel()
	out, _, code := newWorld(t, basic...).run("--list")
	if code != 0 {
		t.Fatalf("--list exited with %d", code)
	}
	for _, want := range []string{"write the plugin", "tidy the garage", "   1 P1"} {
		if !strings.Contains(out, want) {
			t.Errorf("--list lacks %q:\n%s", want, out)
		}
	}
}

func TestBadFlag(t *testing.T) {
	t.Parallel()
	if _, _, code := newWorld(t).run("--no-such-flag"); code != 2 {
		t.Errorf("a bad flag exits with %d, want 2", code)
	}
}

func TestMissingRepository(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.env = append(w.env, "DSTASK_GIT_REPO="+w.home+"/no-such-repo")
	_, stderr, code := w.run()
	if code != 1 || !strings.Contains(stderr, "no dstask repository") {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
}

func TestContext(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.env = append(w.env, "DSTASK_CONTEXT=+work")
	s := w.open()
	s.waitHeader("context +work")
	s.shows("write the plugin", "fix the 3995 bug")
	s.press("c")
	s.waitHeader("context ignored")
	s.waitFor("tidy the garage", "review the docs")
	s.press("c")
	s.waitHeader("context +work")
	s.waitGone("tidy the garage")
	s.quit()
}

func TestAllFlagIgnoresTheContext(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	w.env = append(w.env, "DSTASK_CONTEXT=+work")
	s := w.open("--all")
	s.waitHeader("context ignored")
	s.waitFor("tidy the garage")
	s.quit()
}
