package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/config"
	"larascope/internal/conns"
	"strings"
	"testing"
	"time"
)

func TestConnectionsLoadStateAndBody(t *testing.T) {
	m, cmd := updateWithKey(t, New(), runeKey("3"))
	if !m.conns.loading || cmd == nil {
		t.Fatalf("loading=%v cmd=%v", m.conns.loading, cmd)
	}
	_, cmd = updateWithKey(t, m, runeKey("3"))
	if cmd != nil {
		t.Fatal("duplicate load")
	}
	updated, _ := m.Update(connsLoadedMsg{results: []conns.Result{{Name: "DB", Target: "x", OK: true, Latency: 2 * time.Millisecond}, {Name: "bad", Err: "s3cr3t"}}})
	m = updated.(Model)
	if m.conns.loading || m.conns.checked.IsZero() {
		t.Fatal("load not stored")
	}
	m.settings.DBPassword = "s3cr3t"
	view := m.View()
	for _, want := range []string{"Last check:", "● up", "2ms", "***"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q: %s", want, view)
		}
	}
	if strings.Contains(view, "s3cr3t") {
		t.Fatal("secret shown")
	}
	m.conns.loading = true
	m.conns.results = nil
	if got := m.connsBody(); got != "Checking…" {
		t.Fatal(got)
	}
}
func TestConnectionTickAndTabLoad(t *testing.T) {
	m := New()
	m.active = TabJobs
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := updated.(Model)
	if got.active != TabConnections || !got.conns.loading || cmd == nil {
		t.Fatal("tab did not load")
	}
	got.conns.loading = false
	got.conns.connsTicking = true
	updated, cmd = got.Update(connsTickMsg{})
	if cmd == nil || !updated.(Model).conns.loading {
		t.Fatal("tick did not reload")
	}
	got.active = TabLogs
	updated, cmd = got.Update(connsTickMsg{})
	if cmd != nil || updated.(Model).conns.connsTicking {
		t.Fatal("inactive tick remained")
	}
}

func TestConnectionsReloadKey(t *testing.T) {
	m := New()
	m.active = TabConnections
	m.settings = config.Settings{}
	m, cmd := updateWithKey(t, m, runeKey("r"))
	if !m.conns.loading || cmd == nil {
		t.Fatal("first reload")
	}
	m, cmd = updateWithKey(t, m, runeKey("r"))
	if cmd != nil || !m.conns.loading {
		t.Fatal("duplicate reload")
	}
	updated, _ := m.Update(connsLoadedMsg{})
	m = updated.(Model)
	if m.conns.loading {
		t.Fatal("loaded")
	}
	m, cmd = updateWithKey(t, m, runeKey("r"))
	if cmd == nil || !m.conns.loading {
		t.Fatal("reload after result")
	}
	for _, tab := range []Tab{TabLogs, TabJobs} {
		m.active = tab
		m.conns.loading = false
		m, _ = updateWithKey(t, m, runeKey("r"))
		if m.conns.loading {
			t.Fatal("reload outside connections")
		}
	}
}

func TestConnectionsShiftTabAndNumberKeys(t *testing.T) {
	m := New()
	m.active = TabJobs
	m.settings = config.Settings{}
	m, cmd := updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.active != TabLogs || cmd != nil {
		t.Fatal("jobs shift tab")
	}
	m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.active != TabConnections || !m.conns.loading || !m.conns.connsTicking || cmd == nil {
		t.Fatal("logs shift tab")
	}
	for _, key := range []string{"1", "2"} {
		m.conns.loading = false
		m, _ = updateWithKey(t, m, runeKey(key))
		if m.conns.loading {
			t.Fatalf("%s loaded", key)
		}
	}
}

