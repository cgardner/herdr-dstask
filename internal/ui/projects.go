package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/paginator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cgardner/herdr-dstask/internal/store"
)

// barWidth is the number of cells in a progress bar.
const barWidth = 20

// projectSort is an order for the project view. s and S cycle through them.
type projectSort int

const (
	sortUrgency  projectSort = iota // highest open priority first
	sortProgress                    // most done first
	sortOpen                        // most open tasks first
	sortLastDone                    // most recently finished task first
	projectSorts                    // the number of orders
)

var projectSortNames = [projectSorts]string{"urgency", "progress", "open", "last done"}

func (s projectSort) String() string { return projectSortNames[s] }

// less orders two open projects. Every order falls back to the name, so the
// result never depends on the order the library returned.
func (s projectSort) less(a, b store.Project) bool {
	switch s {
	case sortProgress:
		if a.Done() != b.Done() {
			return a.Done() > b.Done()
		}
	case sortOpen:
		if a.OpenTasks() != b.OpenTasks() {
			return a.OpenTasks() > b.OpenTasks()
		}
	case sortLastDone:
		if !a.Resolved.Equal(b.Resolved) {
			return a.Resolved.After(b.Resolved) // never finished sorts last
		}
	default:
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
	}
	return a.Name < b.Name
}

// projectsMsg carries a fresh project list.
type projectsMsg struct {
	projects []store.Project
	err      error
}

func (m Model) loadProjects() tea.Cmd {
	b := m.backend
	return func() tea.Msg {
		p, err := b.Projects()
		return projectsMsg{projects: p, err: err}
	}
}

// openProjects switches to the project view and loads it. The projects load
// every time the view opens, because a change in the list moves the numbers.
func (m Model) openProjects() (tea.Model, tea.Cmd) {
	m.mode = modeProjects
	m.projectsLoading = true
	return m, m.loadProjects()
}

// setProjects sorts the projects and keeps the cursor on the same project.
//
// Projects with open tasks come first, in the chosen order. Finished projects
// always follow, most recently finished first, and only when the view shows
// them. Under the progress order they would otherwise fill the top rows at
// 100%, above the projects that still need work.
func (m *Model) setProjects(all []store.Project) {
	keep := ""
	if p, ok := m.selectedProject(); ok {
		keep = p.Name
	}
	m.allProjects = all
	m.projects = m.projects[:0:0]
	for _, p := range all {
		if p.OpenTasks() > 0 || m.showFinished {
			m.projects = append(m.projects, p)
		}
	}
	sort.SliceStable(m.projects, func(i, j int) bool {
		a, b := m.projects[i], m.projects[j]
		if (a.OpenTasks() > 0) != (b.OpenTasks() > 0) {
			return a.OpenTasks() > 0
		}
		if a.OpenTasks() == 0 {
			return a.Resolved.After(b.Resolved)
		}
		return m.psort.less(a, b)
	})
	m.pcursor = 0
	for i, p := range m.projects {
		if p.Name == keep {
			m.pcursor = i
		}
	}
	m.clampProjects()
}

func (m Model) finishedProjects() int {
	n := 0
	for _, p := range m.allProjects {
		if p.OpenTasks() == 0 {
			n++
		}
	}
	return n
}

func (m Model) selectedProject() (store.Project, bool) {
	if m.pcursor < 0 || m.pcursor >= len(m.projects) {
		return store.Project{}, false
	}
	return m.projects[m.pcursor], true
}

// projectRows is the number of projects on a page, found the same way as
// listRows: less one line for the page dots when there is more than one page.
func (m Model) projectRows() int {
	rows := max(1, m.height-4)
	if len(m.projects) > rows {
		rows = max(1, m.height-5)
	}
	return rows
}

func (m Model) projectPages() int {
	return max(1, (len(m.projects)+m.projectRows()-1)/m.projectRows())
}

func (m *Model) clampProjects() {
	m.pcursor = min(max(m.pcursor, 0), max(len(m.projects)-1, 0))
	rows := m.projectRows()
	m.poffset = (m.pcursor / rows) * rows
}

