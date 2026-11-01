package tui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	up, down := 0, 0
	for _, r := range m.conns.results {
		if r.OK {
			up++
		} else {
			down++
		}
	}
	a = append(a, fmt.Sprintf("%s · %s", badge(fmt.Sprintf("%d up", up), colOK), badge(fmt.Sprintf("%d down", down), colErr)), lipgloss.NewStyle().Foreground(colMuted).Render("auto-refresh 10s · r to refresh"), lipgloss.NewStyle().Foreground(colMuted).Bold(true).Render("STATUS  NAME  TARGET  "+func() string {
		if m.width == 0 || m.width >= 80 {
			return "LATENCY  "
		}
		return ""
	}()+"ERROR"))
	rows := len(m.conns.results)
	if m.height > 0 {
		rows = m.bodyHeight() - len(a)
		if len(m.conns.results) > rows {
			rows--
		}
		rows = max(1, rows)
	}
	end := min(len(m.conns.results), rows)
	for _, r := range m.conns.results[:end] {
		v := "○ down"
		if r.OK {
			v = "● up"
		}
		statusColor := colErr
		if r.OK {
			statusColor = colOK
		}
		line := lipgloss.NewStyle().Foreground(statusColor).Render(v) + "  " + padRight(sanitizeLine(r.Name), 12) + "  " + padRight(sanitizeLine(r.Target), 20)
		if m.width == 0 || m.width >= 80 {
			latency := conns.FormatLatency(r.Latency)
			c := colInfo
			if r.Latency > 500*time.Millisecond {
				c = colWarn
			}
			line += "  " + lipgloss.NewStyle().Foreground(c).Render(latency)
		}
		if r.Err != "" {
			err := sanitizeLine(config.Redact(r.Err, m.settings.DBPassword, m.settings.DSN()))
			if cw := m.clipWidth(); cw > 0 {
				err = truncateStrict(err, cw-lipgloss.Width(line)-2)
			}
			if err != "" {
				line += "  " + err
			}
		}
		a = append(a, truncate(line, m.clipWidth()))
	}
	if end < len(m.conns.results) {
		a = append(a, fmt.Sprintf("rows 1–%d of %d (enlarge terminal to see more)", end, len(m.conns.results)))
	}
	return strings.Join(a, "\n")
}
