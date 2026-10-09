package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

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
	active        Tab
	width, height int
	showHelp      bool
	settings      config.Settings
	cfgErr        error
	entries       []logs.Entry
	logErr        error
	logLoaded     bool
}

const maxLogLines = 20

var (
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func New() Model {
	return Model{active: TabLogs}
}

func (m Model) WithConfig(s config.Settings, err error) Model {
	m.settings = s
	m.cfgErr = err
	return m
}

func (m Model) WithLogs(entries []logs.Entry, err error) Model {
	m.entries, m.logErr, m.logLoaded = entries, err, true
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
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.active = Tab((int(m.active) + 1) % len(tabNames))
		case "shift+tab":
			m.active = Tab((int(m.active) - 1 + len(tabNames)) % len(tabNames))
		case "1":
			m.active = TabLogs
		case "2":
			m.active = TabJobs
		case "3":
			m.active = TabConnections
		case "?":
			m.showHelp = !m.showHelp
		}
	}

	return m, nil
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
		return "tab: next tab\nshift+tab: previous tab\n1/2/3: select tab\n?: toggle help\nq: quit"
	}

	switch m.active {
	case TabJobs:
		return "No jobs detected yet"
	case TabConnections:
		return "No connections detected yet"
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

	start := max(0, len(m.entries)-maxLogLines)
	lines := make([]string, 0, len(m.entries)-start)
	for _, entry := range m.entries[start:] {
		line := entry.Time.Format("2006-01-02 15:04:05") + " " + entry.Level + " " + entry.Message
		if m.width > 0 {
			runes := []rune(line)
			if len(runes) > m.width {
				line = string(runes[:m.width])
			}
		}
		lines = append(lines, line)
	}

	return fmt.Sprintf("%d entries\n\n%s", len(m.entries), strings.Join(lines, "\n"))
}
