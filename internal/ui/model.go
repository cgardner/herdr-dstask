// Package ui is the Bubble Tea interface: a task list, a detail view, and a
// one-line prompt for the changes that take text.
package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/naggie/dstask"
)

// Backend is the part of the task store the UI uses. store.Store satisfies
// it, and a test supplies a fake.
type Backend interface {
	Open() ([]dstask.Task, error)
	Resolved() ([]dstask.Task, error)
	ContextString() string
	SetIgnoreContext(bool)
	Done(id int) error
	Start(id int) error
	Stop(id int) error
	Remove(id int) error
	Modify(id int, modifiers string) error
	Note(id int, text string) error
	Add(input string) error
	Undo() error
	EditTask(id int) (Editor, error)
	EditNotes(id int) (Editor, error)
}

// Editor is a task open in a temporary file. The UI hands Command to the
// terminal and then saves the result with Apply, or drops it with Discard.
type Editor interface {
	Command() *exec.Cmd
	Apply() error
	Discard()
}

type mode int

const (
	modeList mode = iota
	modeDetail
	modePrompt
	modeConfirm
	modeHelp
)

type promptKind int

const (
	promptFilter promptKind = iota
	promptModify
	promptNote
	promptAdd
)

// Model is the whole UI state.
type Model struct {
	backend Backend

	showResolved  bool
	ignoreContext bool
	context       string

	all     []dstask.Task
	visible []dstask.Task
	filter  string

	cursor int
	offset int
	width  int
	height int

	mode   mode
	back   mode // the mode a prompt, confirm or help returns to
	prompt promptKind
	input  textinput.Model
	detail viewport.Model

	// target is the task a prompt or confirm acts on, fixed when it opens so
	// a reload underneath cannot redirect the change to another task.
	target dstask.Task

	status    string
	statusErr bool
	loading   bool

	now func() time.Time
}

// New builds the model. The first load starts from Init.
func New(b Backend) Model {
	in := textinput.New()
	in.Prompt = "› "
	in.CharLimit = 2000
	return Model{backend: b, input: in, now: time.Now, loading: true, width: 80, height: 24}
}

// Messages.
type (
	loadedMsg struct {
		tasks   []dstask.Task
		context string
		err     error
	}
	actionMsg struct {
		ok  string
		err error
	}
)

// Init loads the first listing.
func (m Model) Init() tea.Cmd { return m.load() }

func (m Model) load() tea.Cmd {
	b, resolved := m.backend, m.showResolved
	return func() tea.Msg {
		ctx := b.ContextString()
		var tasks []dstask.Task
		var err error
		if resolved {
			tasks, err = b.Resolved()
		} else {
			tasks, err = b.Open()
		}
		return loadedMsg{tasks: tasks, context: ctx, err: err}
	}
}

