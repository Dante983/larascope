package tui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"larascope/internal/artisan"
	"larascope/internal/conns"
	"larascope/internal/jobs"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"larascope/internal/config"
	"larascope/internal/logs"
)

type Tab int

const (
	TabLogs Tab = iota
	TabJobs
	TabConnections
)

var tabNames = []string{"Logs", "Jobs", "Connections"}

type Model struct {
	active          Tab
	width, height   int
	showHelp        bool
	detail          bool
	settings        config.Settings
	cfgErr          error
	entries         []logs.Entry
	cursor          int
	logErr          error
	logLoaded       bool
	logFilter       logs.Filter
	searching       bool
	searchInput     string
	grouped, follow bool
	visible         []int
	jobs            jobsState
	conns           connsState
}

const maxLogLines = 20

type logsLoadedMsg struct {
	entries []logs.Entry
	err     error
}

func loadLogsCmd(root string) tea.Cmd {
	return func() tea.Msg {
		entries, err := logs.LoadTail(logs.LatestLogFile(root, ""), 5<<20)
		return logsLoadedMsg{entries, err}
	}
}

var (
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle    = lipgloss.NewStyle().Reverse(true)
)

func New() Model {
	return Model{active: TabLogs, jobs: jobsState{view: "pending"}}
}

func (m Model) WithConfig(s config.Settings, err error) Model {
	m.settings = s
	m.cfgErr = err
	return m
}

func (m Model) WithLogs(entries []logs.Entry, err error) Model {
	m.entries, m.logErr, m.logLoaded = entries, err, true
	m.recompute()
	m.cursor = max(0, len(entries)-1)
	m.detail = false
	return m
}

func (m Model) statusLine() string {
	if errors.Is(m.cfgErr, config.ErrNotLaravel) {
		return "Not a Laravel project (no artisan found)"
	}
	if m.cfgErr != nil {
		return "Config error: " + m.cfgErr.Error()
	}

	parts := make([]string, 0, 4)
	if m.settings.Root != "" {
		parts = append(parts, "root: "+m.settings.Root)
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

	return strings.Join(parts, "  ")
}

func (m Model) Init() tea.Cmd {
	if m.follow {
		return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return followTickMsg{} })
	}
	return nil
}

