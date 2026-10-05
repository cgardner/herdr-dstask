package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points every state folder at a temporary directory, so the tests
// never touch the real state.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	t.Setenv("XDG_STATE_HOME", "")
	return dir
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := isolate(t)
	repo := t.TempDir()
	want := View{
		Repo: repo, View: "projects", Filter: "project:atlas", ActiveOnly: true,
		ShowResolved: true, IgnoreContext: true, Task: "uuid-1",
		ProjectSort: "progress", ProjectFilter: "ai", ShowFinished: true, Project: "atlas",
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got := Load(repo)
	want.Repo = key(repo)
	if got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	// The file is under the state folder, not in the repository.
	if p := Path(repo); !strings.HasPrefix(p, filepath.Join(dir, "views")) {
		t.Errorf("path %s is outside the state folder", p)
	}
	if entries, _ := os.ReadDir(repo); len(entries) != 0 {
		t.Errorf("Save wrote into the repository")
	}
}

func TestEachRepositoryHasItsOwnView(t *testing.T) {
	isolate(t)
	a, b := t.TempDir(), t.TempDir()
	Save(View{Repo: a, Filter: "real"})
	Save(View{Repo: b, Filter: "sandbox"})
	if Load(a).Filter != "real" || Load(b).Filter != "sandbox" {
		t.Errorf("views mixed: %+v %+v", Load(a), Load(b))
	}
}

func TestPathsThatNameTheSameRepositoryShareAView(t *testing.T) {
	isolate(t)
	repo := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	Save(View{Repo: repo + "/", Filter: "same"})
	if Load(link).Filter != "same" {
		t.Errorf("a symlink and a trailing slash should find the same view")
	}
}

func TestMissingOrBrokenFilesGiveTheZeroView(t *testing.T) {
	isolate(t)
	repo := t.TempDir()
	if (Load(repo) != View{}) {
		t.Errorf("no file should give the zero view")
	}
	os.MkdirAll(filepath.Dir(Path(repo)), 0o700)
	os.WriteFile(Path(repo), []byte("{not json"), 0o600)
	if (Load(repo) != View{}) {
		t.Errorf("a broken file should give the zero view")
	}
	// A file that names another repository is a hash collision, not ours.
	os.WriteFile(Path(repo), []byte(`{"repo":"/elsewhere","filter":"x"}`), 0o600)
	if (Load(repo) != View{}) {
		t.Errorf("a file for another repository must not load")
	}
}

func TestSaveLeavesNoTemporaryFiles(t *testing.T) {
	isolate(t)
	repo := t.TempDir()
	Save(View{Repo: repo})
	Save(View{Repo: repo, Filter: "again"})
	entries, _ := os.ReadDir(filepath.Dir(Path(repo)))
	if len(entries) != 1 {
		t.Errorf("want one file, got %d", len(entries))
	}
}

func TestSaveReportsAFolderItCannotMake(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0o600)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", blocker) // a file, so views/ cannot exist
	if err := Save(View{Repo: t.TempDir()}); err == nil {
		t.Errorf("expected an error")
	}
}

func TestDirFallbacks(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "/xdg")
	if got := Dir(); got != "/xdg/herdr-dstask/views" {
		t.Errorf("XDG: %s", got)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/me")
	if got := Dir(); got != "/home/me/.local/state/herdr-dstask/views" {
		t.Errorf("home: %s", got)
	}
	t.Setenv("HOME", "")
	if Dir() != "" || Path("/r") != "" || Save(View{Repo: "/r"}) == nil || (Load("/r") != View{}) {
		t.Errorf("no home and no state folder: nothing to read or write")
	}
}