// act runs a change in the background and reports it with the given text.
func act(ok string, fn func() error) tea.Cmd {
	return func() tea.Msg { return actionMsg{ok: ok, err: fn()} }
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(10, msg.Width-6)
		m.detail.Width, m.detail.Height = msg.Width, max(1, msg.Height-3)
		if m.mode == modeDetail || m.back == modeDetail {
			m.refreshDetail()
		}
		m.clamp()
		return m, nil

	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		m.context = msg.context
		m.setTasks(msg.tasks)
		return m, nil

	case actionMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		m.setStatus(msg.ok)
		m.loading = true
		return m, m.load()

	case tea.KeyMsg:
		// A status line reports the last action until the next key.
		m.status, m.statusErr = "", false
		switch m.mode {
		case modePrompt:
			return m.updatePrompt(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modeHelp:
			m.mode = m.back
			return m, nil
		case modeDetail:
			return m.updateDetail(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

// setTasks installs a fresh listing and keeps the cursor on the same task
// when that task is still present.
func (m *Model) setTasks(tasks []dstask.Task) {
	keep := ""
	if t, ok := m.selected(); ok {
		keep = t.UUID
	}
	m.all = tasks
	m.applyFilter()
	for i, t := range m.visible {
		if t.UUID == keep {
			m.cursor = i
			break
		}
	}
	m.clamp()
	if m.mode == modeDetail {
		if t, ok := m.selected(); ok && t.UUID == m.target.UUID {
			m.target = t
			m.refreshDetail()
		} else {
			m.mode = modeList
		}
	}
}

func (m *Model) applyFilter() {
	m.visible = m.visible[:0:0]
	for _, t := range m.all {
		if m.filter == "" || matches(t, m.filter) {
			m.visible = append(m.visible, t)
		}
	}
	m.clamp()
}

// matches reports whether every word of the filter occurs in the summary,
// project, tags, notes, priority or status, ignoring case.
func matches(t dstask.Task, filter string) bool {
	hay := strings.ToLower(strings.Join([]string{
		t.Summary, t.Project, strings.Join(t.Tags, " "), t.Notes, t.Priority, t.Status,
	}, "\n"))
	for _, word := range strings.Fields(strings.ToLower(filter)) {
		if !strings.Contains(hay, strings.TrimPrefix(word, "+")) {
			return false
		}
	}
	return true
}

func (m *Model) clamp() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	rows := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// listRows is the number of task rows that fit: the pane less the header,
// the column titles and the footer.
func (m Model) listRows() int { return max(1, m.height-4) }

func (m Model) selected() (dstask.Task, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return dstask.Task{}, false
	}
	return m.visible[m.cursor], true
}

func (m *Model) setStatus(s string) { m.status, m.statusErr = s, false }
func (m *Model) setErr(err error)   { m.status, m.statusErr = err.Error(), true }

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.applyFilter()
			return m, nil
		}
		return m, tea.Quit
	case "j", "down":
		m.cursor++
	case "k", "up":
		m.cursor--
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.visible) - 1
	case "ctrl+d", "pgdown":
		m.cursor += m.listRows() / 2
	case "ctrl+u", "pgup":
		m.cursor -= m.listRows() / 2
	case "enter", "l", "right":
		if t, ok := m.selected(); ok {
			m.target = t
			m.mode = modeDetail
			m.refreshDetail()
			m.detail.GotoTop()
		}
		return m, nil
	case "/":
		return m.openPrompt(promptFilter, m.filter)
	case "tab":
		m.showResolved = !m.showResolved
		m.filter = ""
		m.cursor, m.offset = 0, 0
		m.loading = true
		return m, m.load()
	case "c":
		m.ignoreContext = !m.ignoreContext
		m.backend.SetIgnoreContext(m.ignoreContext)
		m.loading = true
		return m, m.load()
	case "r":
		m.loading = true
		return m, m.load()
	case "a":
		return m.openPrompt(promptAdd, "")
	case "u":
		return m, act("undid the last change", m.backend.Undo)
	case "?":
		m.back, m.mode = modeList, modeHelp
		return m, nil
	default:
		return m.taskKey(msg)
	}
	m.clamp()
	return m, nil
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "h", "left", "backspace":
		m.mode = modeList
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "?":
		m.back, m.mode = modeDetail, modeHelp
		return m, nil
	case "a":
		return m.openPrompt(promptAdd, "")
	case "u":
		return m, act("undid the last change", m.backend.Undo)
	case "j", "k", "down", "up", "ctrl+d", "ctrl+u", "pgdown", "pgup", "g", "G", "home", "end":
		switch msg.String() {
		case "g", "home":
			m.detail.GotoTop()
		case "G", "end":
			m.detail.GotoBottom()
		default:
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	return m.taskKey(msg)
}