type followTickMsg struct{}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case logsLoadedMsg:
		return m.reloadLogs(msg), nil
	case followTickMsg:
		if m.follow && m.active == TabLogs && m.settings.Root != "" {
			return m, tea.Batch(loadLogsCmd(m.settings.Root), tea.Tick(2*time.Second, func(time.Time) tea.Msg { return followTickMsg{} }))
		}
	case jobsLoadedMsg:
		m.jobs.loaded = true
		m.jobs.pending, m.jobs.counts, m.jobs.failed, m.jobs.err = msg.pending, msg.counts, msg.failed, msg.err
		m.jobs.pendingCursor = clampCursor(m.jobs.pendingCursor, len(m.jobs.pending))
		m.jobs.failedCursor = clampCursor(m.jobs.failedCursor, len(m.jobs.failed))
	case connsLoadedMsg:
		m.conns.loading = false
		m.conns.results = msg.results
		m.conns.checked = time.Now()
	case connsTickMsg:
		if m.active != TabConnections {
			m.conns.connsTicking = false
			return m, nil
		}
		return m, tea.Batch(m.startConnsLoad(), connsTick())
	case artisanDoneMsg:
		m.jobs.status = config.Redact(msg.out+errText(msg.err), m.settings.DBPassword, m.settings.DSN())
		return m, loadJobsCmd(m.settings)
	case tea.KeyMsg:
		if m.jobs.confirm != nil {
			a := m.jobs.confirm
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			if msg.String() == "esc" || msg.String() == "q" {
				m.jobs.confirm = nil
				return m, nil
			}
			if a.need == "" {
				m.jobs.confirm = nil
				if msg.String() == "y" {
					return m, a.run()
				}
				return m, nil
			}
			switch msg.String() {
			case "backspace":
				r := []rune(a.typed)
				if len(r) > 0 {
					a.typed = string(r[:len(r)-1])
				}
			case "enter":
				if a.typed == a.need {
					m.jobs.confirm = nil
					return m, a.run()
				}
				m.jobs.status = "type " + a.need + " to confirm"
			default:
				if (msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace) && !(a.typed == "" && msg.String() == "y") {
					if msg.Type == tea.KeySpace && len(msg.Runes) == 0 {
						a.typed += " "
					} else {
						a.typed += string(msg.Runes)
					}
				}
			}
			return m, nil
		}
		if m.active == TabLogs && m.searching {
			return m.handleSearch(msg)
		}
		if m.active == TabJobs && !m.showHelp {
			switch msg.String() {
			case "p", "R", "A", "d", "F", "up", "k", "down", "j", "enter", "esc":
				m, cmd := m.handleJobsKey(msg.String())
				return m, cmd
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.active = Tab((int(m.active) + 1) % len(tabNames))
			return m, m.tabLoadCmd()
		case "shift+tab":
			m.active = Tab((int(m.active) - 1 + len(tabNames)) % len(tabNames))
			return m, m.tabLoadCmd()
		case "1":
			m.active = TabLogs
		case "2":
			m.active = TabJobs
			if !m.jobs.loaded && m.settings.Root != "" {
				return m, loadJobsCmd(m.settings)
			}
		case "3":
			m.active = TabConnections
			return m, m.tabLoadCmd()
		case "r":
			if m.active == TabLogs && !m.showHelp && m.settings.Root != "" {
				return m, loadLogsCmd(m.settings.Root)
			}
			if m.active == TabJobs && m.settings.Root != "" {
				return m, loadJobsCmd(m.settings)
			}
			if m.active == TabConnections {
				return m, m.startConnsLoad()
			}
		case "?":
			m.showHelp = !m.showHelp
		case "l":
			if m.active == TabLogs && !m.showHelp {
				m.cycleLevel()
			}
		case "/":
			if m.active == TabLogs && !m.showHelp {
				m.searching = true
				m.searchInput = ""
			}
		case "s":
			if m.active == TabLogs && !m.showHelp {
				m.grouped = !m.grouped
			}
		case "f":
			if m.active == TabLogs && !m.showHelp {
				m.follow = !m.follow
				if m.follow {
					return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return followTickMsg{} })
				}
			}
		case "enter":
			if m.active == TabLogs && !m.showHelp && len(m.entries) > 0 {
				m.detail = !m.detail
			}
		case "esc":
			m.detail = false
		case "up", "k":
			if m.active == TabLogs && !m.showHelp {
				m = m.moveCursor(-1)
			}
		case "down", "j":
			if m.active == TabLogs && !m.showHelp {
				m = m.moveCursor(1)
			}
		case "g":
			if m.active == TabLogs && !m.showHelp && !m.detail {
				m = m.jumpCursor(0)
			}
		case "G":
			if m.active == TabLogs && !m.showHelp && !m.detail {
				m = m.jumpCursor(len(m.entries) - 1)
			}
		case "pgup":
			if m.active == TabLogs && !m.showHelp && !m.detail {
				m = m.moveCursor(-maxLogLines)
			}
		case "pgdown":
			if m.active == TabLogs && !m.showHelp && !m.detail {
				m = m.moveCursor(maxLogLines)
			}
		}
	}

	return m, nil
}

func (m Model) reloadLogs(msg logsLoadedMsg) Model {
	if msg.err != nil {
		m.logErr = msg.err
		m.logLoaded = true
		return m
	}

	oldEntriesEmpty := len(m.entries) == 0
	wasLast := m.cursor >= len(m.entries)-1
	m.entries = msg.entries
	m.recompute()
	m.logErr = nil
	m.logLoaded = true

	last := max(0, len(m.entries)-1)
	if wasLast || oldEntriesEmpty {
		m.cursor = last
	} else {
		m.cursor = min(m.cursor, last)
	}
	if len(m.entries) == 0 {
		m.detail = false
	}
	return m
}

