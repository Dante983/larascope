package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/config"
	"larascope/internal/logs"
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

func TestStatusLine(t *testing.T) {
	fullSettings := config.Settings{
		Root:            "/srv/app",
		LogChannel:      "stack",
		QueueConnection: "redis",
		DBConnection:    "mysql",
		DBHost:          "127.0.0.1",
		DBPort:          "3306",
		DBDatabase:      "app",
	}

	tests := []struct {
		name     string
		settings config.Settings
		err      error
		want     string
	}{
		{
			name: "not Laravel",
			err:  config.ErrNotLaravel,
			want: "Not a Laravel project (no artisan found)",
		},
		{
			name: "wrapped not Laravel",
			err:  fmt.Errorf("load config: %w", config.ErrNotLaravel),
			want: "Not a Laravel project (no artisan found)",
		},
		{
			name: "generic error",
			err:  errors.New("permission denied"),
			want: "Config error: permission denied",
		},
		{
			name:     "full settings",
			settings: fullSettings,
			want:     "root: /srv/app  log: stack  queue: redis  db: mysql://127.0.0.1:3306/app",
		},
		{
			name:     "root only",
			settings: config.Settings{Root: "/srv/app"},
			want:     "root: /srv/app",
		},
		{
			name: "zero settings",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New().WithConfig(tt.settings, tt.err).statusLine(); got != tt.want {
				t.Errorf("statusLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestViewWithConfig(t *testing.T) {
	settings := config.Settings{
		Root:         "/srv/app",
		DBConnection: "mysql",
		DBHost:       "127.0.0.1",
		DBPort:       "3306",
		DBDatabase:   "app",
	}

	view := New().WithConfig(settings, nil).View()
	for _, want := range []string{"root: /srv/app", "mysql://127.0.0.1:3306/app"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q", want)
		}
	}
}

func TestLogsBody(t *testing.T) {
	tests := []struct {
		name    string
		model   Model
		want    []string
		notWant []string
	}{
		{
			name:  "not loaded",
			model: New(),
			want:  []string{"No log file detected yet"},
		},
		{
			name:  "missing file",
			model: New().WithLogs(nil, fmt.Errorf("x: %w", fs.ErrNotExist)),
			want:  []string{"No log file found"},
		},
		{
			name:  "load error",
			model: New().WithLogs(nil, errors.New("boom")),
			want:  []string{"Log error: boom"},
		},
		{
			name:  "no entries",
			model: New().WithLogs([]logs.Entry{}, nil),
			want:  []string{"Log file has no entries"},
		},
	}

	entries := make([]logs.Entry, 25)
	for i := range entries {
		entries[i] = logs.Entry{Header: logs.Header{
			Time:    time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
			Level:   "ERROR",
			Message: fmt.Sprintf("msg-%02d", i),
		}}
	}
	tests = append(tests, struct {
		name    string
		model   Model
		want    []string
		notWant []string
	}{
		name:    "last twenty entries",
		model:   New().WithLogs(entries, nil),
		want:    []string{"25 entries", "msg-24", "msg-05"},
		notWant: []string{"msg-04"},
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := tt.model.View()
			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Errorf("view does not contain %q", want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(view, notWant) {
					t.Errorf("view contains %q", notWant)
				}
			}
		})
	}
}

func TestWithLogsSelectsNewestEntry(t *testing.T) {
	entries := logEntries(3)
	m := New().WithLogs(entries, nil)
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.cursor)
	}
}

func TestLogCursorMovesAndClamps(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)

	for range 2 {
		m, _ = updateWithKey(t, m, runeKey("k"))
	}
	if m.cursor != 0 {
		t.Fatalf("cursor after two k keys = %d, want 0", m.cursor)
	}

	m, _ = updateWithKey(t, m, runeKey("k"))
	if m.cursor != 0 {
		t.Errorf("cursor after k at first entry = %d, want 0", m.cursor)
	}

	m, _ = updateWithKey(t, m, runeKey("j"))
	if m.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", m.cursor)
	}

	for range 3 {
		m, _ = updateWithKey(t, m, runeKey("j"))
	}
	if m.cursor != 2 {
		t.Errorf("cursor after j past last entry = %d, want 2", m.cursor)
	}
}

