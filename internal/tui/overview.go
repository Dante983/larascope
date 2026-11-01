package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dante983/larascope/internal/config"
	"github.com/Dante983/larascope/internal/logs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type logSummary struct {
	total, errors, warnings, info, debug int
	firstTime, lastTime                  time.Time
	recentErrors                         []int
}

func summarizeLogs(entries []logs.Entry) logSummary {
	s := logSummary{total: len(entries)}
	if len(entries) > 0 {
		s.firstTime = entries[0].Time
		s.lastTime = entries[len(entries)-1].Time
	}
	for i, entry := range entries {
		switch strings.ToUpper(entry.Level) {
		case "ERROR", "CRITICAL", "ALERT", "EMERGENCY":
			s.errors++
		case "WARNING":
			s.warnings++
		case "INFO", "NOTICE":
			s.info++
		case "DEBUG":
			s.debug++
		}
		if len(s.recentErrors) < 5 {
			// Prepend while scanning file order, leaving newest entries first.
			level := strings.ToUpper(entry.Level)
			if level == "ERROR" || level == "CRITICAL" || level == "ALERT" || level == "EMERGENCY" {
				s.recentErrors = append([]int{i}, s.recentErrors...)
			}
		} else {
			// The summary is capped, but later errors may be newer than all saved ones.
			level := strings.ToUpper(entry.Level)
			if level == "ERROR" || level == "CRITICAL" || level == "ALERT" || level == "EMERGENCY" {
				s.recentErrors = append([]int{i}, s.recentErrors[:4]...)
			}
		}
	}
	return s
}

func (m Model) overviewBody() string {
	w := m.clipWidth()
	if m.demo {
		return m.overviewContent([]string{metaLine("Demo project · sample data", "", w)})
	}
	if errors.Is(m.cfgErr, config.ErrNotLaravel) {
		return metaLine("Not a Laravel project (no artisan found) · try: larascope --demo", "", w)
	}
	if m.cfgErr != nil {
		return metaLine("Config error: "+sanitizeLine(config.Redact(m.cfgErr.Error(), m.settings.Secrets()...)), "", w)
	}
	parts := []string{}
	if m.settings.Root != "" {
		parts = append(parts, "root: "+filepath.Base(m.settings.Root))
	}
	if m.settings.LogChannel != "" {
		parts = append(parts, "log: "+m.settings.LogChannel)
	}
	if m.settings.QueueConnection != "" {
		parts = append(parts, "queue: "+m.settings.QueueConnection)
	}
	if m.settings.DBConnection != "" || m.settings.DBHost != "" {
		db := m.settings.DBConnection + "://" + m.settings.DBHost
		if m.settings.DBPort != "" {
			db += ":" + m.settings.DBPort
		}
		db += "/" + m.settings.DBDatabase
		parts = append(parts, "db: "+db)
	}
	return m.overviewContent([]string{metaLine(sanitizeLine(strings.Join(parts, "  ")), "", w)})
}

