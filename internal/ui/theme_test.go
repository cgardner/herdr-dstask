package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

var sgr = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// bgCoverage reports, for each visible cell of a rendered line, whether the
// selection background was active. A colored segment emits its own reset, so
// this is where a background painted only on the outside would show its gaps.
func bgCoverage(line string) []bool {
	var cells []bool
	on := false
	for len(line) > 0 {
		if loc := sgr.FindStringSubmatchIndex(line); loc != nil && loc[0] == 0 {
			params := line[loc[2]:loc[3]]
			switch {
			case strings.Contains(params, "48;2;"):
				on = true
			case params == "" || params == "0":
				on = false
			}
			line = line[loc[1]:]
			continue
		}
		r := []rune(line)[0]
		cells = append(cells, on)
		line = line[len(string(r)):]
	}
	return cells
}

func TestSelectedRowIsHighlightedAcrossTheWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := start(t, &fake{open: tasks()})
	m.selectionBg = lipgloss.Color(defaultSelectionBg)
	m.now = func() time.Time { return time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC) }

	line := m.row(tasks()[0], true)
	if w := ansi.StringWidth(line); w != m.width {
		t.Fatalf("selected row is %d wide, want %d", w, m.width)
	}
	for i, on := range bgCoverage(line) {
		if !on {
			t.Fatalf("cell %d has no selection background: %q", i, line)
		}
	}
	if !strings.HasPrefix(ansi.Strip(line), "▌") {
		t.Errorf("selected row lacks the gutter bar: %q", ansi.Strip(line))
	}

	plain := m.row(tasks()[1], false)
	if strings.Contains(plain, "48;2;") {
		t.Errorf("an unselected row carries the selection background")
	}
}

func TestSelectionBackgroundPrecedence(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(cfg, []byte("[theme.custom]\nselection_bg = \"#445566\"\n"), 0o600)
	t.Setenv("HERDR_CONFIG_PATH", cfg)

	t.Setenv(selectionBgEnv, "#112233")
	if got := selectionBackground(); got != "#112233" {
		t.Errorf("env should win, got %s", got)
	}
	t.Setenv(selectionBgEnv, "not a color")
	if got := selectionBackground(); got != "#445566" {
		t.Errorf("config should win over the default, got %s", got)
	}
	t.Setenv("HERDR_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.toml"))
	if got := selectionBackground(); got != defaultSelectionBg {
		t.Errorf("default expected, got %s", got)
	}
}
