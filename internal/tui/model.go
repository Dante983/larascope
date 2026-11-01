package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	demo            bool
	notice          string
	corr            correlationState
	logNote         string
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
	m.corr = correlationState{}
	return m
}

func (m Model) statusLine() string {
	if m.demo {
		if m.notice != "" {
			return "DEMO · " + m.notice
		}
		return "DEMO · sample data · no database or Artisan access"
	}
	if errors.Is(m.cfgErr, config.ErrNotLaravel) {
		return "Not a Laravel project (no artisan found) · try: larascope --demo"
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

	return sanitize(strings.Join(parts, "  "))
}

func (m Model) Init() tea.Cmd {
	if m.demo {
		return nil
	}
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
		if m.liveAllowed() && m.follow && m.active == TabLogs && m.settings.Root != "" {
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
		if m.demo {
			return m, nil
		}
		if m.active != TabConnections {
			m.conns.connsTicking = false
			return m, nil
		}
		return m, tea.Batch(m.startConnsLoad(), connsTick())
	case artisanDoneMsg:
		m.jobs.status = config.Redact(msg.out+errText(msg.err), m.settings.Secrets()...)
		if !m.liveAllowed() {
			return m, nil
		}
		return m, loadJobsCmd(m.settings)
	case tea.KeyMsg:
		if msg.String() != "r" && msg.String() != "f" && msg.String() != "J" {
			m.notice = ""
			m.logNote = ""
		}
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
						a.typed += sanitize(string(msg.Runes))
					}
				}
			}
			return m, nil
		}
		if m.active == TabLogs && m.searching {
			return m.handleSearch(msg)
		}
		if m.active == TabLogs && m.corr.active && !m.showHelp {
			if nm, cmd, handled := m.handleCorrKey(msg); handled {
				return nm, cmd
			}
		}
		if m.active == TabJobs && !m.showHelp {
			switch msg.String() {
			case "p", "R", "A", "d", "F", "L", "up", "k", "down", "j", "enter", "esc":
				m, cmd := m.handleJobsKey(msg.String())
				return m, cmd
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.active = Tab((int(m.active) + 1) % len(tabNames))
			m.notice = ""
			return m, m.tabLoadCmd()
		case "shift+tab":
			m.active = Tab((int(m.active) - 1 + len(tabNames)) % len(tabNames))
			m.notice = ""
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
			if m.demo {
				m.notice = "demo: sample data, refresh disabled"
				return m, nil
			}
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
			if m.demo {
				m.notice = "demo: sample data, refresh disabled"
				return m, nil
			}
			if m.active == TabLogs && !m.showHelp {
				m.follow = !m.follow
				if m.follow {
					return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return followTickMsg{} })
				}
			}
		case "J":
			if m.active == TabLogs && m.detail && !m.showHelp {
				m = m.jumpToJobFromLog()
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
			if m.active == TabConnections && !m.showHelp {
				m.conns.cursor = clampCursor(m.conns.cursor-1, len(m.conns.results))
			}
		case "down", "j":
			if m.active == TabLogs && !m.showHelp {
				m = m.moveCursor(1)
			}
			if m.active == TabConnections && !m.showHelp {
				m.conns.cursor = clampCursor(m.conns.cursor+1, len(m.conns.results))
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
				m = m.moveCursor(-m.listHeight())
			}
		case "pgdown":
			if m.active == TabLogs && !m.showHelp && !m.detail {
				m = m.moveCursor(m.listHeight())
			}
		}
	}

	return m, nil
}

