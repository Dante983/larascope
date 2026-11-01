package tui

import (
	"context"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/artisan"
	"larascope/internal/config"
	"larascope/internal/jobs"
	"strings"
	"time"
)

type confirmAction struct {
	desc  string
	need  string
	typed string
	run   func() tea.Cmd
}
type jobsState struct {
	loaded                      bool
	err                         error
	pending                     []jobs.PendingRow
	counts                      []jobs.QueueCount
	failed                      []jobs.FailedRow
	view                        string
	pendingCursor, failedCursor int
	confirm                     *confirmAction
	status                      string
	detail                      bool
}
type jobsLoadedMsg struct {
	pending []jobs.PendingRow
	counts  []jobs.QueueCount
	failed  []jobs.FailedRow
	err     error
}
type artisanDoneMsg struct {
	out string
	err error
}

func loadJobsCmd(s config.Settings) tea.Cmd {
	return func() tea.Msg {
		if s.DBConnection != "mysql" && s.DBConnection != "mariadb" {
			if s.DBConnection == "" {
				return jobsLoadedMsg{}
			}
			return jobsLoadedMsg{err: fmt.Errorf("Jobs: unsupported DB_CONNECTION %s (mysql only)", s.DBConnection)}
		}
		ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		src, e := jobs.OpenMySQL(s.DSN())
		if e != nil {
			return jobsLoadedMsg{err: errors.New(config.Redact(e.Error(), s.DBPassword, s.DSN()))}
		}
		defer src.Close()
		p, n, e := src.Pending(ctx, 100)
		if e != nil {
			return jobsLoadedMsg{err: errors.New(config.Redact(e.Error(), s.DBPassword, s.DSN()))}
		}
		f, e := src.Failed(ctx, 100)
		if errors.Is(e, jobs.ErrNoTable) {
			e = nil
		}
		if e != nil {
			e = errors.New(config.Redact(e.Error(), s.DBPassword, s.DSN()))
		}
		return jobsLoadedMsg{p, n, f, e}
	}
}
func (m Model) jobsBody() string {
	if m.settings.Root == "" {
		return "No jobs detected yet"
	}
	if m.jobs.err != nil {
		return config.Redact(m.jobs.err.Error(), m.settings.DBPassword, m.settings.DSN())
	}
	lines := []string{"Jobs [" + m.jobs.view + "] (p to switch)"}
	for _, c := range m.jobs.counts {
		lines = append(lines, fmt.Sprintf("%s ready=%d delayed=%d reserved=%d", c.Queue, c.Ready, c.Delayed, c.Reserved))
	}
	if m.jobs.view == "pending" {
		if len(m.jobs.pending) == 0 {
			lines = append(lines, "No pending jobs")
		} else {
			for i, j := range m.jobs.pending {
				line := fmt.Sprintf("%d %s %s %s %d", j.ID, j.State, j.Queue, j.Class, j.Attempts)
				if i == m.jobs.pendingCursor {
					line = selectedStyle.Render(line)
				}
				lines = append(lines, line)
			}
			if m.jobs.detail {
				j := m.jobs.pending[m.jobs.pendingCursor]
				lines = append(lines, "", fmt.Sprintf("id: %d\nqueue: %s\nstate: %s\nattempts: %d\nclass: %s\navailable: %s\ncreated: %s\nreserved: %v", j.ID, j.Queue, j.State, j.Attempts, j.Class, j.AvailableAt, j.CreatedAt, j.ReservedAt))
			}
		}
	} else if len(m.jobs.failed) == 0 {
		lines = append(lines, "No failed jobs")
	} else {
		for i, j := range m.jobs.failed {
			line := fmt.Sprintf("%d %s %s %s", j.ID, j.Queue, j.Class, j.FailedAt)
			if i == m.jobs.failedCursor {
				line = selectedStyle.Render(line)
			}
			lines = append(lines, line)
		}
		if m.jobs.detail {
			j := m.jobs.failed[m.jobs.failedCursor]
			lines = append(lines, "", fmt.Sprintf("id: %d\nuuid: %s\nconnection: %s\nqueue: %s\nclass: %s\nfailed_at: %s\nexception:\n%s", j.ID, j.UUID, j.Connection, j.Queue, j.Class, j.FailedAt, firstLines(j.Exception, 20)))
		}
	}
	if m.jobs.status != "" {
		lines = append(lines, "", m.jobs.status)
	}
	return strings.Join(lines, "\n")
}
func (m Model) handleJobsKey(k string) (Model, tea.Cmd) {
	if k == "p" {
		if m.jobs.view == "pending" {
			m.jobs.view = "failed"
		} else {
			m.jobs.view = "pending"
		}
		m.jobs.detail = false
		return m, nil
	}
	length, cursor := len(m.jobs.pending), &m.jobs.pendingCursor
	if m.jobs.view == "failed" {
		length, cursor = len(m.jobs.failed), &m.jobs.failedCursor
	}
	switch k {
	case "up", "k":
		if length > 0 {
			*cursor = max(0, *cursor-1)
		}
		return m, nil
	case "down", "j":
		if length > 0 {
			*cursor = min(length-1, *cursor+1)
		}
		return m, nil
	case "enter":
		if length > 0 {
			m.jobs.detail = !m.jobs.detail
		}
		return m, nil
	case "esc":
		m.jobs.detail = false
		return m, nil
	}
	if k == "R" || k == "d" {
		if m.jobs.view != "failed" || len(m.jobs.failed) == 0 || m.settings.Root == "" {
			return m, nil
		}
		f := m.jobs.failed[m.jobs.failedCursor]
		id := f.UUID
		if id == "" {
			id = fmt.Sprint(f.ID)
		}
		desc := "retry " + f.Class + " " + id
		if k == "d" {
			desc = "forget " + f.Class + " " + id
		}
		root := m.settings.Root
		m.jobs.confirm = &confirmAction{desc: desc, run: func() tea.Cmd { return artisanCmd(root, k, id) }}
		return m, nil
	}
	if (k == "A" || k == "F") && (m.jobs.view != "failed" || len(m.jobs.failed) == 0 || m.settings.Root == "") {
		return m, nil
	}
	root := m.settings.Root
	if k == "A" {
		m.jobs.confirm = &confirmAction{desc: fmt.Sprintf("retry all %d failed jobs", len(m.jobs.failed)), need: "RETRY ALL", run: func() tea.Cmd { return artisanCmd(root, "A", "") }}
	}
	if k == "F" {
		m.jobs.confirm = &confirmAction{desc: fmt.Sprintf("permanently deletes %d failed jobs", len(m.jobs.failed)), need: "FLUSH", run: func() tea.Cmd { return artisanCmd(root, "F", "") }}
	}
	return m, nil
}
func clampCursor(cursor, length int) int { return min(max(0, cursor), max(0, length-1)) }
func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[:min(n, len(lines))], "\n")
}
func artisanCmd(root, k, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, c := context.WithTimeout(context.Background(), 60*time.Second)
		defer c()
		r := artisan.Runner{Root: root}
		var o string
		var e error
		switch k {
		case "R":
			o, e = r.Retry(ctx, id)
		case "d":
			o, e = r.Forget(ctx, id)
		case "A":
			o, e = r.RetryAll(ctx)
		case "F":
			o, e = r.Flush(ctx)
		}
		return artisanDoneMsg{o, e}
	}
}