func TestLogCursorArrowKeys(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Fatalf("cursor after up = %d, want 1", m.cursor)
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("cursor after down = %d, want 2", m.cursor)
	}
}

func TestLogCursorScrollsList(t *testing.T) {
	m := New().WithLogs(logEntries(30), nil)
	view := m.View()
	if !strings.Contains(view, "msg29") || strings.Contains(view, "msg9") {
		t.Errorf("view at newest cursor did not show only the tail window")
	}

	for range 25 {
		m, _ = updateWithKey(t, m, runeKey("k"))
	}
	if m.cursor != 4 {
		t.Fatalf("cursor = %d, want 4", m.cursor)
	}

	view = m.View()
	if !strings.Contains(view, "msg4") || strings.Contains(view, "msg29") {
		t.Errorf("view at cursor 4 did not scroll to the cursor window")
	}
}

func TestLogCursorKeysIgnoredOutsideLogsAndWithEmptyEntries(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)
	m, _ = updateWithKey(t, m, runeKey("2"))
	m, _ = updateWithKey(t, m, runeKey("k"))
	if m.cursor != 2 {
		t.Errorf("cursor on Jobs tab = %d, want 2", m.cursor)
	}

	empty := New().WithLogs(nil, nil)
	empty, _ = updateWithKey(t, empty, runeKey("k"))
	if empty.cursor != 0 {
		t.Errorf("empty cursor = %d, want 0", empty.cursor)
	}
}

func TestLogDetailToggleAndEscape(t *testing.T) {
	entries := logEntries(2)
	entries[1].Env = "local"
	entries[1].Trace = []string{"trace line"}
	m := New().WithLogs(entries, nil)

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.detail {
		t.Fatal("detail = false after enter")
	}
	view := m.View()
	for _, want := range []string{"msg1", "trace line"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view does not contain %q", want)
		}
	}
	if strings.Contains(view, "entries  (") {
		t.Error("detail view contains list header")
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.detail {
		t.Fatal("detail = true after second enter")
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.detail {
		t.Fatal("detail = true after esc")
	}
}

func TestLogDetailCursorKeysDoNotMoveCursor(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	for _, key := range []string{"j", "k"} {
		m, _ = updateWithKey(t, m, runeKey(key))
		if m.cursor != 2 {
			t.Errorf("cursor after %q in detail = %d, want 2", key, m.cursor)
		}
	}
}

func TestLogDetailEnterIgnoredWithoutLogEntriesOrOutsideLogs(t *testing.T) {
	empty := New().WithLogs(nil, nil)
	empty, _ = updateWithKey(t, empty, tea.KeyMsg{Type: tea.KeyEnter})
	if empty.detail {
		t.Error("detail = true after enter with no entries")
	}

	m := New().WithLogs(logEntries(1), nil)
	m, _ = updateWithKey(t, m, runeKey("2"))
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.detail {
		t.Error("detail = true after enter on Jobs tab")
	}
}

func TestWithLogsResetsDetail(t *testing.T) {
	m := New().WithLogs(logEntries(1), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.detail {
		t.Fatal("detail = false after enter")
	}

	m = m.WithLogs(logEntries(1), nil)
	if m.detail {
		t.Error("detail = true after WithLogs")
	}
}

func TestLogDetailWithoutTraceHasNoTrailingBlankSection(t *testing.T) {
	m := New().WithLogs(logEntries(1), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if strings.HasSuffix(m.detailBody(), "\n\n") {
		t.Errorf("detailBody() = %q, ends with a blank section", m.detailBody())
	}
}

func logEntries(count int) []logs.Entry {
	entries := make([]logs.Entry, count)
	for i := range entries {
		entries[i] = logs.Entry{Header: logs.Header{
			Time:    time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
			Level:   "ERROR",
			Message: fmt.Sprintf("msg%d", i),
		}}
	}
	return entries
}
