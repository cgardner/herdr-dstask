package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// filterChip is one filter that backspace can drop: the active filter, or one
// word of the text filter.
type filterChip struct {
	label  string
	active bool // the active-only filter, not a word of the text filter
	word   int  // the index of the word in the text filter
}

// filterChips lists the filters in the order the header shows them: the
// active filter, then each word of the text filter. The last one goes first.
func (m Model) filterChips() []filterChip {
	var chips []filterChip
	if m.activeOnly {
		chips = append(chips, filterChip{label: "▶ active only", active: true})
	}
	for i, word := range strings.Fields(m.filter) {
		chips = append(chips, filterChip{label: word, word: i})
	}
	return chips
}

// filterHeader draws each filter in angle brackets. The one that backspace
// drops next is bold and underlined, and it carries the key.
func (m Model) filterHeader() string {
	chips := m.filterChips()
	cells := make([]string, len(chips))
	for i, c := range chips {
		st := styleTag
		if c.active {
			st = styleActive
		}
		label := st.Render(c.label)
		if i == len(chips)-1 {
			label = st.Bold(true).Underline(true).Render(c.label) + styleKey.Render(" ⌫")
		}
		cells[i] = styleDim.Render("‹") + label + styleDim.Render("›")
	}
	return strings.Join(cells, " ")
}

// dropFilter takes the last filter off, so each backspace widens the list by
// one filter. The selection stays on the same task, found by UUID, when that
// task is still in the list. With no filter set, it does nothing.
func (m Model) dropFilter() (tea.Model, tea.Cmd) {
	chips := m.filterChips()
	if len(chips) == 0 {
		return m, nil
	}
	last := chips[len(chips)-1]
	if last.active {
		m.activeOnly = false
	} else {
		words := strings.Fields(m.filter)
		m.filter = strings.Join(words[:last.word], " ")
	}
	keep := ""
	if t, ok := m.selected(); ok {
		keep = t.UUID
	}
	m.applyFilter()
	for i, t := range m.visible {
		if t.UUID == keep {
			m.cursor = i
			break
		}
	}
	m.clamp()
	return m, nil
}
