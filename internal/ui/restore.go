package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cgardner/herdr-dstask/internal/state"
)

// State describes the current view, for the next opening to restore.
//
// A task view, a prompt, a confirmation or the help screen is a short step
// on top of a list, so the state names the list behind it.
func (m Model) State() state.View {
	v := state.View{
		View:          "list",
		Filter:        m.filter,
		ActiveOnly:    m.activeOnly,
		ShowResolved:  m.showResolved,
		IgnoreContext: m.ignoreContext,
		ProjectSort:   m.psort.String(),
		ProjectFilter: m.pfilter,
		ShowFinished:  m.showFinished,
	}
	if m.mode == modeProjects || (m.mode != modeList && m.mode != modeDetail && m.back == modeProjects) {
		v.View = "projects"
	}
	if t, ok := m.selected(); ok {
		v.Task = t.UUID
	} else if m.wantTask != "" {
		v.Task = m.wantTask // the list had not loaded yet
	}
	if p, ok := m.selectedProject(); ok {
		v.Project = p.Name
	} else if m.wantProject != "" {
		v.Project = m.wantProject
	}
	return v
}

// Restore applies a saved view before the first load. The selected task and
// project are kept aside until their lists arrive, and are dropped quietly
// when they no longer exist.
func (m Model) Restore(v state.View) Model {
	m.filter = v.Filter
	m.activeOnly = v.ActiveOnly && !v.ShowResolved
	m.showResolved = v.ShowResolved
	m.ignoreContext = v.IgnoreContext
	m.backend.SetIgnoreContext(v.IgnoreContext)
	m.pfilter = v.ProjectFilter
	m.showFinished = v.ShowFinished
	m.psort = sortUrgency
	for s := projectSort(0); s < projectSorts; s++ {
		if s.String() == v.ProjectSort {
			m.psort = s
		}
	}
	m.wantTask, m.wantProject = v.Task, v.Project
	if v.View == "projects" {
		m.mode = modeProjects
		m.projectsLoading = true
	}
	return m
}

// initCmd loads the task list, and the projects too when the popup reopens
// on the project view.
func (m Model) initCmd() tea.Cmd {
	if m.mode == modeProjects {
		return tea.Batch(m.load(), m.loadProjects())
	}
	return m.load()
}
