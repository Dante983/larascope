package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/config"
	"larascope/internal/tui"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	settings, cfgErr := config.Load(cwd)
	m := tui.New().WithConfig(settings, cfgErr)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
