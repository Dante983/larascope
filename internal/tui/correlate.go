package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"larascope/internal/correlate"
	"larascope/internal/jobs"
	"larascope/internal/logs"
	"strings"
)

type correlationState struct {
	active     bool
	job        jobs.FailedRow
	matches    []correlate.Match
	byLog      map[int]correlate.Match
	cursor     int
	detailLog  int
	prevCursor int
	prevDetail bool
}

func (m Model) enterCorrelation(f jobs.FailedRow) Model {
	if !m.logLoaded || m.logErr != nil || len(m.entries) == 0 {
		m.jobs.status = "no logs loaded"
		return m
	}
	ms := correlate.LogsForJob(m.entries, f, 50)
	if len(ms) == 0 {
		m.jobs.status = fmt.Sprintf("no related log entries found for job %d", f.ID)
		return m
	}
	by := map[int]correlate.Match{}
	for _, x := range ms {
		by[x.LogIndex] = x
	}
	m.active = TabLogs
	m.corr = correlationState{active: true, job: f, matches: ms, byLog: by, detailLog: -1, prevCursor: m.cursor, prevDetail: m.detail}
	m.detail = false
	return m
}

// handleCorrKey owns keyboard input while the related-log list is open. Keep
// the fall-through allow-list below explicit: new global keys must opt in so
// they cannot accidentally operate on the normal Logs cursor.
func (m Model) handleCorrKey(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if len(m.corr.matches) == 0 {
		m.corr = correlationState{}
		return m, nil, true
	}

	// These are deliberately the only global keys that may escape corr mode.
	// Check key types first: a KeyRunes message spelling "enter" is text, not
	// an Enter key.
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, nil, false
	case tea.KeyTab, tea.KeyShiftTab:
		return m, nil, false
	}
	k := msg.String()
	if msg.Type == tea.KeyRunes {
		switch k {
		case "q", "1", "2", "3", "?", "r":
			return m, nil, false
		}
	}

	if m.detail {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyEnter:
			m.detail = false
			m.corr.detailLog = -1
			return m, nil, true
		}
		if msg.Type == tea.KeyRunes && k == "J" {
			return m.jumpToJobFromLog(), nil, true
		}
		// There is no independent Logs-detail scroll state.
		return m, nil, true
	}

	switch msg.Type {
	case tea.KeyUp:
		m.corr.cursor = clampCursor(m.corr.cursor-1, len(m.corr.matches))
	case tea.KeyDown:
		m.corr.cursor = clampCursor(m.corr.cursor+1, len(m.corr.matches))
	case tea.KeyPgUp:
		m.corr.cursor = clampCursor(m.corr.cursor-max(1, m.listHeight()), len(m.corr.matches))
	case tea.KeyPgDown:
		m.corr.cursor = clampCursor(m.corr.cursor+max(1, m.listHeight()), len(m.corr.matches))
	case tea.KeyEnter:
		if m.corr.cursor >= 0 && m.corr.cursor < len(m.corr.matches) && m.corr.matches[m.corr.cursor].LogIndex >= 0 && m.corr.matches[m.corr.cursor].LogIndex < len(m.entries) {
			m.corr.detailLog = m.corr.matches[m.corr.cursor].LogIndex
			m.detail = true
		}
	case tea.KeyEsc:
		m.cursor = clampCursor(m.corr.prevCursor, len(m.entries))
		m.detail = false
		m.corr = correlationState{}
	default:
		if msg.Type == tea.KeyRunes {
			switch k {
			case "k":
				m.corr.cursor = clampCursor(m.corr.cursor-1, len(m.corr.matches))
			case "j":
				m.corr.cursor = clampCursor(m.corr.cursor+1, len(m.corr.matches))
			case "g":
				m.corr.cursor = 0
			case "G":
				m.corr.cursor = clampCursor(len(m.corr.matches)-1, len(m.corr.matches))
			case "J":
				m = m.jumpToJobFromCorrelation()
			}
		}
	}
	return m, nil, true
}

// shownEntryIndex is the entry currently displayed in a Logs detail pane.
func (m Model) shownEntryIndex() int {
	if m.corr.active && m.detail {
		return m.corr.detailLog
	}
	return m.cursor
}

