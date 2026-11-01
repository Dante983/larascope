package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/config"
	"larascope/internal/conns"
	"strings"
	"time"
)

type connsState struct {
	results               []conns.Result
	loading, connsTicking bool
	checked               time.Time
}
type connsLoadedMsg struct{ results []conns.Result }
type connsTickMsg struct{}

func loadConnsCmd(s config.Settings) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return connsLoadedMsg{conns.Run(ctx, conns.FromSettings(s), s.DBPassword, s.DSN())}
	}
}
func connsTick() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg { return connsTickMsg{} })
}
func (m *Model) startConnsLoad() tea.Cmd {
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
	var a []string
	if !m.conns.checked.IsZero() {
		header := "Last check: " + m.conns.checked.Format("15:04:05")
		if m.conns.loading {
			header += " (refreshing…)"
		}
		a = append(a, header)
	}
	for _, r := range m.conns.results {
		v := "○ down"
		if r.OK {
			v = "● up"
		}
		line := v + "  " + r.Name + "  " + r.Target + "  " + conns.FormatLatency(r.Latency)
		if r.Err != "" {
			line += " " + config.Redact(r.Err, m.settings.DBPassword, m.settings.DSN())
		}
		a = append(a, line)
	}
	return strings.Join(a, "\n")
}
