package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Dante983/larascope/internal/config"
	"github.com/Dante983/larascope/internal/conns"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type connsState struct {
	results               []conns.Result
	loading, connsTicking bool
	checked               time.Time
	cursor                int
}
type connsLoadedMsg struct{ results []conns.Result }
type connsTickMsg struct{}

func loadConnsCmd(s config.Settings) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return connsLoadedMsg{conns.Run(ctx, conns.FromSettings(s), s.Secrets()...)}
	}
}
func connsTick() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg { return connsTickMsg{} })
}
func (m *Model) startConnsLoad() tea.Cmd {
	if !m.liveAllowed() {
		return nil
	}
	if m.conns.loading {
		return nil
	}
	m.conns.loading = true
	return loadConnsCmd(m.settings)
}
func (m Model) connsBody() string {
	if m.conns.loading && len(m.conns.results) == 0 {
		return "Checking…"
	}
	if len(m.conns.results) == 0 {
		return "No connections detected yet"
	}
	up, down := 0, 0
	for _, r := range m.conns.results {
		if r.OK {
			up++
		} else {
			down++
		}
	}
	rows := make([][]Cell, len(m.conns.results))
	for i, r := range m.conns.results {
		status, st := statusDot(r.OK)
		latency := conns.FormatLatency(r.Latency)
		var lst *lipgloss.Style
		if r.Latency > 500*time.Millisecond {
			x := lipgloss.NewStyle().Foreground(colWarn)
			lst = &x
		}
		rows[i] = []Cell{{Text: status, Style: &st}, {Text: sanitizeLine(r.Name)}, {Text: sanitizeLine(r.Target)}, {Text: latency, Style: lst}, {Text: sanitizeLine(config.Redact(r.Err, m.settings.Secrets()...))}}
	}
	t := Table{Cols: []Column{{"STATUS", 6, 9, false, AlignLeft, 0}, {"NAME", 8, 16, false, AlignLeft, 0}, {"TARGET", 8, 0, true, AlignLeft, 0}, {"LATENCY", 5, 0, false, AlignRight, 2}, {"ERROR", 10, 0, false, AlignLeft, 1}}, Rows: rows, Cursor: m.conns.cursor, Width: m.clipWidth()}
	lines, s, e := t.Render(max(1, m.bodyHeight()-2))
	right := ""
	if len(rows) > 0 {
		right = fmt.Sprintf("%d–%d of %d", s+1, e, len(rows))
	}
	refresh := "auto-refresh 10s"
	if m.demo {
		refresh = "sample data"
	}
	return strings.Join(append([]string{metaLine(fmt.Sprintf("%d up · %d down · checked %s · %s", up, down, m.conns.checked.Format("15:04:05"), refresh), right, m.clipWidth())}, lines...), "\n")
}