func (m Model) moveCursor(delta int) Model {
	list := m.visible
	if m.logFilter.Level == "" && m.logFilter.Query == "" {
		list = make([]int, len(m.entries))
		for i := range m.entries {
			list[i] = i
		}
	}
	if len(list) == 0 {
		return m
	}
	pos := 0
	for i, x := range list {
		if x == m.cursor {
			pos = i
			break
		}
	}
	pos = min(max(0, pos+delta), len(list)-1)
	m.cursor = list[pos]
	return m
}
func (m Model) jumpCursor(to int) Model {
	list := m.visible
	if m.logFilter.Level == "" && m.logFilter.Query == "" {
		list = make([]int, len(m.entries))
		for i := range m.entries {
			list[i] = i
		}
	}
	if len(list) == 0 {
		return m
	}
	m.cursor = list[min(max(0, to), len(list)-1)]
	return m
}
func (m *Model) tabLoadCmd() tea.Cmd {
	if m.active == TabJobs && !m.jobs.loaded && m.settings.Root != "" {
		return loadJobsCmd(m.settings)
	}
	if m.active == TabConnections {
		cmd := m.startConnsLoad()
		if !m.conns.connsTicking {
			m.conns.connsTicking = true
			return tea.Batch(cmd, connsTick())
		}
		return cmd
	}
	return nil
}

func (m Model) View() string {
	tabs := make([]string, len(tabNames))
	for i, name := range tabNames {
		if Tab(i) == m.active {
			tabs[i] = activeTabStyle.Render(name)
		} else {
			tabs[i] = inactiveTabStyle.Render(name)
		}
	}

	body := m.body()
	if m.width > 0 && m.centered() {
		body = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, body)
	}

	footer := footerStyle.Render("tab: switch  ?: help  q: quit")
	if m.jobs.confirm != nil {
		if m.jobs.confirm.need == "" {
			footer = footerStyle.Render("Confirm: " + m.jobs.confirm.desc + "  y=yes, any other key=cancel")
		} else {
			footer = footerStyle.Render("Confirm: " + m.jobs.confirm.desc + " — type " + m.jobs.confirm.need + " then enter, esc cancel")
		}
	}
	if status := m.statusLine(); status != "" {
		footer = footerStyle.Render(status) + "\n" + footer
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s", lipgloss.JoinHorizontal(lipgloss.Top, tabs...), body, footer)
}

func (m Model) centered() bool {
	return !(m.active == TabLogs && m.logLoaded && m.logErr == nil && len(m.entries) > 0)
}

func (m Model) body() string {
	if m.showHelp {
		return "tab: next tab\nshift+tab: previous tab\n1/2/3: select tab\nup/k, down/j: move in logs (prev/next entry in detail)\ng/G: first/last entry\npgup/pgdown: page up/down\nenter: toggle entry detail\nesc: back to list\nr: reload log file\nl: level filter  /: search  s: group  f: follow\np: pending/failed  j/k: move  enter: detail  R/d: retry/forget selected  A: retry all (type to confirm)  F: flush (type to confirm)\n?: toggle help\nq: quit"
	}

	switch m.active {
	case TabJobs:
		return m.jobsBody()
	case TabConnections:
		return m.connsBody()
	default:
		return m.logsBody()
	}
}

