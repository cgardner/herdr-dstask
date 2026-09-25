package ui

import (
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The selection matches herdr-switcher-plus, which matches Herdr's own
// overlays: a Surface0 background from catppuccin, Herdr's default theme,
// with a bar in the accent color at the left edge.
const (
	defaultSelectionBg = "#313244"
	accentColor        = "6"
	selectionBgEnv     = "HERDR_DSTASK_SELECTION_BG"
)

var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// selectionBackground resolves the highlight color: the environment variable
// first, then an explicit selection_bg in the Herdr config, then the
// catppuccin default. Herdr exposes no API for the palette of a named theme,
// so those two are the only ways to follow a different theme. A malformed
// value falls back rather than rendering a broken escape sequence.
func selectionBackground() lipgloss.Color {
	if v := strings.TrimSpace(os.Getenv(selectionBgEnv)); hexColor.MatchString(v) {
		return lipgloss.Color(v)
	}
	if v := configSelectionBg(); hexColor.MatchString(v) {
		return lipgloss.Color(v)
	}
	return lipgloss.Color(defaultSelectionBg)
}

var selectionBgLine = regexp.MustCompile(`(?m)^\s*selection_bg\s*=\s*"([^"]*)"`)

// configSelectionBg reads the one key out of the Herdr config with an
// expression rather than a TOML parser. A miss is harmless, because the caller
// validates the result and falls back to the default.
func configSelectionBg() string {
	path := os.Getenv("HERDR_CONFIG_PATH")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = home + "/.config/herdr/config.toml"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	m := selectionBgLine.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}