func (m Model) overviewContent(lines []string) string {
	w := m.clipWidth()
	health, _, _ := (Table{Cols: []Column{{"AREA", 6, 9, false, AlignLeft, 0}, {"STATUS", 6, 9, false, AlignLeft, 0}, {"DETAIL", 8, 0, true, AlignLeft, 0}, {"KEY", 3, 6, false, AlignLeft, 3}}, Rows: m.overviewHealthRows(), Cursor: -1, Width: w}).Render(3)
	lines = append(lines, health...)
	if m.height == 0 || m.bodyHeight() >= 12 {
		if len(m.logSum.recentErrors) > 0 {
			lines = append(lines, metaLine("Recent errors (e: open in Logs)", "", w))
			rows := make([][]Cell, len(m.logSum.recentErrors))
			for i, idx := range m.logSum.recentErrors {
				e := m.entries[idx]
				tm := e.Time.Format("15:04:05")
				if m.width == 0 || m.width >= 80 {
					tm = e.Time.Format("2006-01-02 15:04:05")
				}
				st := levelStyle(e.Level)
				rows[i] = []Cell{{Text: tm}, {Text: sanitizeLine(e.Level), Style: &st}, {Text: sanitizeLine(strings.SplitN(e.Message, "\n", 2)[0])}}
			}
			recent, _, _ := (Table{Cols: []Column{{"TIME", 8, 19, false, AlignLeft, 3}, {"LEVEL", 5, 9, false, AlignLeft, 0}, {"MESSAGE", 10, 0, true, AlignLeft, 0}}, Rows: rows, Cursor: -1, Width: w}).Render(len(rows))
			lines = append(lines, recent...)
		} else if m.logLoaded && m.logErr == nil {
			lines = append(lines, "No errors in loaded log tail")
		}
	}
	if m.height == 0 || m.bodyHeight() >= 18 {
		down := make([]int, 0)
		for i, r := range m.conns.results {
			if !r.OK {
				down = append(down, i)
			}
		}
		if len(down) > 0 {
			lines = append(lines, metaLine("Services down", "", w))
			rows := make([][]Cell, min(3, len(down)))
			for i, idx := range down[:len(rows)] {
				r := m.conns.results[idx]
				rows[i] = []Cell{{Text: sanitizeLine(r.Name)}, {Text: sanitizeLine(r.Target)}, {Text: sanitizeLine(config.Redact(r.Err, m.settings.Secrets()...))}}
			}
			table, _, _ := (Table{Cols: []Column{{"NAME", 4, 16, false, AlignLeft, 0}, {"TARGET", 6, 0, false, AlignLeft, 2}, {"ERROR", 8, 0, true, AlignLeft, 0}}, Rows: rows, Cursor: -1, Width: w}).Render(len(rows))
			lines = append(lines, table...)
			if len(down) > len(rows) {
				lines = append(lines, lipgloss.NewStyle().Foreground(colMuted).Render(fmt.Sprintf("… +%d more", len(down)-len(rows))))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) overviewHealthRows() [][]Cell {
	muted := lipgloss.NewStyle().Foreground(colMuted)
	ok := lipgloss.NewStyle().Foreground(colOK)
	warn := lipgloss.NewStyle().Foreground(colWarn)
	err := lipgloss.NewStyle().Foreground(colErr)
	logsStatus, logsDetail, logsKey, logsStyle := "—", "No log file detected yet", "l/e", &muted
	if m.logLoaded {
		switch {
		case errors.Is(m.logErr, fs.ErrNotExist):
			logsStatus, logsDetail = "none", "No log file found (storage/logs/laravel.log)"
		case m.logErr != nil:
			logsStatus, logsDetail, logsStyle = "error", sanitizeLine(config.Redact(m.logErr.Error(), m.settings.Secrets()...)), &err
		case m.logSum.total == 0:
			logsStatus, logsDetail = "empty", "Log file has no entries"
		default:
			logsStatus, logsStyle = "● ok", &ok
			if m.logSum.errors > 0 {
				logsStatus, logsStyle = fmt.Sprintf("● %d err", m.logSum.errors), &err
			} else if m.logSum.warnings > 0 {
				logsStatus, logsStyle = "▲ warn", &warn
			}
			last := m.logSum.lastTime.Format("2006-01-02 15:04:05")
			if m.width > 0 && m.width < 80 {
				last = m.logSum.lastTime.Format("15:04:05")
			}
			logsDetail = fmt.Sprintf("%d entries · %d error · %d warn · last %s", m.logSum.total, m.logSum.errors, m.logSum.warnings, last)
		}
	}

	jobsStatus, jobsDetail, jobsKey, jobsStyle := "—", "Not loaded (press 2 or r)", "f/p", &muted
	if m.settings.Root == "" && !m.demo {
		jobsStatus, jobsDetail = "—", "No jobs detected yet"
	} else if m.jobs.loading && !m.jobs.loaded {
		jobsStatus, jobsDetail = "…", "Loading…"
	} else if m.jobs.loaded && m.jobs.err != nil {
		jobsStatus, jobsDetail, jobsStyle = "error", sanitizeLine(config.Redact(m.jobs.err.Error(), m.settings.Secrets()...)), &err
	} else if m.jobs.loaded && m.settings.DBConnection == "" && !m.demo && len(m.jobs.counts) == 0 && len(m.jobs.failed) == 0 && len(m.jobs.pending) == 0 {
		jobsStatus, jobsDetail = "n/a", "No database queue configured"
	} else if m.jobs.loaded {
		ready, delayed, reserved := 0, 0, 0
		for _, c := range m.jobs.counts {
			ready, delayed, reserved = ready+c.Ready, delayed+c.Delayed, reserved+c.Reserved
		}
		jobsStatus, jobsStyle = "● ok", &ok
		if len(m.jobs.failed) > 0 {
			jobsStatus, jobsStyle = fmt.Sprintf("▲ %d failed", len(m.jobs.failed)), &err
		}
		jobsDetail = fmt.Sprintf("ready %d · delayed %d · reserved %d · failed %d", ready, delayed, reserved, len(m.jobs.failed))
		if len(m.jobs.failed) == 100 {
			jobsDetail += " (first 100)"
		}
		if m.settings.QueueConnection != "" && m.settings.QueueConnection != "database" && !m.demo {
			jobsDetail += " · queue driver " + sanitizeLine(m.settings.QueueConnection) + " not inspected"
		}
	}

	servicesStatus, servicesDetail, servicesKey, servicesStyle := "—", "No connections checked yet (press 3)", "c", &muted
	if m.conns.loading && len(m.conns.results) == 0 {
		servicesStatus, servicesDetail = "…", "Checking…"
	} else if len(m.conns.results) > 0 {
		up, down := 0, 0
		for _, r := range m.conns.results {
			if r.OK {
				up++
			} else {
				down++
			}
		}
		servicesStatus, servicesStyle = "● all up", &ok
		if down > 0 {
			servicesStatus, servicesStyle = fmt.Sprintf("○ %d down", down), &err
		}
		servicesDetail = fmt.Sprintf("%d up · %d down · checked %s", up, down, m.conns.checked.Format("15:04:05"))
		for _, r := range m.conns.results {
			if r.Latency > 500*time.Millisecond {
				servicesDetail += " · slow: " + sanitizeLine(r.Name)
				break
			}
		}
	}
	return [][]Cell{
		{{Text: "Logs"}, {Text: logsStatus, Style: logsStyle}, {Text: logsDetail}, {Text: logsKey}},
		{{Text: "Jobs"}, {Text: jobsStatus, Style: jobsStyle}, {Text: jobsDetail}, {Text: jobsKey}},
		{{Text: "Services"}, {Text: servicesStatus, Style: servicesStyle}, {Text: servicesDetail}, {Text: servicesKey}},
	}
}

func (m Model) jumpOverview(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "l":
		m.active = TabLogs
	case "e":
		m.active, m.logFilter.Level, m.logFilter.Query = TabLogs, "ERROR", ""
		m.searching, m.detail, m.logNote = false, false, ""
		m.recompute()
	case "f", "p":
		m.active, m.jobs.detail = TabJobs, false
		if key == "f" {
			m.jobs.view = "failed"
		} else {
			m.jobs.view = "pending"
		}
		return m, m.tabLoadCmd(), true
	case "c":
		m.active = TabConnections
		return m, m.tabLoadCmd(), true
	case "r":
		if m.demo {
			m.notice = "demo: sample data, refresh disabled"
			return m, nil, true
		}
		if m.settings.Root == "" {
			return m, nil, true
		}
		return m, tea.Batch(loadLogsCmd(m.settings.Root), m.startJobsLoad(), m.startConnsLoad()), true
	default:
		return m, nil, false
	}
	return m, nil, true
}
