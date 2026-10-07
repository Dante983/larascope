package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func updateWithKey(t *testing.T, m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func runeKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func TestTabNavigationWraps(t *testing.T) {
	m := New()
	for _, want := range []Tab{TabJobs, TabConnections, TabLogs} {
		m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
		if m.active != want {
			t.Fatalf("active tab = %d, want %d", m.active, want)
		}
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.active != TabConnections {
		t.Fatalf("active tab = %d, want %d", m.active, TabConnections)
	}
}

func TestNumberKeysSelectTabs(t *testing.T) {
	for key, want := range map[string]Tab{"1": TabLogs, "2": TabJobs, "3": TabConnections} {
		m, _ := updateWithKey(t, New(), runeKey(key))
		if m.active != want {
			t.Errorf("key %q selected tab %d, want %d", key, m.active, want)
		}
	}
}

func TestHelpKeyToggles(t *testing.T) {
	m, _ := updateWithKey(t, New(), runeKey("?"))
	if !m.showHelp {
		t.Fatal("showHelp = false after first toggle")
	}
	m, _ = updateWithKey(t, m, runeKey("?"))
	if m.showHelp {
		t.Fatal("showHelp = true after second toggle")
	}
}

func TestQuitKeyReturnsQuitCommand(t *testing.T) {
	_, cmd := updateWithKey(t, New(), runeKey("q"))
	if cmd == nil {
		t.Fatal("q returned a nil command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q command returned %T, want tea.QuitMsg", cmd())
	}
}

func TestViewWithZeroSize(t *testing.T) {
	view := New().View()
	for _, name := range tabNames {
		if !strings.Contains(view, name) {
			t.Errorf("view does not contain tab name %q", name)
		}
	}
}
