package tui

import (
	"context"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		body := wrap(sanitizeLine(config.Redact(m.jobs.err.Error(), m.settings.DBPassword, m.settings.DSN())), m.clipWidth())
		driver := strings.ToLower(strings.TrimSpace(m.settings.DBConnection))
		if driver != "" && driver != "mysql" && driver != "mariadb" {
			body += "\nJobs supports MySQL/MariaDB only"
		}
		return body
	}
	if m.jobs.detail && m.detailCompact() {
		lines := []string{"Jobs [" + m.jobs.view + "] detail (enter/esc: back)"}
		if m.jobs.view == "pending" && len(m.jobs.pending) > 0 {
			lines = append(lines, m.addJobDetail(nil, m.pendingDetail())...)
		} else if m.jobs.view == "failed" && len(m.jobs.failed) > 0 {
			lines = append(lines, m.addJobDetail(nil, m.failedDetail())...)
		}
		if m.jobs.status != "" {
			lines = append(lines, "", sanitizeLine(m.jobs.status))
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{"Jobs [" + m.jobs.view + "] (p to switch)"}
	totalReady, totalDelayed, totalReserved := 0, 0, 0
	for i, c := range m.jobs.counts {
		totalReady += c.Ready
		totalDelayed += c.Delayed
		totalReserved += c.Reserved
		if i < 3 {
			lines = append(lines, fmt.Sprintf("%s %s %s %s", sanitizeLine(c.Queue), badge(fmt.Sprintf("ready=%d", c.Ready), colOK), badge(fmt.Sprintf("delayed=%d", c.Delayed), colWarn), badge(fmt.Sprintf("reserved=%d", c.Reserved), colInfo)))
		}
	}
	if len(m.jobs.counts) > 3 {
		lines = append(lines, fmt.Sprintf("… +%d queues", len(m.jobs.counts)-3))
	}
	lines = append(lines, fmt.Sprintf("totals ready=%d delayed=%d reserved=%d  failed: %d%s", totalReady, totalDelayed, totalReserved, len(m.jobs.failed), func() string {
		if len(m.jobs.failed) == 100 {
			return " (first 100)"
		}
		return ""
	}()))
	if m.jobs.view == "pending" {
		if len(m.jobs.pending) == 0 {
			lines = append(lines, "No pending jobs")
		} else {
			w := m.clipWidth()
			if m.twoPane() {
				w = (m.contentWidth() * 55) / 100
			}
			list := m.pendingTable(w)
			lines = append(lines, list...)
			if m.jobs.detail {
				lines = m.addJobDetail(lines, m.pendingDetail())
			}
		}
	} else if len(m.jobs.failed) == 0 {
		lines = append(lines, "No failed jobs")
	} else {
		w := m.clipWidth()
		if m.twoPane() {
			w = (m.contentWidth() * 55) / 100
		}
		list := m.failedTable(w)
		lines = append(lines, list...)
		if m.jobs.detail {
			lines = m.addJobDetail(lines, m.failedDetail())
		}
	}
	if m.jobs.status != "" {
		lines = append(lines, "", sanitizeLine(m.jobs.status))
	}
	return strings.Join(lines, "\n")
}

// detailCompact is used when a single-column detail cannot coexist with the list.
func (m Model) detailCompact() bool {
	return m.jobs.detail && !m.twoPane() && m.height > 0 && m.bodyHeight() < 14
}
func (m Model) jobRows(overhead int) int {
	rows := m.bodyHeight() - overhead
	if m.jobs.detail {
		rows -= max(3, m.bodyHeight()/2)
	}
	return max(1, rows)
}
func (m Model) pendingTable(w int) []string {
	all := make([]int, len(m.jobs.pending))
	for i := range all {
		all[i] = i
	}
	overhead := 1 + min(3, len(m.jobs.counts)) + 1 + 1 + 1
	if len(m.jobs.counts) > 3 {
		overhead++
	}
	if m.jobs.status != "" {
		overhead += 2
	}
	s, e := window(all, m.jobs.pendingCursor, m.jobRows(overhead))
	out := []string{lipgloss.NewStyle().Foreground(colMuted).Bold(true).Render("ID  STATE     QUEUE  CLASS" + func() string {
		if m.width >= 80 {
			return "  TRIES"
		}
		return ""
	}())}
	for _, i := range all[s:e] {
		j := m.jobs.pending[i]
		stateText := sanitizeLine(j.State)
		state := levelStyle(map[string]string{"ready": "INFO", "delayed": "WARNING", "reserved": "DEBUG"}[stateText]).Render(padRight(stateText, 9))
		class := sanitizeLine(j.Class)
		if w != 0 {
			class = truncate(class, max(8, w-32))
		}
		line := fmt.Sprintf("%d  %s  %-8s  %s", j.ID, state, sanitizeLine(j.Queue), class)
		if m.width >= 80 {
			line += fmt.Sprintf("  %d", j.Attempts)
		}
		if i == m.jobs.pendingCursor {
			line = selectedStyle.Render(line)
		}
		out = append(out, truncate(line, w))
	}
	return append(out, fmt.Sprintf("rows %d–%d of %d", s+1, e, len(all)))
}
func (m Model) failedTable(w int) []string {
	all := make([]int, len(m.jobs.failed))
	for i := range all {
		all[i] = i
	}
	overhead := 1 + min(3, len(m.jobs.counts)) + 1 + 1 + 1
	if len(m.jobs.counts) > 3 {
		overhead++
	}
	if m.jobs.status != "" {
		overhead += 2
	}
	s, e := window(all, m.jobs.failedCursor, m.jobRows(overhead))
	out := []string{lipgloss.NewStyle().Foreground(colMuted).Bold(true).Render("ID  QUEUE  CLASS  FAILED AT")}
	for _, i := range all[s:e] {
		j := m.jobs.failed[i]
		class := sanitizeLine(j.Class)
		if w != 0 {
			class = truncate(class, max(8, w-42))
		}
		line := fmt.Sprintf("%d  %-8s  %s  %s", j.ID, sanitizeLine(j.Queue), class, j.FailedAt.Format("2006-01-02 15:04"))
		if i == m.jobs.failedCursor {
			line = selectedStyle.Render(line)
		}
		out = append(out, truncate(line, w))
	}
	return append(out, fmt.Sprintf("rows %d–%d of %d", s+1, e, len(all)))
}
func (m Model) pendingDetail() string {
	j := m.jobs.pending[m.jobs.pendingCursor]
	return fmt.Sprintf("id: %d\nqueue: %s\nstate: %s\nattempts: %d\nclass: %s\navailable: %s\ncreated: %s\nreserved: %v", j.ID, sanitizeLine(j.Queue), sanitizeLine(j.State), j.Attempts, sanitizeLine(j.Class), j.AvailableAt, j.CreatedAt, j.ReservedAt)
}
func (m Model) failedDetail() string {
	j := m.jobs.failed[m.jobs.failedCursor]
	return fmt.Sprintf("id: %d\nqueue: %s\nuuid: %s\nconnection: %s\nclass: %s\nfailed_at: %s\nexception:\n%s", j.ID, sanitizeLine(j.Queue), sanitizeLine(j.UUID), sanitizeLine(j.Connection), sanitizeLine(j.Class), j.FailedAt, firstLines(sanitize(j.Exception), 20))
}
func (m Model) addJobDetail(lines []string, detail string) []string {
	unlimited := m.width == 0
	listW, detailW, inner := 0, 0, 0
	if !unlimited {
		w := m.contentWidth()
		listW = w
		detailW = w
		if m.twoPane() {
			listW = (w * 55) / 100
			detailW = w - listW - 2
		}
		inner = max(1, detailW-2)
	}
	detailLines := []string{}
	for _, line := range strings.Split(sanitize(detail), "\n") {
		if unlimited {
			detailLines = append(detailLines, line)
		} else {
			detailLines = append(detailLines, strings.Split(wrap(line, inner), "\n")...)
		}
	}
	cap := max(3, m.bodyHeight()/2)
	if m.detailCompact() {
		cap = max(3, m.bodyHeight()-1-2-1)
		if m.jobs.status != "" {
			cap = max(1, cap-2)
		}
	}
	if m.height == 0 {
		cap = len(detailLines)
	}
	if len(detailLines) > cap {
		if cap == 1 {
			detailLines = []string{fmt.Sprintf("… %d more lines", len(detailLines))}
		} else {
			detailLines = append(detailLines[:cap-1], fmt.Sprintf("… %d more lines", len(detailLines)-(cap-1)))
		}
	}
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colMuted)
	if !unlimited {
		st = st.Width(inner)
	}
	box := st.Render(strings.Join(detailLines, "\n"))
	if m.twoPane() {
		left := make([]string, len(lines))
		for i, line := range lines {
			left[i] = padRight(truncate(line, listW), listW)
		}
		return []string{clipLines(lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), "  ", box), m.contentWidth())}
	}
	if m.detailCompact() {
		return []string{box}
	}
	return append(lines, "", box)
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