func (m Model) updateProjects(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.projectRows()
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "p", "backspace":
		m.mode = modeList
		return m, nil
	case "j", "down":
		m.pcursor++
	case "k", "up":
		m.pcursor--
	case "g", "home":
		m.pcursor = 0
	case "G", "end":
		m.pcursor = len(m.projects) - 1
	case "ctrl+d":
		m.pcursor += rows / 2
	case "ctrl+u":
		m.pcursor -= rows / 2
	case "right", "l", "pgdown":
		if m.pcursor/rows < m.projectPages()-1 {
			m.pcursor = min(m.pcursor+rows, len(m.projects)-1)
		}
	case "left", "h", "pgup":
		if m.pcursor/rows > 0 {
			m.pcursor -= rows
		}
	case "tab":
		m.showFinished = !m.showFinished
		m.setProjects(m.allProjects)
		return m, nil
	case "s", "S":
		step := projectSort(1)
		if msg.String() == "S" {
			step = projectSorts - 1
		}
		m.psort = (m.psort + step) % projectSorts
		// A new order is a request to see a different top of the list, so
		// the view returns to the first project, as the switcher does.
		m.pcursor = 0
		m.setProjects(m.allProjects)
		m.pcursor = 0
		m.clampProjects()
		return m, nil
	case "r":
		m.projectsLoading = true
		return m, m.loadProjects()
	case "?":
		m.back, m.mode = modeProjects, modeHelp
		return m, nil
	case "enter":
		return m.showProject()
	}
	m.clampProjects()
	return m, nil
}

// showProject returns to the open task list, filtered to the project. The
// filter is the "project:" word, so it shows in the header and esc clears
// it, as for any filter.
func (m Model) showProject() (tea.Model, tea.Cmd) {
	p, ok := m.selectedProject()
	if !ok {
		return m, nil
	}
	m.mode = modeList
	m.filter = "project:" + p.Name
	m.activeOnly = false
	m.cursor, m.offset = 0, 0
	if m.showResolved {
		m.showResolved = false
		m.loading = true
		return m, m.load()
	}
	m.applyFilter()
	if len(m.visible) == 0 && p.OpenTasks() > 0 && m.context != "" && !m.ignoreContext {
		m.setErr(fmt.Errorf("the context %s hides the open tasks of %s; c shows every task", m.context, p.Name))
	}
	return m, nil
}

func (m Model) viewProjects() string {
	var b strings.Builder
	b.WriteString(m.projectsHeader())
	b.WriteByte('\n')

	cols := m.projectColumns()
	b.WriteString(styleDim.Render(fit(cols.header(m.psort), m.width)))
	b.WriteByte('\n')

	rows := m.projectRows()
	if len(m.projects) == 0 {
		msg := "no projects with open tasks (tab shows finished ones)"
		switch {
		case m.projectsLoading:
			msg = "loading…"
		case len(m.allProjects) == 0:
			msg = "no projects: give a task one with project:name"
		}
		b.WriteString(styleDim.Render("  " + msg))
		b.WriteByte('\n')
		rows--
	}
	end := min(len(m.projects), m.poffset+rows)
	for i := m.poffset; i < end; i++ {
		b.WriteString(m.projectRow(m.projects[i], cols, i == m.pcursor))
		b.WriteByte('\n')
	}
	for i := end - m.poffset; i < rows; i++ {
		b.WriteByte('\n')
	}
	if m.projectPages() > 1 {
		b.WriteString(m.dots(len(m.projects), m.projectRows(), m.pcursor/m.projectRows()))
		b.WriteByte('\n')
	}
	b.WriteString(m.projectsFooter())
	return b.String()
}

func (m Model) projectsHeader() string {
	parts := []string{styleTitle.Render("dstask"), fmt.Sprintf("projects %d", len(m.projects))}
	if n := m.finishedProjects(); n > 0 && !m.showFinished {
		parts = append(parts, styleDim.Render(fmt.Sprintf("%d finished hidden", n)))
	}
	parts = append(parts, "sort "+styleKey.Render(m.psort.String()))
	if m.projectsLoading {
		parts = append(parts, styleDim.Render("…"))
	}
	return fit(" "+strings.Join(parts, styleDim.Render(" · ")), m.width)
}

func (m Model) projectsFooter() string {
	if m.status != "" {
		if m.statusErr {
			return styleError.Render(fit(" "+m.status, m.width))
		}
		return styleOK.Render(fit(" "+m.status, m.width))
	}
	finished := "show finished"
	if m.showFinished {
		finished = "hide finished"
	}
	return hints(m.width, "enter", "show tasks", "s", "sort", "tab", finished, "p", "back", "r", "reload", "?", "help")
}

// projectCols are the widths of the columns that vary, measured over every
// project in the view, so the columns line up down the page. A width of 0
// means no project has that item, and the column is left out.
type projectCols struct {
	name, open, active, paused, late int
}

