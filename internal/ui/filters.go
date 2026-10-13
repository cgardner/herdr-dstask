package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// filterChip is one filter that backspace can drop: the active filter, or one
// word of a text filter.
type filterChip struct {
	label  string
	active bool // the active-only filter, not a word of the text filter
}

// wordChips makes a chip of each word of a filter.
func wordChips(filter string) []filterChip {
	var chips []filterChip
	for _, word := range strings.Fields(filter) {
		chips = append(chips, filterChip{label: word})
	}
	return chips
}

// dropLastWord is the filter without its last word.
func dropLastWord(filter string) string {
	words := strings.Fields(filter)
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words[:len(words)-1], " ")
}

// filterChips lists the filters of the task list in the order the header
// shows them: the active filter, then each word of the text filter. The last
// one goes first.
func (m Model) filterChips() []filterChip {
	var chips []filterChip
	if m.activeOnly {
		chips = append(chips, filterChip{label: "▶ active only", active: true})
	}
	return append(chips, wordChips(m.filter)...)
}

// renderChips draws each filter in angle brackets. The one that backspace
// drops next is bold and underlined, and it carries the key. Both the task
// list and the project view use it, so the two look the same.
func renderChips(chips []filterChip) string {
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

// dropFilter takes the last filter off the task list, so each backspace
// widens the list by one filter. The selection stays on the same task, found
// by UUID, when that task is still in the list. With no filter set, it does
// nothing.
func (m Model) dropFilter() (tea.Model, tea.Cmd) {
	chips := m.filterChips()
	if len(chips) == 0 {
		return m, nil
	}
	if chips[len(chips)-1].active {
		m.activeOnly = false
	} else {
		m.filter = dropLastWord(m.filter)
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

// dropProjectFilter takes the last word off the project name filter.
// setProjects keeps the selection on the same project when it still matches.
func (m Model) dropProjectFilter() Model {
	m.pfilter = dropLastWord(m.pfilter)
	m.setProjects(m.allProjects)
	return m
}
