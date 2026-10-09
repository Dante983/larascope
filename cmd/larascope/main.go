package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/config"
	"larascope/internal/logs"
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
	if cfgErr == nil && settings.Root != "" {
		entries, logErr := logs.LoadFile(filepath.Join(settings.Root, "storage", "logs", "laravel.log"))
		m = m.WithLogs(entries, logErr)
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
