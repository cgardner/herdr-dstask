package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/naggie/dstask"
)

// Colors are ANSI palette indexes 0 to 15, not fixed values, so the terminal
// theme decides the real color. Herdr gives a plugin no way to read its own
// theme, so the terminal palette is the only theme the pane can follow.
var (
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleCritical = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleHigh     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleLow      = lipgloss.NewStyle().Faint(true)
	styleActive   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	stylePaused   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleProject  = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleTag      = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleOverdue  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleKey      = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

// View renders the current mode.
func (m Model) View() string {
	switch m.mode {
	case modeHelp:
		return m.viewHelp()
	case modeDetail:
		return m.viewDetail()
	case modePrompt, modeConfirm:
		if m.back == modeDetail {
			return m.viewDetail()
		}
	}
	return m.viewList()
}

func (m Model) viewList() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteByte('\n')
	b.WriteString(styleDim.Render(fit(fmt.Sprintf("  %4s %-1s %-1s %-16s %s", "ID", "", "", "PROJECT", "SUMMARY"), m.width)))
	b.WriteByte('\n')

	rows := m.listRows()
	if len(m.visible) == 0 {
		msg := "no tasks"
		switch {
		case m.loading:
			msg = "loading…"
		case m.filter != "":
			msg = "no task matches " + fmt.Sprintf("%q", m.filter)
		case m.activeOnly:
			msg = "no active tasks (s starts one, A shows all)"
		}
		b.WriteString(styleDim.Render("  " + msg))
		b.WriteByte('\n')
		rows--
	}
	end := min(len(m.visible), m.offset+rows)
	for i := m.offset; i < end; i++ {
		b.WriteString(m.row(m.visible[i], i == m.cursor))
		b.WriteByte('\n')
	}
	for i := end - m.offset; i < rows; i++ {
		b.WriteByte('\n')
	}
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) header() string {
	view := "open"
	if m.showResolved {
		view = "resolved"
	}
	parts := []string{styleTitle.Render("dstask"), fmt.Sprintf("%s %d", view, len(m.visible))}
	if len(m.visible) != len(m.all) {
		parts[1] = fmt.Sprintf("%s %d of %d", view, len(m.visible), len(m.all))
	}
	switch {
	case m.ignoreContext:
		parts = append(parts, styleDim.Render("context ignored"))
	case m.context != "":
		parts = append(parts, "context "+styleProject.Render(m.context))
	}
	if m.activeOnly {
		parts = append(parts, styleActive.Render("▶ active only"))
	}
	if m.filter != "" {
		parts = append(parts, "filter "+styleTag.Render(m.filter))
	}
	if m.loading {
		parts = append(parts, styleDim.Render("…"))
	}
	return fit(" "+strings.Join(parts, styleDim.Render(" · ")), m.width)
}

// row draws one task. The selected row carries a background across the full
// width and a bar in the gutter, the way herdr-switcher-plus and Herdr's own
// overlays mark a selection.
//
// Every segment is rendered with the row base inherited. A foreground style
// emits its own reset at the end of each segment, so a background applied to
// the finished line would stop at the first colored segment.
func (m Model) row(t dstask.Task, selected bool) string {
	base := lipgloss.NewStyle()
	gutter := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.selectionBg)
		gutter = base.Foreground(lipgloss.Color(accentColor))
	}
	seg := func(st lipgloss.Style, text string) string { return st.Inherit(base).Render(text) }
	sp := base.Render(" ")

	bar := " "
	if selected {
		bar = "▌"
	}
	id := fmt.Sprintf("%4d", t.ID)
	if t.ID == 0 {
		id = "   –"
	}
	glyph, glyphStyle := statusMark(t.Status)
	line := gutter.Render(bar) + sp +
		seg(styleDim, id) + sp +
		seg(priorityStyle(t.Priority), priorityMark(t.Priority)) + sp +
		seg(glyphStyle, glyph) + sp +
		seg(styleProject, fmt.Sprintf("%-16s", ansi.Truncate(t.Project, 16, "…"))) + sp +
		seg(summaryStyle(t), t.Summary)
	if due, st := m.due(t); due != "" {
		line += sp + seg(st, due)
	}
	for _, tag := range t.Tags {
		line += sp + seg(styleTag, "+"+tag)
	}
	return fill(line, m.width, base)
}

