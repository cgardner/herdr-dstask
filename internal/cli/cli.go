// Package cli parses the command line and starts the UI.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cgardner/herdr-dstask/internal/store"
	"github.com/cgardner/herdr-dstask/internal/ui"
)

// version is set at build time from herdr-plugin.toml.
var version = "dev"

// backend adapts store.Store to ui.Backend. The two differ only in the type
// the editor methods return, which keeps the store free of UI types.
type backend struct{ *store.Store }

func (b backend) EditTask(id int) (ui.Editor, error)  { return editor(b.Store.EditTask(id)) }
func (b backend) EditNotes(id int) (ui.Editor, error) { return editor(b.Store.EditNotes(id)) }

func editor(e *store.Edit, err error) (ui.Editor, error) {
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Seams. Each reaches outside the process, and a test replaces it.
var (
	// openStore opens the repository that dstask itself would use.
	openStore = store.New

	// runProgram is the one call that needs a real terminal.
	runProgram = func(m ui.Model) error {
		_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
		return err
	}
)

// Run is the whole program. It returns the process exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("herdr-dstask", flag.ContinueOnError)
	fs.SetOutput(stderr)
	list := fs.Bool("list", false, "print the open tasks and exit, without the UI")
	all := fs.Bool("all", false, "ignore the dstask context")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}

	s, err := openStore()
	if err != nil {
		fmt.Fprintln(stderr, "herdr-dstask:", err)
		return 1
	}
	s.SetIgnoreContext(*all)

	if *list {
		tasks, err := s.Open()
		if err != nil {
			fmt.Fprintln(stderr, "herdr-dstask:", err)
			return 1
		}
		for _, t := range tasks {
			fmt.Fprintf(stdout, "%4d %s %-9s %-16s %s %s\n", t.ID, t.Priority, t.Status, t.Project, t.Summary, strings.Join(t.Tags, ","))
		}
		return 0
	}

	if err := runProgram(ui.New(backend{s})); err != nil {
		fmt.Fprintln(stderr, "herdr-dstask:", err)
		return 1
	}
	return 0
}