func (m Model) reloadLogs(msg logsLoadedMsg) Model {
	m.corr = correlationState{}
	if msg.err != nil {
		m.logErr = msg.err
		m.logLoaded = true
		return m
	}

	oldEntriesEmpty := len(m.entries) == 0
	wasLast := m.cursor >= len(m.entries)-1
	m.entries = msg.entries
	m.corr = correlationState{}
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
	if !m.liveAllowed() {
		return nil
	}
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
	if m.width > 0 && (m.width < 40 || m.height < 10) {
		return "Terminal too small (need 40x10)\nq: quit"
	}
	bar := strings.Split(tabBar(m.active, m.contentWidth()), "\n")
	top := lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render("larascope")
	if m.demo {
		top += "  " + lipgloss.NewStyle().Foreground(colWarn).Reverse(true).Bold(true).Render(" DEMO ")
	}
	top += "  " + bar[0]
	if m.width >= 80 && m.settings.Root != "" {
		top += "  " + filepath.Base(m.settings.Root)
	}
	header := truncate(top, m.clipWidth()) + "\n" + bar[1]
	body := m.body()
	if m.height > 0 {
		body = clipBody(body, m.bodyHeight())
	}
	body = clipLines(body, m.clipWidth())
	if m.width > 0 && m.centered() {
		body = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, body)
	}
	footer := m.footer()
	return fmt.Sprintf("%s\n%s\n%s", header, body, footer)
}
func (m Model) footer() string {
	cw := m.clipWidth()
	hints := "tab: switch  ?: help  q: quit"
	if m.active == TabLogs {
		if m.searching {
			hints = "type query, enter apply, esc clear"
		} else if h := m.correlationHints(); h != "" {
			hints = h
		} else if m.detail {
			hints = "up/down: previous/next entry  enter/esc: back  J: failed job  ?: help"
		} else if m.demo {
			hints = "up/down: move  enter: detail  /: search  l: level  s: group  ?: help"
		} else {
			hints = "up/down: move  enter: detail  /: search  l: level  s: group  f: follow"
		}
	}
	if m.active == TabJobs {
		hints = "p: pending/failed  j/k: move  enter: detail"
		if h := m.correlationHints(); h != "" {
			hints += "  " + h
		}
		hints += "  R retry  d forget  A retry all  F flush"
	}
	if m.active == TabConnections {
		if m.demo {
			hints = "up/down: move  ?: help  q: quit"
		} else {
			hints = "up/down: move  r: refresh  ?: help  q: quit"
		}
	}
	if m.jobs.confirm != nil {
		if m.jobs.confirm.need == "" {
			suffix := "  y=yes, any other key=cancel"
			desc := "Confirm: " + sanitizeLine(m.jobs.confirm.desc)
			if cw > 0 {
				desc = truncate(desc, max(1, cw-lipgloss.Width(suffix)))
			}
			hints = desc + suffix
		} else {
			prefix := "Confirm: " + sanitizeLine(m.jobs.confirm.desc) + " — type " + m.jobs.confirm.need + " then enter, esc cancel"
			tail := " > " + sanitizeLine(m.jobs.confirm.typed)
			if cw > 0 {
				if lipgloss.Width(tail) >= cw {
					hints = ansi.TruncateLeft(tail, lipgloss.Width(tail)-cw+1, "…")
				} else {
					hints = truncateStrict(prefix, cw-lipgloss.Width(tail)) + tail
				}
			} else {
				hints = prefix + tail
			}
		}
		hints = lipgloss.NewStyle().Foreground(colWarn).Bold(true).Render(hints)
	} else {
		hints = truncate(hints, cw)
	}
	return lipgloss.NewStyle().Foreground(colMuted).Render(truncate(sanitizeLine(m.statusLine()), cw)) + "\n" + hints
}

func (m Model) centered() bool {
	if m.showHelp {
		return false
	}
	if m.active == TabLogs {
		return !m.logLoaded || m.logErr != nil || len(m.entries) == 0
	}
	return (m.active == TabJobs && m.settings.Root == "" && !m.demo) || (m.active == TabConnections && (m.conns.loading && len(m.conns.results) == 0 || len(m.conns.results) == 0))
}

func (m Model) body() string {
	if m.showHelp {
		help := "Global\n" + keyHint("tab", "next tab") + "\n" + keyHint("1/2/3", "select tab") + "\n" + keyHint("?", "toggle help") + "\n" + keyHint("q", "quit") + "\n\nUse larascope --demo for offline sample data.\n\nLogs\nup/k, down/j: move in logs\ng/G: first/last entry\npgup/pgdown: page up/down\nenter: toggle entry detail\nesc: back to list\n"
		if m.demo {
			help += "l: level filter  /: search  s: group\nreload and follow disabled in demo\n"
		} else {
			help += "r: reload log file\nl: level filter  /: search  s: group  f: follow\n"
		}
		help += "J  (Logs detail/related list) jump to matching failed job\nesc leaves related-log mode\n\nJobs\np: pending/failed  j/k: move  enter: detail  R/d: retry/forget selected  A: retry all (type to confirm)  F: flush (type to confirm)\nL  (Jobs, failed) show related log entries\n\nConnections\nup/down: move"
		if !m.demo {
			help += "  r: refresh"
		}
		return lipgloss.NewStyle().Width(max(1, m.contentWidth()-4)).Border(lipgloss.RoundedBorder()).BorderForeground(colMuted).Padding(0, 1).Render(help)
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
		return "Log error: " + sanitizeLine(config.Redact(m.logErr.Error(), m.settings.Secrets()...))
	}
	if len(m.entries) == 0 {
		return "Log file has no entries"
	}
	if m.detail {
		return m.detailBody()
	}
	if m.corr.active {
		return m.correlationBody()
	}
	if m.grouped {
		return m.groupedLogsBody()
	}
	if len(m.visible) != len(m.entries) || m.logFilter.Level != "" || m.logFilter.Query != "" {
		return m.filteredLogsBody()
	}

	return m.logTableBody(m.visible)
}

func levelCounts(entries []logs.Entry, idx []int) map[string]int {
	out := map[string]int{}
	for _, i := range idx {
		out[strings.ToUpper(entries[i].Level)]++
	}
	return out
}