// taskKey handles the keys that change the selected task. They behave the
// same in the list and in the detail view.
func (m Model) taskKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t, ok := m.selected()
	if m.mode == modeDetail {
		t, ok = m.target, true
	}
	key := msg.String()
	if !ok || !strings.Contains("dsmnNex", key) || len(key) != 1 {
		return m, nil
	}
	// dstask gives a resolved task ID 0 and addresses tasks only by ID, so the
	// CLI offers no way to change one.
	if t.ID == 0 {
		m.setErr(errors.New("resolved tasks have no id, so dstask cannot change them (u undoes the last change)"))
		return m, nil
	}
	id := t.ID
	switch key {
	case "d":
		return m, act(fmt.Sprintf("resolved #%d", id), func() error { return m.backend.Done(id) })
	case "s":
		if t.Status == dstask.STATUS_ACTIVE {
			return m, act(fmt.Sprintf("stopped #%d", id), func() error { return m.backend.Stop(id) })
		}
		return m, act(fmt.Sprintf("started #%d", id), func() error { return m.backend.Start(id) })
	case "m":
		m.target = t
		return m.openPrompt(promptModify, "")
	case "n":
		m.target = t
		return m.openPrompt(promptNote, "")
	case "x":
		m.target = t
		m.back, m.mode = m.mode, modeConfirm
		return m, nil
	case "e":
		return m.runEditor(m.backend.EditTask, id, fmt.Sprintf("edited #%d", id))
	case "N":
		return m.runEditor(m.backend.EditNotes, id, fmt.Sprintf("edited the notes of #%d", id))
	}
	return m, nil
}

// runEditor suspends the UI, gives the terminal to $EDITOR, and saves the
// file when the editor exits cleanly.
func (m Model) runEditor(open func(int) (Editor, error), id int, ok string) (tea.Model, tea.Cmd) {
	ed, err := open(id)
	if err != nil {
		m.setErr(err)
		return m, nil
	}
	return m, tea.ExecProcess(ed.Command(), func(err error) tea.Msg {
		if err != nil {
			ed.Discard()
			return actionMsg{err: fmt.Errorf("editor: %w; nothing saved", err)}
		}
		return actionMsg{ok: ok, err: ed.Apply()}
	})
}

func (m Model) openPrompt(kind promptKind, value string) (tea.Model, tea.Cmd) {
	m.back, m.mode, m.prompt = m.mode, modePrompt, kind
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Placeholder = placeholders[kind]
	return m, m.input.Focus()
}

var placeholders = map[promptKind]string{
	promptFilter: "words to match in summary, project, tags or notes",
	promptModify: "+tag -tag project:name P1 due:friday",
	promptNote:   "text to append to the notes",
	promptAdd:    "+tag project:name P1 summary of the task",
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = m.back
		m.input.Blur()
		return m, nil
	case "enter":
		value := m.input.Value()
		m.mode = m.back
		m.input.Blur()
		id := m.target.ID
		switch m.prompt {
		case promptFilter:
			m.filter = strings.TrimSpace(value)
			m.cursor, m.offset = 0, 0
			m.applyFilter()
			return m, nil
		case promptModify:
			return m, act(fmt.Sprintf("modified #%d", id), func() error { return m.backend.Modify(id, value) })
		case promptNote:
			return m, act(fmt.Sprintf("noted #%d", id), func() error { return m.backend.Note(id, value) })
		case promptAdd:
			return m, act("added a task", func() error { return m.backend.Add(value) })
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.prompt == promptFilter {
		// The filter narrows the list as the user types.
		m.filter = strings.TrimSpace(m.input.Value())
		m.cursor, m.offset = 0, 0
		m.applyFilter()
	}
	return m, cmd
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = m.back
	if msg.String() != "y" {
		m.setStatus("kept #" + fmt.Sprint(m.target.ID))
		return m, nil
	}
	id := m.target.ID
	if m.mode == modeDetail {
		m.mode = modeList
	}
	return m, act(fmt.Sprintf("removed #%d", id), func() error { return m.backend.Remove(id) })
}