func TestConnectionsTickRearmSingle(t *testing.T) {
	m := New()
	m.active = TabJobs
	m.settings = config.Settings{}
	m, cmd := updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil || !m.conns.connsTicking {
		t.Fatal("initial entry")
	}
	m.active = TabLogs
	m.conns.loading = false
	m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if cmd == nil || !m.conns.connsTicking {
		t.Fatal("reentry")
	}
	m.conns.loading = true
	updated, cmd := m.Update(connsTickMsg{})
	if cmd == nil || !updated.(Model).conns.loading {
		t.Fatal("tick while loading")
	}
	m.active = TabLogs
	updated, cmd = m.Update(connsTickMsg{})
	m = updated.(Model)
	if cmd != nil || m.conns.connsTicking {
		t.Fatal("inactive tick")
	}
	m, _ = updateWithKey(t, m, runeKey("3"))
	if !m.conns.connsTicking {
		t.Fatal("tick not restarted")
	}
}

func TestConnectionsLoadedReplacesAndTimestamps(t *testing.T) {
	m := New()
	m.active = TabConnections
	m.conns.loading = true
	updated, _ := m.Update(connsLoadedMsg{results: []conns.Result{{Name: "one"}}})
	m = updated.(Model)
	first := m.conns.checked
	time.Sleep(time.Millisecond)
	m.conns.loading = true
	updated, _ = m.Update(connsLoadedMsg{results: []conns.Result{{Name: "two"}}})
	m = updated.(Model)
	if m.conns.loading || len(m.conns.results) != 1 || m.conns.results[0].Name != "two" || !m.conns.checked.After(first) {
		t.Fatal("replacement or timestamp")
	}
}

func TestConnectionsBodyRendering(t *testing.T) {
	m := New()
	m.active = TabConnections
	if m.connsBody() != "No connections detected yet" {
		t.Fatal("empty")
	}
	m.conns.loading = true
	if m.connsBody() != "Checking…" {
		t.Fatal("loading")
	}
	m.conns.results = []conns.Result{{Name: "first", Target: "a", OK: true, Latency: 0}, {Name: "second", Target: "b", OK: true, Latency: 250 * time.Millisecond}, {Name: "third", Target: "c", OK: true, Latency: 1500 * time.Millisecond}}
	m.conns.checked = time.Now()
	v := m.connsBody()
	for _, want := range []string{"(refreshing…)", "first", "<1ms", "250ms", "1.5s"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Index(v, "first") > strings.Index(v, "second") {
		t.Fatal("order")
	}
	m.conns.results = []conns.Result{{Name: "downname", Target: "downhost", Err: "broken"}}
	v = m.connsBody()
	for _, want := range []string{"○ down", "downname", "downhost", "broken"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing down %q", want)
		}
	}
	m.settings = config.Settings{DBUsername: "user", DBPassword: "s3cr3t", DBHost: "host", DBDatabase: "db"}
	dsn := m.settings.DSN()
	m.conns.results[0].Err = "dial " + dsn + " s3cr3t"
	v = m.connsBody()
	if strings.Contains(v, "s3cr3t") || strings.Contains(v, dsn) {
		t.Fatal("secret exposed")
	}
}

func TestConnectionsView(t *testing.T) {
	m := New()
	m.active = TabConnections
	m.conns.results = []conns.Result{{Name: "DB", Target: "host", OK: true}}
	v := m.View()
	if !strings.Contains(v, "DB") || !strings.Contains(v, "Connections") || strings.Contains(v, "No log file detected yet") {
		t.Fatal("connections view")
	}
}

func TestConnectionsWindowAndUnlimitedWidth(t *testing.T) {
	m := New()
	m.active = TabConnections
	m.width, m.height = 80, 12
	m.conns.results = make([]conns.Result, 30)
	for i := range m.conns.results {
		m.conns.results[i] = conns.Result{Name: fmt.Sprintf("connection-%d", i), Target: "host", OK: true}
	}
	v := m.View()
	if len(strings.Split(v, "\n")) > 12 || !strings.Contains(v, "rows 1–") || !strings.Contains(v, "of 30") {
		t.Fatalf("connection window missing or too tall: %q", v)
	}
	m.width, m.height = 0, 0
	m.conns.results[0].Err = strings.Repeat("x", 150)
	v = m.View()
	if !strings.Contains(v, m.conns.results[29].Name) || !strings.Contains(v, strings.Repeat("x", 150)) {
		t.Fatal("unlimited connections were clipped")
	}
}
