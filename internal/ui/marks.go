package ui

import (
	"errors"
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/naggie/dstask"

	"github.com/cgardner/herdr-dstask/internal/store"
)

// toggleMark marks or unmarks the task under the cursor and moves down one
// row, so a run of tasks is marked with space, space, space.
func (m Model) toggleMark() (tea.Model, tea.Cmd) {
	t, ok := m.selected()
	if !ok {
		return m, nil
	}
	if m.showResolved {
		m.setErr(errors.New("resolved tasks cannot be changed, so they cannot be marked"))
		return m, nil
	}
	if m.marked == nil {
		m.marked = map[string]bool{}
	}
	if m.marked[t.UUID] {
		delete(m.marked, t.UUID)
	} else {
		m.marked[t.UUID] = true
	}
	if m.cursor < len(m.visible)-1 {
		m.cursor++
		m.clamp()
	}
	return m, nil
}

// toggleMarkAll marks every visible task, or unmarks them all when every one
// is already marked. Marks on hidden tasks are left alone.
func (m Model) toggleMarkAll() (tea.Model, tea.Cmd) {
	if m.showResolved {
		m.setErr(errors.New("resolved tasks cannot be changed, so they cannot be marked"))
		return m, nil
	}
	all := len(m.visible) > 0
	for _, t := range m.visible {
		all = all && m.marked[t.UUID]
	}
	if m.marked == nil {
		m.marked = map[string]bool{}
	}
	for _, t := range m.visible {
		if all {
			delete(m.marked, t.UUID)
		} else {
			m.marked[t.UUID] = true
		}
	}
	return m, nil
}

// pruneMarks drops marks on tasks that are no longer loaded, such as a task
// resolved in another pane. A change never reaches a task the list lost.
func (m *Model) pruneMarks() {
	if len(m.marked) == 0 {
		return
	}
	loaded := map[string]bool{}
	for _, t := range m.all {
		loaded[t.UUID] = true
	}
	for uuid := range m.marked {
		if !loaded[uuid] {
			delete(m.marked, uuid)
		}
	}
}

// markedTasks lists the marked tasks in ID order.
func (m Model) markedTasks() []dstask.Task {
	var out []dstask.Task
	for _, t := range m.all {
		if m.marked[t.UUID] {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// hiddenMarks counts marked tasks that the filters hide. The header shows
// it, so a change never reaches a task the user cannot see without warning.
func (m Model) hiddenMarks() int {
	visible := map[string]bool{}
	for _, t := range m.visible {
		visible[t.UUID] = true
	}
	n := 0
	for _, t := range m.markedTasks() {
		if !visible[t.UUID] {
			n++
		}
	}
	return n
}

// bulkKey applies d, s, m, n or x to the marked tasks. e and N open an
// editor on one file, so they stay single-task.
func (m Model) bulkKey(key string) (tea.Model, tea.Cmd) {
	tasks := m.markedTasks()
	refs := make([]store.Ref, len(tasks))
	for i, t := range tasks {
		refs[i] = store.Ref{ID: t.ID, UUID: t.UUID}
	}
	label := fmt.Sprintf("%d tasks", len(tasks))
	if len(tasks) == 1 {
		label = fmt.Sprintf("#%d", tasks[0].ID)
	}
	switch key {
	case "d":
		return m, actOn(true, "resolved "+label, func() error { return m.backend.DoneAll(refs) })
	case "s":
		// Start the marked tasks that are not active. When every one is
		// already active, stop them all instead, so s toggles as it does for
		// one task.
		var idle []store.Ref
		for i, t := range tasks {
			if t.Status != dstask.STATUS_ACTIVE {
				idle = append(idle, refs[i])
			}
		}
		if len(idle) == 0 {
			return m, actOn(true, "stopped "+label, func() error { return m.backend.StopAll(refs) })
		}
		started := fmt.Sprintf("started %d tasks", len(idle))
		if len(idle) == 1 {
			started = fmt.Sprintf("started #%d", idle[0].ID)
		}
		return m, actOn(true, started, func() error { return m.backend.StartAll(idle) })
	case "m", "n", "x":
		m.targets, m.targetLabel = refs, label
		return m.openTargeted(key)
	}
	m.setErr(errors.New("e and N edit one task: esc clears the marks"))
	return m, nil
}
