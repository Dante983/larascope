package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Dante983/larascope/internal/logs"
)

func TestSummarizeLogs(t *testing.T) {
	entries := []logs.Entry{
		{Header: logs.Header{Time: time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC), Level: "notice"}},
		{Header: logs.Header{Time: time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC), Level: "CRITICAL"}},
		{Header: logs.Header{Time: time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC), Level: "error"}},
	}
	s := summarizeLogs(entries)
	if s.total != 3 || s.errors != 2 || s.info != 1 || len(s.recentErrors) != 2 || s.recentErrors[0] != 2 {
		t.Fatalf("unexpected summary: %#v", s)
	}
}

func TestOverviewDemoAndKeys(t *testing.T) {
	m := NewDemo()
	m.active, m.width, m.height = TabOverview, 120, 40
	view := m.View()
	for _, want := range []string{"Logs", "Jobs", "Services", "9", "1 down", "mail"} {
		if !strings.Contains(view, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	m, _ = updateWithKey(t, m, runeKey("e"))
	if m.active != TabLogs || m.logFilter.Level != "ERROR" {
		t.Fatal("e did not open error logs")
	}
	m.active = TabOverview
	m, _ = updateWithKey(t, m, runeKey("f"))
	if m.active != TabJobs || m.jobs.view != "failed" {
		t.Fatal("f did not open failed jobs")
	}
}
