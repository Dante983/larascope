package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/config"
	"larascope/internal/tui"
)

func main() {
	demo := flag.Bool("demo", false, "offline demo with sample data (no database, no Artisan, no network)")
	flag.Parse()
	if *demo {
		run(tui.NewDemo())
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	settings, cfgErr := config.Load(cwd)
	m := tui.New().WithConfig(settings, cfgErr)
	if cfgErr == nil && settings.Root != "" {
		entries, logErr := tui.LoadLogs(settings.Root)
		m = m.WithLogs(entries, logErr)
	}
	run(m)
}
func run(m tui.Model) {
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