func (m Model) logsBody() string {
	if !m.logLoaded {
		return "No log file detected yet"
	}
	if errors.Is(m.logErr, fs.ErrNotExist) {
		return "No log file found (storage/logs/laravel.log)"
	}
	if m.logErr != nil {
		return "Log error: " + m.logErr.Error()
	}
	if len(m.entries) == 0 {
		return "Log file has no entries"
	}
	if m.detail {
		return m.detailBody()
	}
	if m.grouped {
		return m.groupedLogsBody()
	}
	if len(m.visible) != len(m.entries) || m.logFilter.Level != "" || m.logFilter.Query != "" {
		return m.filteredLogsBody()
	}

	start := max(0, len(m.entries)-maxLogLines)
	if m.cursor < start {
		start = m.cursor
	}
	end := min(len(m.entries), start+maxLogLines)
	lines := make([]string, 0, end-start)
	for i, entry := range m.entries[start:end] {
		line := entry.Time.Format("2006-01-02 15:04:05") + " " + entry.Level + " " + entry.Message
		if m.width > 0 {
			runes := []rune(line)
			if len(runes) > m.width {
				line = string(runes[:m.width])
			}
		}
		if start+i == m.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}

	return fmt.Sprintf("%d entries  (%d/%d)\n\n%s", len(m.entries), m.cursor+1, len(m.entries), strings.Join(lines, "\n"))
}

func (m Model) detailBody() string {
	entry := m.entries[m.cursor]
	body := entry.Time.Format("2006-01-02 15:04:05") + " " + entry.Level + " " + entry.Env + "\n\n" + entry.Message
	if len(entry.Trace) > 0 {
		body += "\n\n" + strings.Join(entry.Trace, "\n")
	}
	return body
}

func (m *Model) recompute() {
	m.visible = m.logFilter.Apply(m.entries)
	if m.logFilter.Level == "" && m.logFilter.Query == "" {
		m.visible = make([]int, len(m.entries))
		for i := range m.entries {
			m.visible[i] = i
		}
	}
	if len(m.visible) > 0 {
		found := false
		for _, i := range m.visible {
			if i == m.cursor {
				found = true
			}
		}
		if !found {
			m.cursor = m.visible[len(m.visible)-1]
		}
	}
}
func (m *Model) cycleLevel() {
	switch m.logFilter.Level {
	case "":
		m.logFilter.Level = "ERROR"
	case "ERROR":
		m.logFilter.Level = "WARNING"
	case "WARNING":
		m.logFilter.Level = "INFO"
	case "INFO":
		m.logFilter.Level = "DEBUG"
	default:
		m.logFilter.Level = ""
	}
	m.recompute()
}
func (m Model) handleSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "enter":
		m.searching = false
		m.logFilter.Query = m.searchInput
		m.recompute()
	case "esc":
		m.searching = false
		m.searchInput = ""
		m.logFilter.Query = ""
		m.recompute()
	case "backspace":
		r := []rune(m.searchInput)
		if len(r) > 0 {
			m.searchInput = string(r[:len(r)-1])
		}
	default:
		if k.Type == tea.KeyRunes {
			m.searchInput += string(k.Runes)
		}
	}
	return m, nil
}
func (m Model) filteredLogsBody() string {
	if len(m.visible) == 0 {
		return "0 entries"
	}
	lines := []string{}
	for _, i := range m.visible {
		e := m.entries[i]
		line := e.Time.Format("2006-01-02 15:04:05") + " " + e.Level + " " + e.Message
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return fmt.Sprintf("%d entries  level=%s search=%q\n\n%s", len(m.visible), m.logFilter.Level, m.logFilter.Query, strings.Join(lines, "\n"))
}
func (m Model) groupedLogsBody() string {
	gs := logs.GroupEntries(func() []logs.Entry {
		r := []logs.Entry{}
		for _, i := range m.visible {
			r = append(r, m.entries[i])
		}
		return r
	}())
	lines := make([]string, len(gs))
	for i, g := range gs {
		lines[i] = fmt.Sprintf("%5dx %s %s", g.Count, g.Sample.Level, g.Sample.Message)
	}
	return strings.Join(lines, "\n")
}
func errText(e error) string {
	if e == nil {
		return ""
	}
	return ": " + e.Error()
}

var _ = filepath.Join
var _ = context.Background
var _ = artisan.Runner{}
var _ = jobs.ErrNoTable
var _ = conns.Result{}

func LoadLogs(root string) ([]logs.Entry, error) {
	return logs.LoadTail(logs.LatestLogFile(root, ""), 5<<20)
}
