// Package state remembers the last view for each task repository.
//
// Herdr gives a plugin a private state folder in HERDR_PLUGIN_STATE_DIR, and
// its docs say plugin state goes there, not under the plugin root. Each
// repository gets its own file, so a filter set in the sandbox copy never
// shows up on the real tasks. Nothing is written inside the repository.
//
// Every failure here is silent. A forgotten view is a small annoyance, and
// refusing to open the plugin over it would be worse.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// View is everything the popup restores when it opens again.
type View struct {
	// Repo is the repository the view belongs to. It is stored so a person
	// reading the folder can tell the files apart; the file name is a hash.
	Repo string `json:"repo"`

	// View is "list" or "projects". A task view, a prompt or a confirmation
	// is a short step, so the popup reopens on the view behind it.
	View string `json:"view"`

	Filter        string `json:"filter,omitempty"`
	ActiveOnly    bool   `json:"activeOnly,omitempty"`
	ShowResolved  bool   `json:"showResolved,omitempty"`
	IgnoreContext bool   `json:"ignoreContext,omitempty"`
	// Task is the UUID of the selected task. A UUID survives the ID changes
	// that resolving and adding tasks cause.
	Task string `json:"task,omitempty"`

	ProjectSort   string `json:"projectSort,omitempty"`
	ProjectFilter string `json:"projectFilter,omitempty"`
	ShowFinished  bool   `json:"showFinished,omitempty"`
	Project       string `json:"project,omitempty"`
}

// Dir is the folder that holds the view files.
func Dir() string {
	if dir := os.Getenv("HERDR_PLUGIN_STATE_DIR"); dir != "" {
		return filepath.Join(dir, "views")
	}
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "herdr-dstask", "views")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "herdr-dstask", "views")
}

// key is the canonical form of a repository path, so ~/.dstask, a symlink
// to it and a path with a trailing slash all share one file.
func key(repo string) string {
	if abs, err := filepath.Abs(repo); err == nil {
		repo = abs
	}
	if real, err := filepath.EvalSymlinks(repo); err == nil {
		repo = real
	}
	return filepath.Clean(repo)
}

// Path is the file for a repository, or "" when no folder is available.
func Path(repo string) string {
	dir := Dir()
	if dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key(repo)))
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".json")
}

// Load returns the saved view for a repository, or the zero View when none is
// saved or the file cannot be read.
func Load(repo string) View {
	p := Path(repo)
	if p == "" {
		return View{}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return View{}
	}
	var v View
	if json.Unmarshal(b, &v) != nil || v.Repo != key(repo) {
		// A broken file, or a hash that collides with another repository.
		return View{}
	}
	return v
}

// Save records the view for its repository. It writes a temporary file and
// renames it, so a crash can never leave a half-written file behind.
func Save(v View) error {
	p := Path(v.Repo)
	if p == "" {
		return os.ErrNotExist
	}
	v.Repo = key(v.Repo)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".view-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