func (m Model) detailBody() string {
	idx := m.shownEntryIndex()
	if idx < 0 || idx >= len(m.entries) {
		return ""
	}
	entry := m.entries[idx]
	w := m.clipWidth()
	message := strings.Split(wrap(sanitize(entry.Message), w), "\n")
	messageCap := max(3, m.bodyHeight()/3)
	if m.height == 0 {
		messageCap = len(message)
	}
	if len(message) > messageCap {
		message = append(message[:messageCap], fmt.Sprintf("… %d more lines", len(message)-messageCap))
	}
	body := levelStyle(entry.Level).Render(sanitizeLine(entry.Level)) + " " + entry.Time.Format("2006-01-02 15:04:05") + " " + sanitizeLine(entry.Env) + fmt.Sprintf("  entry %d/%d", idx+1, len(m.entries)) + "\n\n" + strings.Join(message, "\n")
	if m.jobs.loaded && len(m.jobs.failed) > 0 {
		if line := m.relatedJobLine(entry); line != "" {
			body += "\n" + lipgloss.NewStyle().Foreground(colMuted).Render(line)
		}
	}
	if m.logNote != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(colMuted).Render(sanitizeLine(m.logNote))
	}
	if len(entry.Trace) > 0 {
		trace := make([]string, 0, len(entry.Trace))
		for _, line := range entry.Trace {
			trace = append(trace, strings.Split(wrap(sanitizeLine(line), w), "\n")...)
		}
		available := max(0, m.bodyHeight()-(2+len(message)+2))
		if m.height == 0 {
			available = max(0, m.listHeight()-6)
		}
		n := min(len(trace), available)
		body += "\n\n" + lipgloss.NewStyle().Foreground(colMuted).Render(strings.Join(trace[:n], "\n"))
		if n < len(trace) {
			body += fmt.Sprintf("\n… %d more lines", len(trace)-n)
		}
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
			m.searchInput += sanitize(string(k.Runes))
		}
	}
	return m, nil
}
func (m Model) filteredLogsBody() string {
	if len(m.visible) == 0 {
		return "0 entries\nesc in search clears filter / l cycles level"
	}
	return m.logTableBody(m.visible)
}
func (m Model) logTableBody(indices []int) string {
	rows := make([][]Cell, len(indices))
	cursor := -1
	for n, i := range indices {
		e := m.entries[i]
		if i == m.cursor {
			cursor = n
		}
		tm := e.Time.Format("15:04:05")
		if m.width == 0 || m.width >= 80 {
			tm = e.Time.Format("2006-01-02 15:04:05")
		}
		st := levelStyle(sanitizeLine(e.Level))
		rows[n] = []Cell{{Text: tm}, {Text: sanitizeLine(e.Level), Style: &st}, {Text: sanitizeLine(e.Env)}, {Text: sanitizeLine(strings.SplitN(e.Message, "\n", 2)[0]), Mark: sanitizeLine(m.logFilter.Query)}}
	}
	cols := []Column{{"TIME", 8, 19, false, AlignLeft, 3}, {"LEVEL", 5, 9, false, AlignLeft, 0}, {"ENV", 3, 10, false, AlignLeft, 2}, {"MESSAGE", 10, 0, true, AlignLeft, 0}}
	t := Table{Cols: cols, Rows: rows, Cursor: cursor, Width: m.clipWidth()}
	lines, s, e := t.Render(m.listHeight())
	left := fmt.Sprintf("%d entries", len(indices))
	if m.width >= 80 {
		c := levelCounts(m.entries, indices)
		left += fmt.Sprintf(" · %d error · %d warn", c["ERROR"], c["WARNING"])
	}
	right := ""
	if len(indices) > 0 {
		right = fmt.Sprintf("%d–%d of %d", s+1, e, len(indices))
	}
	prefix := []string{metaLine(left, right, m.clipWidth())}
	if m.searching {
		prefix = append(prefix, "/ "+sanitizeLine(m.searchInput)+"▏")
	}
	return strings.Join(append(prefix, lines...), "\n")
}
func (m Model) liveAllowed() bool { return !m.demo }
func (m Model) groupedLogsBody() string {
	gs := logs.GroupEntries(func() []logs.Entry {
		r := []logs.Entry{}
		for _, i := range m.visible {
			r = append(r, m.entries[i])
		}
		return r
	}())
	rows := make([][]Cell, len(gs))
	for i, g := range gs {
		st := levelStyle(sanitizeLine(g.Sample.Level))
		rows[i] = []Cell{{Text: fmt.Sprint(g.Count)}, {Text: sanitizeLine(g.Sample.Level), Style: &st}, {Text: sanitizeLine(strings.SplitN(g.Sample.Message, "\n", 2)[0])}}
	}
	lines, _, _ := (Table{Cols: []Column{{"COUNT", 3, 0, false, AlignRight, 0}, {"LEVEL", 5, 9, false, AlignLeft, 0}, {"MESSAGE", 10, 0, true, AlignLeft, 0}}, Rows: rows, Cursor: -1, Width: m.clipWidth()}).Render(m.listHeight())
	return strings.Join(append([]string{metaLine(fmt.Sprintf("Grouped (%d unique)", len(gs)), "", m.clipWidth())}, lines...), "\n")
}
func errText(e error) string {
	if e == nil {
		return ""
	}
	return ": " + e.Error()
}

func LoadLogs(root string) ([]logs.Entry, error) {
	return logs.LoadTail(logs.LatestLogFile(root, ""), 5<<20)
}
