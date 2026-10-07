package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/tui"
)

func main() {
	m := tui.New()
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
