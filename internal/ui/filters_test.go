package ui

import (
	"strings"
	"testing"
)

func TestBackspaceDropsTheLastFilterWord(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("/"))
	m = drive(t, m, append(typed("the garage"), key("enter"))...)
	if len(m.visible) != 1 {
		t.Fatalf("the filter should match #9 only: %+v", m.visible)
	}
	m = drive(t, m, key("backspace"))
	if m.filter != "the" || len(m.visible) != 3 {
		t.Errorf("one backspace drops %q: filter=%q visible=%d", "garage", m.filter, len(m.visible))
	}
	m = drive(t, m, key("backspace"))
	if m.filter != "" {
		t.Errorf("a second backspace drops the last word: filter=%q", m.filter)
	}
}

func TestBackspaceDropsTheActiveFilterAfterTheWords(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("A"), key("/"))
	m = drive(t, m, append(typed("docs"), key("enter"))...)
	m = drive(t, m, key("backspace"))
	if m.filter != "" || !m.activeOnly {
		t.Fatalf("the word goes first: filter=%q active=%v", m.filter, m.activeOnly)
	}
	m = drive(t, m, key("backspace"))
	if m.activeOnly || len(m.visible) != 3 {
		t.Errorf("the active filter goes second: active=%v visible=%d", m.activeOnly, len(m.visible))
	}
}

func TestBackspaceWithNoFilterDoesNothing(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("j"), key("backspace"))
	if m.cursor != 1 || m.status != "" || m.mode != modeList {
		t.Errorf("no filter, no change: cursor=%d status=%q", m.cursor, m.status)
	}
}

func TestBackspaceKeepsTheSelectedTask(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("/"))
	m = drive(t, m, append(typed("tidy the"), key("enter"))...)
	m = drive(t, m, key("backspace"))
	if got, _ := m.selected(); got.ID != 9 {
		t.Errorf("the selection should stay on #9 when the list widens, got #%d", got.ID)
	}
}

func TestBackspaceFromTheNoMatchNotice(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("/"))
	m = drive(t, m, append(typed("garage zzzz"), key("enter"))...)
	if !m.fallback {
		t.Fatalf("a word that matches nothing shows every task")
	}
	m = drive(t, m, key("backspace"))
	if m.fallback || len(m.visible) != 1 {
		t.Errorf("dropping the bad word leaves the garage task: fallback=%v visible=%d", m.fallback, len(m.visible))
	}
}

func TestBackspaceInThePromptEditsTheText(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	m = drive(t, m, key("/"))
	m = drive(t, m, append(typed("garage"), key("enter"))...)
	m = drive(t, m, key("/"), key("backspace"))
	// The prompt filters as you type, so the filter follows the text.
	if m.mode != modePrompt || m.filter != "garag" || m.input.Value() != "garag" {
		t.Errorf("in the prompt backspace edits the text: mode=%v filter=%q input=%q", m.mode, m.filter, m.input.Value())
	}
}

func TestHeaderShowsEachFilterAndTheKey(t *testing.T) {
	m := start(t, &fake{open: tasks()})
	if strings.Contains(m.View(), "⌫") {
		t.Errorf("no filter, no hint")
	}
	m = drive(t, m, key("A"), key("/"))
	m = drive(t, m, append(typed("project:herdr docs"), key("enter"))...)
	header := strings.Split(m.View(), "\n")[0]
	for _, want := range []string{"▶ active only", "project:herdr", "docs", "⌫"} {
		if !strings.Contains(header, want) {
			t.Errorf("header lacks %q: %q", want, header)
		}
	}
	if strings.Count(header, "⌫") != 1 || strings.Index(header, "⌫") < strings.Index(header, "docs") {
		t.Errorf("only the last filter carries the key: %q", header)
	}
}
