//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReopenRestoresTheFilter(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	s := w.open()
	s.filter("garage")
	s.quit()

	s = w.open()
	s.waitHeader("garage ⌫")
	s.shows("tidy the garage")
	s.quit()
}

func TestReopenRestoresTheResolvedList(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	if err := w.store().Done(3); err != nil {
		t.Fatal(err)
	}
	s := w.open()
	s.press("tab")
	s.waitHeader("resolved 1")
	s.quit()

	s = w.open()
	s.waitHeader("resolved 1")
	s.quit()
}

func TestReopenRestoresTheProjectView(t *testing.T) {
	t.Parallel()
	w := projectWorld(t)
	s := w.openProjects()
	s.press("s")
	s.waitHeader("sort progress")
	s.quit()

	s = w.open()
	s.waitFor("PROGRESS")
	s.waitHeader("sort progress")
	s.quit()
}

// The selection is found by UUID, so it survives a new task that moves the
// selected task to another row.
func TestReopenRestoresTheSelectedTask(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	s := w.open()
	s.press("G")
	s.waitSelected("tidy the garage")
	s.quit()

	if err := w.store().Add("P0 a new first task"); err != nil {
		t.Fatal(err)
	}
	s = w.open()
	s.waitFor("a new first task")
	s.waitSelected("tidy the garage")
	s.quit()
}

func TestTheSavedViewStaysOutsideTheRepository(t *testing.T) {
	t.Parallel()
	w := newWorld(t, basic...)
	s := w.open()
	s.filter("garage")
	s.quit()
	if st := strings.TrimSpace(w.git("status", "--porcelain")); st != "" {
		t.Errorf("the plugin left files in the task repository: %q", st)
	}
	views, _ := filepath.Glob(filepath.Join(w.state, "views", "*.json"))
	if len(views) != 1 {
		t.Fatalf("want one saved view in the state folder, got %v", views)
	}
	b, err := os.ReadFile(views[0])
	if err != nil || !strings.Contains(string(b), "garage") {
		t.Errorf("the saved view holds the filter: %s %v", b, err)
	}
}