func (m Model) correlationHints() string {
	if m.active == TabJobs {
		if m.jobs.view == "failed" {
			return "L: related logs"
		}
		return ""
	}
	if m.active != TabLogs || !m.corr.active {
		return ""
	}
	if m.detail {
		return "J: job · esc: list"
	}
	return "enter: detail · J: job · esc: back"
}
func confStyle(c correlate.Confidence) lipgloss.Style {
	switch c {
	case correlate.ConfHigh:
		return levelStyle("INFO")
	case correlate.ConfMedium:
		return levelStyle("WARNING")
	default:
		return levelStyle("DEBUG")
	}
}
func (m Model) correlationBody() string {
	w := m.clipWidth()
	rows := make([][]Cell, len(m.corr.matches))
	for i, x := range m.corr.matches {
		if x.LogIndex < 0 || x.LogIndex >= len(m.entries) {
			continue
		}
		e := m.entries[x.LogIndex]
		st := confStyle(x.Confidence)
		rows[i] = []Cell{{Text: x.Confidence.String(), Style: &st}, {Text: e.Time.Format("15:04:05")}, {Text: sanitizeLine(e.Level), Style: func() *lipgloss.Style { s := levelStyle(e.Level); return &s }()}, {Text: sanitizeLine(strings.SplitN(e.Message, "\n", 2)[0])}, {Text: sanitizeLine(x.Reason)}}
	}
	cols := []Column{{"CONF", 4, 6, false, AlignLeft, 0}, {"TIME", 8, 8, false, AlignLeft, 3}, {"LEVEL", 5, 9, false, AlignLeft, 0}, {"MESSAGE", 10, 0, true, AlignLeft, 0}, {"WHY", 10, 0, true, AlignLeft, 2}}
	lines, _, _ := (Table{Cols: cols, Rows: rows, Cursor: m.corr.cursor, Width: w}).Render(m.listHeight())
	return strings.Join(append([]string{metaLine(fmt.Sprintf("Related to failed job %d %s · %d entries", m.corr.job.ID, sanitizeLine(m.corr.job.Class), len(rows)), "esc: back", w)}, lines...), "\n")
}
func (m Model) jumpToJobFromLog() Model {
	if !m.jobs.loaded || len(m.jobs.failed) == 0 {
		m.logNote = "open Jobs tab first to load failed jobs"
		return m
	}
	idx := m.shownEntryIndex()
	if idx < 0 || idx >= len(m.entries) {
		return m
	}
	ms := correlate.JobsForLog(m.entries[idx], m.jobs.failed, 5)
	if len(ms) == 0 {
		m.logNote = "no matching failed job"
		return m
	}
	x := ms[0]
	m.active = TabJobs
	m.jobs.view = "failed"
	m.jobs.failedCursor = x.JobIndex
	m.jobs.detail = true
	m.jobs.status = sanitizeLine("matched from log: " + x.Confidence.String() + " · " + x.Reason)
	m = m.clearCorrelationAfterJump()
	return m
}

func (m Model) clearCorrelationAfterJump() Model {
	if m.corr.active {
		m.cursor = clampCursor(m.corr.prevCursor, len(m.entries))
		m.detail = false
		m.corr = correlationState{}
	}
	return m
}

func (m Model) jumpToJobFromCorrelation() Model {
	if !m.jobs.loaded || len(m.jobs.failed) == 0 {
		m.logNote = "open Jobs tab first to load failed jobs"
		return m
	}
	idx := -1
	status := ""
	if m.corr.cursor >= 0 && m.corr.cursor < len(m.corr.matches) {
		logIndex := m.corr.matches[m.corr.cursor].LogIndex
		if logIndex >= 0 && logIndex < len(m.entries) {
			if ms := correlate.JobsForLog(m.entries[logIndex], m.jobs.failed, 5); len(ms) > 0 {
				idx = ms[0].JobIndex
				status = sanitizeLine("correlated from log list: " + ms[0].Confidence.String() + " · " + ms[0].Reason)
			}
		}
	}
	if idx < 0 {
		for i, j := range m.jobs.failed {
			if j.ID == m.corr.job.ID || (m.corr.job.ID == 0 && m.corr.job.UUID != "" && j.UUID == m.corr.job.UUID) {
				idx = i
				break
			}
		}
		if idx >= 0 {
			status = sanitizeLine(fmt.Sprintf("back to failed job %d", m.jobs.failed[idx].ID))
		} else {
			status = "failed job no longer in list"
			idx = clampCursor(m.jobs.failedCursor, len(m.jobs.failed))
		}
	}
	m.active = TabJobs
	m.jobs.view = "failed"
	m.jobs.failedCursor = clampCursor(idx, len(m.jobs.failed))
	m.jobs.detail = false
	m.jobs.status = status
	m = m.clearCorrelationAfterJump()
	return m
}
func (m Model) relatedLogLines(j jobs.FailedRow) []string {
	ms := correlate.LogsForJob(m.entries, j, 3)
	out := make([]string, 0, len(ms))
	for _, x := range ms {
		e := m.entries[x.LogIndex]
		out = append(out, fmt.Sprintf("  [%s] %s %s · %s", x.Confidence, e.Time.Format("15:04:05"), sanitizeLine(x.Reason), truncate(sanitizeLine(strings.SplitN(e.Message, "\n", 2)[0]), 50)))
	}
	return out
}
func (m Model) relatedJobLine(e logs.Entry) string {
	ms := correlate.JobsForLog(e, m.jobs.failed, 1)
	if len(ms) == 0 {
		return ""
	}
	x := ms[0]
	j := m.jobs.failed[x.JobIndex]
	return sanitizeLine(fmt.Sprintf("related failed job: #%d %s [%s] %s", j.ID, j.Class, x.Confidence, x.Reason))
}