// header lines the column titles up with the cells that projectRow draws.
// A ▾ marks the column the view is sorted by. The urgency order has no
// column of its own, and the header names it instead.
func (c projectCols) header(s projectSort) string {
	mark := func(title string, by projectSort) string {
		if s == by {
			return title + " ▾"
		}
		return title
	}
	h := fmt.Sprintf("  %-*s %-*s %4s %7s  %-*s", c.name, "PROJECT", barWidth, mark("PROGRESS", sortProgress),
		"", "DONE", 2+c.open, mark("OPEN", sortOpen))
	for _, w := range []int{c.active, c.paused, c.late} {
		if w > 0 {
			h += strings.Repeat(" ", 2+w)
		}
	}
	return h + "  " + mark("LAST DONE", sortLastDone)
}

func (m Model) projectColumns() projectCols {
	c := projectCols{name: len("PROJECT")}
	width := func(format string, n int) int {
		if n == 0 {
			return 0
		}
		return ansi.StringWidth(fmt.Sprintf(format, n))
	}
	for _, p := range m.projects {
		c.name = max(c.name, len(p.Name))
		c.open = max(c.open, width("%d open", p.OpenTasks()))
		if p.OpenTasks() == 0 {
			c.open = max(c.open, len("finished")) // shares the column
		}
		c.active = max(c.active, width("▶ %d", p.ActiveTasks))
		c.paused = max(c.paused, width("‖ %d", p.PausedTasks))
		c.late = max(c.late, width("%d late", p.OverdueTasks))
	}
	c.name = min(c.name, 24)
	return c
}

// projectRow draws one project: the name, a progress bar, the percent done,
// the done and total counts, then the open work. The selection is drawn the
// same way as a task row.
func (m Model) projectRow(p store.Project, cols projectCols, selected bool) string {
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
	nameStyle := styleProject
	if p.OpenTasks() == 0 {
		nameStyle = styleDim
	}
	filled := int(p.Done()*barWidth + 0.5)
	if p.TasksResolved > 0 && filled == 0 {
		filled = 1 // any progress shows
	}
	if p.OpenTasks() > 0 && filled == barWidth {
		filled = barWidth - 1 // not done until it is done
	}

	line := gutter.Render(bar) + sp +
		seg(nameStyle, fmt.Sprintf("%-*s", cols.name, ansi.Truncate(p.Name, cols.name, "…"))) + sp +
		seg(styleBarDone, strings.Repeat("━", filled)) +
		seg(styleDim, strings.Repeat("━", barWidth-filled)) + sp +
		seg(lipgloss.NewStyle(), fmt.Sprintf("%3d%%", int(p.Done()*100))) + sp +
		seg(styleDim, fmt.Sprintf("%7s", fmt.Sprintf("%d/%d", p.TasksResolved, p.Tasks))) + sp + sp

	// cell pads an item to its column, or fills the column with blanks when
	// this project has none of it.
	cell := func(st lipgloss.Style, format string, n, width int) string {
		if width == 0 {
			return ""
		}
		text := ""
		if n > 0 {
			text = fmt.Sprintf(format, n)
		}
		return sp + sp + seg(st, text+strings.Repeat(" ", width-ansi.StringWidth(text)))
	}
	if p.OpenTasks() > 0 {
		line += seg(priorityStyle(p.Priority), priorityMark(p.Priority)) + sp +
			seg(lipgloss.NewStyle(), fmt.Sprintf("%*s", cols.open, fmt.Sprintf("%d open", p.OpenTasks())))
	} else {
		line += seg(styleDim, "✓"+fmt.Sprintf(" %-*s", cols.open, "finished"))
	}
	line += cell(styleActive, "▶ %d", p.ActiveTasks, cols.active) +
		cell(stylePaused, "‖ %d", p.PausedTasks, cols.paused) +
		cell(styleOverdue, "%d late", p.OverdueTasks, cols.late)
	if !p.Resolved.IsZero() {
		line += sp + sp + seg(styleDim, shortAge(p.Resolved, m.now()))
	}
	return fill(line, m.width, base)
}

// shortAge is a compact age such as "4h ago" or "3d ago".
func shortAge(at, now time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return at.Format("Jan 2006")
}

// dots draws page dots for any paged list. The task list and the project
// view share it, so both look the same.
func (m Model) dots(total, perPage, page int) string {
	p := paginator.New()
	p.Type = paginator.Dots // New starts in the "2/5" mode
	p.PerPage = perPage
	p.SetTotalPages(total)
	p.Page = page
	p.ActiveDot = styleTitle.Render("•")
	p.InactiveDot = styleDim.Render("•")
	if 2+p.TotalPages > m.width {
		p.Type = paginator.Arabic
		p.ArabicFormat = "%d/%d"
	}
	return fit("  "+p.View(), m.width)
}

// projectWord reads a "project:name" filter word. An empty name matches the
// tasks without a project, as "project:" clears a project in dstask.
func projectWord(word string) (string, bool) {
	return strings.CutPrefix(word, "project:")
}
