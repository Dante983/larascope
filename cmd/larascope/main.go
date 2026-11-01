package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Dante983/larascope/internal/config"
	"github.com/Dante983/larascope/internal/tui"
	"github.com/Dante983/larascope/internal/version"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	demo := flag.Bool("demo", false, "offline demo with sample data (no database, no Artisan, no network)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}
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
