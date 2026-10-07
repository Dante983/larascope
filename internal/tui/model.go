package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

var (
	activeTabStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	inactiveTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func New() Model {
	return Model{active: TabLogs}
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
	if m.width > 0 {
		body = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, body)
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s", lipgloss.JoinHorizontal(lipgloss.Top, tabs...), body, footerStyle.Render("tab: switch  ?: help  q: quit"))
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
		return "No log file detected yet"
	}
}
