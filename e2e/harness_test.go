// Package e2e runs the built plugin binary in a pseudo-terminal, against a
// new dstask repository in a temporary directory, and reads the screen back
// through a terminal emulator. The tests press keys as a user does and check
// the screen and the repository. This is a separate module that imports
// nothing from the plugin, so a refactor cannot change what the tests check,
// and the test libraries cannot change the versions the plugin is built with.
// The starting tasks come from tools/e2eseed, which uses the plugin's store.
package e2e

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// binary is the plugin and seed is tools/e2eseed, both built once for the
// whole run from the plugin's module, the parent folder.
var binary, seed string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "herdr-dstask-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary, seed = filepath.Join(dir, "herdr-dstask"), filepath.Join(dir, "e2eseed")
	for out, pkg := range map[string]string{binary: ".", seed: "./tools/e2eseed"} {
		build := exec.Command("go", "build", "-o", out, pkg)
		build.Dir = ".."
		if b, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// world is one test's private home: a task repository, a state folder for
// the saved view, and a home directory, all under t.TempDir. Two sessions on
// the same world share the saved view, as two openings of the popup do.
type world struct {
	t     *testing.T
	repo  string
	home  string
	state string
	env   []string // more variables for every session
	cols  int      // the terminal size of every session, cols x rows when 0
	rows  int
}

// newWorld makes a git repository and adds each task, in dstask syntax, so
// the first task gets ID 1.
func newWorld(t *testing.T, tasks ...string) *world {
	t.Helper()
	root := t.TempDir()
	w := &world{t: t, repo: filepath.Join(root, "repo"), home: filepath.Join(root, "home"), state: filepath.Join(root, "state")}
	for _, d := range []string{w.repo, w.home, w.state, filepath.Join(w.home, "tmp")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w.git("init", "-q")
	w.git("config", "user.email", "e2e@example.com")
	w.git("config", "user.name", "e2e")
	w.git("config", "commit.gpgsign", "false")
	var ops []string
	for _, task := range tasks {
		ops = append(ops, "add", task)
	}
	w.seed(ops...)
	return w
}

// seed runs tools/e2eseed on the repository: pairs of an op and its
// argument, such as "add", "P1 a task" or "done", "3".
func (w *world) seed(ops ...string) {
	w.t.Helper()
	if len(ops) == 0 {
		return
	}
	cmd := exec.Command(seed, append([]string{w.repo}, ops...)...)
	cmd.Env = append(os.Environ(), "DSTASK_CONTEXT=")
	if out, err := cmd.CombinedOutput(); err != nil {
		w.t.Fatalf("seed %q: %v: %s", ops, err, out)
	}
}

func (w *world) git(args ...string) string {
	w.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", w.repo}, args...)...).CombinedOutput()
	if err != nil {
		w.t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// commits is the number of commits in the task repository.
func (w *world) commits() int {
	w.t.Helper()
	var n int
	fmt.Sscan(w.git("rev-list", "--count", "HEAD"), &n)
	return n
}

// lastCommit is the subject of the newest commit in the task repository.
func (w *world) lastCommit() string {
	w.t.Helper()
	return strings.TrimSpace(w.git("log", "-1", "--format=%s"))
}

// editor makes a shell script that the plugin runs as $EDITOR on the file it
// opens, as "$1".
func (w *world) editor(script string) {
	w.t.Helper()
	path := filepath.Join(w.home, "editor.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		w.t.Fatal(err)
	}
	w.env = append(w.env, "EDITOR="+path)
}

// command is the plugin with the arguments, in the world's environment only,
// so nothing from the real home or the real dstask repository leaks in.
func (w *world) command(args ...string) *exec.Cmd {
	cmd := exec.Command(binary, args...)
	cmd.Dir = w.home
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + w.home,
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"DSTASK_GIT_REPO=" + w.repo,
		"DSTASK_CONTEXT=",
		"HERDR_PLUGIN_STATE_DIR=" + w.state,
		"XDG_STATE_HOME=" + filepath.Join(w.home, ".state"),
		"HERDR_CONFIG_PATH=" + filepath.Join(w.home, "no-herdr-config.toml"),
		"EDITOR=true",
		"TMPDIR=" + filepath.Join(w.home, "tmp"),
	}, w.env...)
	return cmd
}

// run runs the plugin without a terminal, for the flags that print and exit.
func (w *world) run(args ...string) (stdout, stderr string, code int) {
	w.t.Helper()
	cmd := w.command(args...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		w.t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

// session is one run of the plugin in a terminal of a fixed size.
type session struct {
	t    *testing.T
	cmd  *exec.Cmd
	pty  *os.File
	mu   sync.Mutex
	emu  *vt.Emulator
	done chan struct{}
	err  error
}

const (
	cols = 110
	rows = 20
)

// open starts the plugin with the arguments and waits for the first screen.
func (w *world) open(args ...string) *session {
	w.t.Helper()
	cmd := w.command(args...)
	c, r := cols, rows
	if w.cols > 0 {
		c, r = w.cols, w.rows
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(r), Cols: uint16(c)})
	if err != nil {
		w.t.Fatal(err)
	}
	s := &session{t: w.t, cmd: cmd, pty: f, emu: vt.NewEmulator(c, r), done: make(chan struct{})}
	// The plugin's output goes into the emulator, and the emulator's answers
	// to terminal queries go back as input, as a real terminal's do.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.emu.Write(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go io.Copy(f, s.emu)
	go func() { s.err = cmd.Wait(); close(s.done) }()
	w.t.Cleanup(s.kill)
	s.waitFor("dstask")
	return s
}

func (s *session) kill() {
	select {
	case <-s.done:
	default:
		s.cmd.Process.Kill()
		<-s.done
	}
	s.emu.Close()
	s.pty.Close()
}

// screen is the visible text, one line for each row.
func (s *session) screen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.emu.String()
}

// line is one row of the screen, counted from 0.
func (s *session) line(n int) string {
	lines := strings.Split(s.screen(), "\n")
	if n < len(lines) {
		return lines[n]
	}
	return ""
}

// header is the first row.
func (s *session) header() string { return s.line(0) }

// footer is the last row that has text: the key hints, a status, a
// confirmation or an open prompt.
func (s *session) footer() string {
	lines := strings.Split(strings.TrimRight(s.screen(), "\n "), "\n")
	return lines[len(lines)-1]
}

// waitFooter waits until the footer shows the text.
func (s *session) waitFooter(text string) {
	s.t.Helper()
	s.waitUntil(fmt.Sprintf("footer shows %q", text), func(string) bool { return strings.Contains(s.footer(), text) })
}

// waitPrompt waits for an open prompt, and waitNoPrompt for none.
func (s *session) waitPrompt() { s.t.Helper(); s.waitFooter("› ") }

func (s *session) waitNoPrompt() {
	s.t.Helper()
	s.waitUntil("the prompt closes", func(string) bool { return !strings.Contains(s.footer(), "› ") })
}

// selected is the screen row with the selection bar.
func (s *session) selected() string {
	for _, l := range strings.Split(s.screen(), "\n") {
		if strings.HasPrefix(l, "▌") {
			return l
		}
	}
	return ""
}

const wait = 10 * time.Second

// waitFor waits until the screen shows every text.
func (s *session) waitFor(texts ...string) {
	s.t.Helper()
	s.waitUntil(fmt.Sprintf("screen shows %q", texts), func(scr string) bool {
		for _, text := range texts {
			if !strings.Contains(scr, text) {
				return false
			}
		}
		return true
	})
}

// waitGone waits until the screen shows none of the texts.
func (s *session) waitGone(texts ...string) {
	s.t.Helper()
	s.waitUntil(fmt.Sprintf("screen hides %q", texts), func(scr string) bool {
		for _, text := range texts {
			if strings.Contains(scr, text) {
				return false
			}
		}
		return true
	})
}

// waitSelected waits until the selected row shows the text.
func (s *session) waitSelected(text string) {
	s.t.Helper()
	s.waitUntil(fmt.Sprintf("selected row shows %q", text), func(string) bool {
		return strings.Contains(s.selected(), text)
	})
}

// waitHeader waits until the header shows every text.
func (s *session) waitHeader(texts ...string) {
	s.t.Helper()
	s.waitUntil(fmt.Sprintf("header shows %q", texts), func(string) bool {
		h := s.header()
		for _, text := range texts {
			if !strings.Contains(h, text) {
				return false
			}
		}
		return true
	})
}

func (s *session) waitUntil(what string, ok func(string) bool) {
	s.t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if ok(s.screen()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("timed out: %s\n%s", what, s.dump())
}

// settle waits until the screen stops changing, before a check that
// something did not happen.
func (s *session) settle() {
	last, still := s.screen(), time.Now()
	for time.Since(still) < 200*time.Millisecond {
		time.Sleep(10 * time.Millisecond)
		if now := s.screen(); now != last {
			last, still = now, time.Now()
		}
	}
}

func (s *session) dump() string {
	return "----- screen -----\n" + s.screen() + "\n------------------"
}

// keys are the bytes a terminal sends for the named keys. Any other name is
// sent as it is.
var keys = map[string]string{
	"enter": "\r", "esc": "\x1b", "tab": "\t", "backspace": "\x7f", "space": " ",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
	"ctrl+c": "\x03", "ctrl+d": "\x04", "ctrl+u": "\x15",
}

// press sends each key alone, with a pause after it. Keys sent together can
// arrive in one read, and Bubble Tea then reads esc and a letter as alt and
// that letter.
func (s *session) press(names ...string) {
	s.t.Helper()
	for _, name := range names {
		b, ok := keys[name]
		if !ok {
			b = name
		}
		if _, err := s.pty.WriteString(b); err != nil {
			s.t.Fatalf("press %s: %v", name, err)
		}
		pause := 40 * time.Millisecond
		if name == "esc" {
			pause = 150 * time.Millisecond
		}
		time.Sleep(pause)
	}
}

// typeText sends text in one write, for a prompt.
func (s *session) typeText(text string) {
	s.t.Helper()
	if _, err := s.pty.WriteString(text); err != nil {
		s.t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
}

// prompt presses the key that opens a prompt, types the text and presses
// enter.
func (s *session) prompt(key, text string) {
	s.t.Helper()
	s.press(key)
	s.waitPrompt()
	s.typeText(text)
	s.press("enter")
	s.waitNoPrompt()
}

// filter sets the task filter.
func (s *session) filter(words string) {
	s.t.Helper()
	s.prompt("/", words)
}

// resize changes the terminal size, as a user who resizes the pane does.
func (s *session) resize(c, r int) {
	s.t.Helper()
	s.mu.Lock()
	s.emu.Resize(c, r)
	s.mu.Unlock()
	if err := pty.Setsize(s.pty, &pty.Winsize{Rows: uint16(r), Cols: uint16(c)}); err != nil {
		s.t.Fatal(err)
	}
}

// quit presses q and waits for a clean exit.
func (s *session) quit() {
	s.t.Helper()
	s.press("q")
	s.waitExit()
}

// waitExit waits for the plugin to exit with status 0.
func (s *session) waitExit() {
	s.t.Helper()
	select {
	case <-s.done:
		if s.err != nil {
			s.t.Fatalf("the plugin exited with %v\n%s", s.err, s.dump())
		}
	case <-time.After(wait):
		s.t.Fatalf("the plugin did not exit\n%s", s.dump())
	}
}
