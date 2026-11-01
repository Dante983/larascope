package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Dante983/larascope/internal/config"
	"github.com/Dante983/larascope/internal/logs"
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
	for _, want := range []Tab{TabJobs, TabConnections, TabOverview, TabLogs} {
		m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
		if m.active != want {
			t.Fatalf("active tab = %d, want %d", m.active, want)
		}
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.active != TabOverview {
		t.Fatalf("active tab = %d, want %d", m.active, TabOverview)
	}
}

func TestNumberKeysSelectTabs(t *testing.T) {
	for key, want := range map[string]Tab{"1": TabLogs, "2": TabJobs, "3": TabConnections, "4": TabOverview} {
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
			want: "Not a Laravel project (no artisan found) · try: larascope --demo",
		},
		{
			name: "wrapped not Laravel",
			err:  fmt.Errorf("load config: %w", config.ErrNotLaravel),
			want: "Not a Laravel project (no artisan found) · try: larascope --demo",
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

func TestLogCursorJumpsToFirstAndLastEntry(t *testing.T) {
	m := New().WithLogs(logEntries(30), nil)

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.cursor != 0 {
		t.Fatalf("cursor after g = %d, want 0", m.cursor)
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.cursor != 29 {
		t.Errorf("cursor after G = %d, want 29", m.cursor)
	}
}

func TestLogCursorPagesAndClamps(t *testing.T) {
	m := New().WithLogs(logEntries(50), nil)

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.cursor != 29 {
		t.Fatalf("cursor after pgup = %d, want 29", m.cursor)
	}

	for range 2 {
		m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	}
	if m.cursor != 0 {
		t.Fatalf("cursor after three pgup keys = %d, want 0", m.cursor)
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.cursor != 20 {
		t.Errorf("cursor after pgdown = %d, want 20", m.cursor)
	}
}

func TestLogCursorPageDownAtLastEntryStaysAtLast(t *testing.T) {
	m := New().WithLogs(logEntries(50), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.cursor != 49 {
		t.Errorf("cursor after pgdown at last entry = %d, want 49", m.cursor)
	}
}

func TestLogCursorPagesClampWithFewerThanOnePage(t *testing.T) {
	m := New().WithLogs(logEntries(5), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.cursor != 0 {
		t.Fatalf("cursor after pgup = %d, want 0", m.cursor)
	}

	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.cursor != 4 {
		t.Errorf("cursor after pgdown = %d, want 4", m.cursor)
	}
}

func TestLogCursorPageKeysWithEmptyEntries(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyPgUp, tea.KeyPgDown} {
		t.Run(tea.KeyMsg{Type: key}.String(), func(t *testing.T) {
			m := New().WithLogs(nil, nil)
			m, _ = updateWithKey(t, m, tea.KeyMsg{Type: key})
			if m.cursor != 0 {
				t.Errorf("cursor after %s with no entries = %d, want 0", tea.KeyMsg{Type: key}.String(), m.cursor)
			}
		})
	}
}

func TestLogCursorPageKeyIgnoredInDetail(t *testing.T) {
	m := New().WithLogs(logEntries(50), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.cursor != 49 {
		t.Errorf("cursor after pgup in detail = %d, want 49", m.cursor)
	}
}

func TestHelpListsLogCursorPageKeys(t *testing.T) {
	m, _ := updateWithKey(t, New(), runeKey("?"))
	if !strings.Contains(m.View(), "pgup/pgdown: page up/down") {
		t.Error("help view does not list pgup/pgdown")
	}
	if !strings.Contains(m.View(), "r: reload log file") {
		t.Error("help view does not list reload log file")
	}
}

func TestReloadKeyRequiresLogRoot(t *testing.T) {
	_, cmd := updateWithKey(t, New(), runeKey("r"))
	if cmd != nil {
		t.Error("r with an empty Root returned a command")
	}

	m := New().WithConfig(config.Settings{Root: t.TempDir()}, nil)
	_, cmd = updateWithKey(t, m, runeKey("r"))
	if cmd == nil {
		t.Error("r with a Root returned a nil command")
	}
}

func TestReloadLogsCursorBehavior(t *testing.T) {
	tests := []struct {
		name       string
		model      Model
		entries    []logs.Entry
		wantCursor int
	}{
		{
			name:       "last entry follows new tail",
			model:      New().WithLogs(logEntries(2), nil),
			entries:    logEntries(3),
			wantCursor: 2,
		},
		{
			name:       "other entry stays put",
			model:      New().WithLogs(logEntries(2), nil).jumpCursor(0),
			entries:    logEntries(3),
			wantCursor: 0,
		},
		{
			name:       "cursor clamps when entries shrink",
			model:      New().WithLogs(logEntries(5), nil),
			entries:    logEntries(2),
			wantCursor: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, cmd := tt.model.Update(logsLoadedMsg{entries: tt.entries})
			if cmd != nil {
				t.Error("reload message returned a command")
			}
			if got := updated.(Model).cursor; got != tt.wantCursor {
				t.Errorf("cursor = %d, want %d", got, tt.wantCursor)
			}
		})
	}
}

func TestReloadLogsErrorKeepsEntries(t *testing.T) {
	m := New().WithLogs(logEntries(2), nil)
	wantErr := errors.New("reload failed")
	updated, _ := m.Update(logsLoadedMsg{err: wantErr})
	got := updated.(Model)

	if !reflect.DeepEqual(got.entries, m.entries) {
		t.Errorf("entries = %#v, want %#v", got.entries, m.entries)
	}
	if !errors.Is(got.logErr, wantErr) {
		t.Errorf("logErr = %v, want %v", got.logErr, wantErr)
	}
}

func TestReloadLogsEmptyEntriesClosesDetail(t *testing.T) {
	m := New().WithLogs(logEntries(1), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	updated, _ := m.Update(logsLoadedMsg{entries: []logs.Entry{}})
	if updated.(Model).detail {
		t.Error("detail = true after reload with no entries")
	}
}

func TestLogCursorJumpKeysWithEmptyEntries(t *testing.T) {
	for _, key := range []string{"g", "G"} {
		t.Run(key, func(t *testing.T) {
			m := New().WithLogs(nil, nil)
			m, _ = updateWithKey(t, m, runeKey(key))
			if m.cursor != 0 {
				t.Errorf("cursor after %s with no entries = %d, want 0", key, m.cursor)
			}
		})
	}
}

func TestLogCursorJumpKeyIgnoredInDetail(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateWithKey(t, m, runeKey("g"))
	if m.cursor != 2 {
		t.Errorf("cursor after g in detail = %d, want 2", m.cursor)
	}
}

func TestLogCursorJumpKeysUpdateVisibleEntries(t *testing.T) {
	m := New().WithLogs(logEntries(30), nil)

	m, _ = updateWithKey(t, m, runeKey("g"))
	view := m.View()
	for _, want := range []string{"msg0", "1–"} {
		if !strings.Contains(view, want) {
			t.Errorf("view after g does not contain %q", want)
		}
	}

	m, _ = updateWithKey(t, m, runeKey("G"))
	if !strings.Contains(m.View(), "of 30") {
		t.Error("view after G does not contain row count")
	}
}

func TestLogCursorKeysIgnoredOutsideLogsAndWithEmptyEntries(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)
	m, _ = updateWithKey(t, m, runeKey("2"))
	m, _ = updateWithKey(t, m, runeKey("k"))
	if m.cursor != 2 {
		t.Errorf("cursor on Jobs tab = %d, want 2", m.cursor)
	}
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.cursor != 2 {
		t.Errorf("cursor after pgup on Jobs tab = %d, want 2", m.cursor)
	}

	help := New().WithLogs(logEntries(3), nil)
	help, _ = updateWithKey(t, help, runeKey("?"))
	help, _ = updateWithKey(t, help, tea.KeyMsg{Type: tea.KeyPgUp})
	if help.cursor != 2 {
		t.Errorf("cursor after pgup in help = %d, want 2", help.cursor)
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

func TestLogDetailCursorKeysMoveAndClamp(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	m, _ = updateWithKey(t, m, runeKey("k"))
	if m.cursor != 1 {
		t.Fatalf("cursor after k in detail = %d, want 1", m.cursor)
	}
	if !m.detail {
		t.Fatal("detail = false after k in detail")
	}
	if !strings.Contains(m.View(), "msg1") {
		t.Error("detail view after k does not contain msg1")
	}

	for range 3 {
		m, _ = updateWithKey(t, m, runeKey("j"))
	}
	if m.cursor != 2 {
		t.Errorf("cursor after j past last entry in detail = %d, want 2", m.cursor)
	}
	if !m.detail {
		t.Error("detail = false after j in detail")
	}

	for range 3 {
		m, _ = updateWithKey(t, m, runeKey("k"))
	}
	if m.cursor != 0 {
		t.Errorf("cursor after k at first entry in detail = %d, want 0", m.cursor)
	}
}

func TestLogDetailJumpAndPageKeysDoNotMoveCursor(t *testing.T) {
	m := New().WithLogs(logEntries(3), nil)
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateWithKey(t, m, runeKey("k"))

	for _, key := range []tea.KeyMsg{
		runeKey("g"),
		runeKey("G"),
		{Type: tea.KeyPgUp},
	} {
		m, _ = updateWithKey(t, m, key)
		if m.cursor != 1 {
			t.Errorf("cursor after %q in detail = %d, want 1", key.String(), m.cursor)
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