// fill truncates a composed line to the width and pads it out with the row
// base, so a selection background reaches the right edge.
func fill(line string, width int, base lipgloss.Style) string {
	if width <= 0 {
		return line
	}
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, base.Render("…"))
	}
	if w := ansi.StringWidth(line); w < width {
		line += base.Render(strings.Repeat(" ", width-w))
	}
	return line
}

// due is the due-date label and its style: red once an open task is late.
func (m Model) due(t dstask.Task) (string, lipgloss.Style) {
	if t.Due.IsZero() {
		return "", styleDim
	}
	label := "due " + t.Due.Format("Mon 02 Jan")
	if t.Status != dstask.STATUS_RESOLVED && t.Due.Before(m.now()) {
		return label, styleOverdue
	}
	return label, styleDim
}

func (m Model) dueLabel(t dstask.Task) string {
	label, st := m.due(t)
	if label == "" {
		return ""
	}
	return st.Render(label)
}

// priorityMarks are quiet one-column dots. Only P0 and P1 are solid, so the
// tasks that need attention stand out, and the priority style gives each its
// color. Emoji are not used: they carry their own colors, which ignore the
// terminal theme.
var priorityMarks = map[string]string{
	dstask.PRIORITY_CRITICAL: "●",
	dstask.PRIORITY_HIGH:     "●",
	dstask.PRIORITY_NORMAL:   "○",
	dstask.PRIORITY_LOW:      "·",
}

// priorityMark is the circle for a priority. dstask validates priorities, so
// an unknown one falls back to blank space of the same width.
func priorityMark(p string) string {
	if mark, ok := priorityMarks[p]; ok {
		return mark
	}
	return " "
}

func priorityStyle(p string) lipgloss.Style {
	switch p {
	case dstask.PRIORITY_CRITICAL:
		return styleCritical
	case dstask.PRIORITY_HIGH:
		return styleHigh
	case dstask.PRIORITY_LOW:
		return styleLow
	}
	return lipgloss.NewStyle()
}

func summaryStyle(t dstask.Task) lipgloss.Style {
	switch t.Status {
	case dstask.STATUS_ACTIVE:
		return styleActive
	case dstask.STATUS_RESOLVED:
		return styleDim
	}
	if t.Priority == dstask.PRIORITY_LOW {
		return styleLow
	}
	return lipgloss.NewStyle()
}

// statusMark is the one-column status mark and its style. Pending has none.
func statusMark(status string) (string, lipgloss.Style) {
	switch status {
	case dstask.STATUS_ACTIVE:
		return "▶", styleActive
	case dstask.STATUS_PAUSED:
		return "‖", stylePaused
	case dstask.STATUS_RESOLVED:
		return "✓", styleDim
	case dstask.STATUS_DELEGATED:
		return "→", styleDim
	case dstask.STATUS_DEFERRED:
		return "z", styleDim
	}
	return " ", lipgloss.NewStyle()
}

func statusGlyph(status string) string {
	glyph, st := statusMark(status)
	return st.Render(glyph)
}

func (m Model) footer() string {
	var line string
	switch m.mode {
	case modePrompt:
		return m.promptLine()
	case modeConfirm:
		return styleError.Render(fit(fmt.Sprintf(" remove #%d %q? y to remove, any other key keeps it", m.target.ID, m.target.Summary), m.width))
	}
	if m.status != "" {
		if m.statusErr {
			line = styleError.Render(fit(" "+m.status, m.width))
		} else {
			line = styleOK.Render(fit(" "+m.status, m.width))
		}
		return line
	}
	if m.mode == modeDetail {
		return hints(m.width, "esc", "back", "s", "start/stop", "d", "done", "m", "modify", "n", "note", "N", "edit notes", "e", "edit", "x", "remove", "?", "help")
	}
	return hints(m.width, "enter", "view", "/", "filter", "A", "active", "a", "add", "s", "start/stop", "d", "done", "m", "modify", "n", "note", "e", "edit", "tab", "resolved", "?", "help")
}

func (m Model) promptLine() string {
	label := map[promptKind]string{
		promptFilter: "filter",
		promptModify: fmt.Sprintf("modify #%d", m.target.ID),
		promptNote:   fmt.Sprintf("note #%d", m.target.ID),
		promptAdd:    "add",
	}[m.prompt]
	return fit(" "+styleKey.Render(label)+" "+m.input.View(), m.width)
}

func hints(width int, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, styleKey.Render(pairs[i])+" "+styleDim.Render(pairs[i+1]))
	}
	return fit(" "+strings.Join(parts, "  "), width)
}

// refreshDetail rebuilds the detail viewport for the target task.
func (m *Model) refreshDetail() {
	m.detail.SetContent(m.detailBody(m.target))
}

func (m Model) viewDetail() string {
	t := m.target
	title := fmt.Sprintf(" #%d %s", t.ID, t.Summary)
	if t.ID == 0 {
		title = " " + t.Summary
	}
	pct := ""
	if m.detail.TotalLineCount() > m.detail.Height {
		pct = styleDim.Render(fmt.Sprintf(" %3.0f%%", m.detail.ScrollPercent()*100))
	}
	return fit(styleTitle.Render(title), m.width-5) + pct + "\n" +
		styleDim.Render(strings.Repeat("─", max(0, m.width))) + "\n" +
		m.detail.View() + "\n" + m.footer()
}

func (m Model) detailBody(t dstask.Task) string {
	width := max(20, m.width-2)
	field := func(name, value string) string {
		return fmt.Sprintf(" %s %s\n", styleDim.Render(fmt.Sprintf("%-9s", name)), value)
	}
	var b strings.Builder
	b.WriteString(field("status", strings.TrimSpace(statusGlyph(t.Status)+" "+t.Status)))
	b.WriteString(field("priority", priorityStyle(t.Priority).Render(priorityMark(t.Priority)+" "+t.Priority)))
	if t.Project != "" {
		b.WriteString(field("project", styleProject.Render(t.Project)))
	}
	if len(t.Tags) > 0 {
		tags := make([]string, len(t.Tags))
		for i, tag := range t.Tags {
			tags[i] = styleTag.Render("+" + tag)
		}
		b.WriteString(field("tags", strings.Join(tags, " ")))
	}
	if due := m.dueLabel(t); due != "" {
		b.WriteString(field("due", due))
	}
	b.WriteString(field("created", age(t.Created, m.now())))
	if !t.Resolved.IsZero() {
		b.WriteString(field("resolved", age(t.Resolved, m.now())))
	}
	b.WriteString(field("uuid", styleDim.Render(t.UUID)))
	b.WriteByte('\n')
	if strings.TrimSpace(t.Notes) == "" {
		b.WriteString(styleDim.Render(" no notes (n appends a line, N opens the editor)"))
		return b.String()
	}
	b.WriteString(lipgloss.NewStyle().Width(width).PaddingLeft(1).Render(t.Notes))
	return b.String()
}

func age(at, now time.Time) string {
	d := now.Sub(at)
	var rel string
	switch {
	case d < time.Hour:
		rel = fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		rel = fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		rel = fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return at.Format("Mon 02 Jan 2006 15:04") + styleDim.Render(" ("+rel+")")
}

func (m Model) viewHelp() string {
	keys := [][2]string{
		{"j k ↑ ↓", "move"},
		{"g G", "first, last"},
		{"ctrl+d ctrl+u", "half a page down, up"},
		{"enter l", "view the task"},
		{"esc h", "back; in the list, clear a filter or quit"},
		{"/", "filter by words in summary, project, tags, notes"},
		{"A", "show only active tasks, or all tasks again"},
		{"tab", "switch between open and resolved tasks"},
		{"c", "switch between the dstask context and every task"},
		{"r", "reload"},
		{"", ""},
		{"a", "add a task (dstask syntax: +tag project:x P1 summary)"},
		{"s", "start, or stop an active task"},
		{"d", "resolve"},
		{"m", "modify (+tag -tag project:x P1 due:friday)"},
		{"n", "append a line to the notes"},
		{"N", "edit the notes in $EDITOR"},
		{"e", "edit the whole task in $EDITOR"},
		{"x", "remove, after a confirmation"},
		{"u", "undo the last change (git revert)"},
		{"q", "quit"},
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render(" dstask keys") + "\n\n")
	for _, k := range keys {
		if k[0] == "" {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-14s", k[0])), k[1]))
	}
	b.WriteString("\n" + styleDim.Render("  any key closes this help"))
	return b.String()
}

// fit truncates a styled line to the pane width without splitting an escape
// sequence.
func fit(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Truncate(s, width, "…")
}
