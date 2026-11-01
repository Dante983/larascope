package tui

import (
	"fmt"
	"larascope/internal/config"
	"larascope/internal/conns"
	"larascope/internal/jobs"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"larascope/internal/logs"
)

func TestViewResponsiveLogsAndSearch(t *testing.T) {
	entries := make([]logs.Entry, 200)
	for i := range entries {
		entries[i] = logs.Entry{Header: logs.Header{Time: time.Now(), Level: "INFO", Message: "message"}}
	}
	m := New().WithLogs(entries, nil)
	m.width, m.height = 80, 24
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("overwide line: %q", line)
		}
	}
	m.searching = true
	m.searchInput = "message"
	if !strings.Contains(m.View(), "/ message▏") {
		t.Fatal("search prompt missing")
	}
	m.width, m.height = 30, 8
	if !strings.Contains(m.View(), "Terminal too small (need 40x10)") {
		t.Fatal("small terminal message missing")
	}
	_, _ = m.Update(tea.WindowSizeMsg{})
}

func matrixModel(w, h int) Model {
	entries := make([]logs.Entry, 200)
	for i := range entries {
		message := fmt.Sprintf("message %d 界😀 %s", i, strings.Repeat("long ", 20))
		if i == 0 {
			message = "x\x1b[2J\x1b]0;t\a\rY"
		}
		entries[i] = logs.Entry{Header: logs.Header{Time: time.Now(), Level: "INFO", Message: message}, Trace: []string{"trace line", "another trace line"}}
	}
	m := New().WithLogs(entries, nil)
	m.width, m.height = w, h
	m.settings = config.Settings{Root: "/tmp/app", DBPassword: "s3cr3t", DBConnection: "mysql"}
	m.jobs.loaded, m.jobs.view = true, "pending"
	for i := 0; i < 100; i++ {
		m.jobs.pending = append(m.jobs.pending, jobs.PendingRow{ID: int64(i + 1), Queue: fmt.Sprintf("queue-%d", i%5), State: "ready", Class: strings.Repeat("PendingClass", 15), Attempts: i})
		m.jobs.failed = append(m.jobs.failed, jobs.FailedRow{ID: int64(i + 1), Queue: fmt.Sprintf("queue-%d", i%5), Class: strings.Repeat("FailedClass", 15), Exception: "bad\x1b[31m\n" + strings.Repeat("exception\n", 40), FailedAt: time.Now()})
	}
	for i := 0; i < 6; i++ {
		m.jobs.counts = append(m.jobs.counts, jobs.QueueCount{Queue: fmt.Sprintf("queue-%d", i), Ready: 10 + i, Delayed: i, Reserved: i % 2})
	}
	for i := 0; i < 12; i++ {
		m.conns.results = append(m.conns.results, conns.Result{Name: fmt.Sprintf("connection-%d", i), Target: "target", Err: "password s3cr3t", OK: i%2 == 0})
	}
	return m
}

func assertViewBounds(t *testing.T, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		t.Fatalf("%d lines exceeds height %d: %q", len(lines), h, view)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > w {
			t.Fatalf("width %d exceeds %d: %q", lipgloss.Width(line), w, line)
		}
	}
	if strings.ContainsAny(view, "\x1b\a\r") || strings.Contains(view, "s3cr3t") {
		t.Fatalf("unsafe output: %q", view)
	}
}

func TestViewMatrix(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(old)
	type tabCase struct {
		name    string
		tab     Tab
		jobView string
	}
	cases := []tabCase{{"logs", TabLogs, ""}, {"jobs-pending", TabJobs, "pending"}, {"jobs-failed", TabJobs, "failed"}, {"conns", TabConnections, ""}}
	for _, w := range []int{40, 41, 80, 100, 119, 120, 160} {
		for _, h := range []int{10, 11, 12, 13, 24, 50} {
			for _, c := range cases {
				variants := []string{"plain", "detail", "help"}
				if c.tab == TabLogs {
					variants = append(variants, "searching", "filtered", "grouped")
				}
				if c.jobView == "failed" {
					variants = append(variants, "confirm")
				}
				for _, variant := range variants {
					t.Run(fmt.Sprintf("w%d/h%d/%s/%s", w, h, c.name, variant), func(t *testing.T) {
						m := matrixModel(w, h)
						m.active = c.tab
						if c.jobView != "" {
							m.jobs.view = c.jobView
						}
						switch variant {
						case "detail":
							if c.tab == TabLogs {
								m.detail = true
							} else if c.tab == TabJobs {
								m.jobs.detail = true
							}
						case "help":
							m.showHelp = true
						case "searching":
							if c.tab == TabLogs {
								m.searching, m.searchInput = true, "mess"
							}
						case "filtered":
							if c.tab == TabLogs {
								m.logFilter.Query = "mess"
								m.recompute()
							}
						case "grouped":
							if c.tab == TabLogs {
								m.grouped = true
							}
						case "confirm":
							m.active, m.jobs.view = TabJobs, "failed"
							m.jobs.confirm = &confirmAction{desc: "retry all", need: "RETRY ALL", typed: "ABC"}
						}
						assertViewBounds(t, m.View(), w, h)
					})
				}
			}
		}
	}
}

func TestViewSmallTerminal(t *testing.T) {
	for _, w := range []int{39} {
		m := matrixModel(w, 9)
		if got := m.View(); got != "Terminal too small (need 40x10)\nq: quit" {
			t.Fatalf("small terminal = %q", got)
		}
	}
	m := matrixModel(40, 10)
	if strings.Contains(m.View(), "too small") {
		t.Fatal("boundary terminal rejected")
	}
}

func TestViewJobsDetailSmallHeights(t *testing.T) {
	for _, view := range []string{"pending", "failed"} {
		for _, w := range []int{40, 41, 80, 100, 119, 120, 160} {
			for _, h := range []int{10, 11, 12, 13} {
				m := matrixModel(w, h)
				m.active, m.jobs.view, m.jobs.detail = TabJobs, view, true
				v := m.View()
				heading := "Jobs [" + view + "] detail"
				if m.twoPane() {
					heading = "Jobs [" + view + "]"
				}
				for _, want := range []string{heading, "id:", "queue:"} {
					if !strings.Contains(v, want) {
						t.Fatalf("%s w=%d h=%d missing %q: %q", view, w, h, want, v)
					}
				}
				if w == 80 && view == "failed" && h == 10 && !strings.Contains(v, "class:") && !strings.Contains(v, "more lines") {
					t.Fatalf("failed compact detail missing class/marker: %q", v)
				}
			}
		}
	}
	m := matrixModel(80, 24)
	m.active, m.jobs.view, m.jobs.detail = TabJobs, "pending", true
	if v := m.View(); !strings.Contains(v, "of 100") || !strings.Contains(v, "id:") {
		t.Fatalf("normal detail missing table/detail: %q", v)
	}
	m = matrixModel(120, 10)
	m.active, m.jobs.view, m.jobs.detail = TabJobs, "pending", true
	found := false
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "│") && strings.Contains(line, "id:") && lipgloss.Width(line) <= 120 {
			found = true
		}
	}
	if !found {
		t.Fatal("two-pane detail was not aligned with table")
	}
}

func TestViewWidthZeroNoTruncation(t *testing.T) {
	m := matrixModel(0, 0)
	long := strings.Repeat("x", 150)
	m.entries[len(m.entries)-1].Message = long
	m.jobs.view = "failed"
	m.jobs.failed[0].Class = long
	m.active = TabLogs
	if !strings.Contains(m.View(), long) {
		t.Fatal("unlimited log message truncated")
	}
	m.active = TabJobs
	if !strings.Contains(m.View(), long) {
		t.Fatal("unlimited job class truncated")
	}
	m = matrixModel(0, 0)
	long = strings.Repeat("x", 150)
	m.active = TabJobs
	for _, v := range []string{"pending", "failed"} {
		m.jobs.view, m.jobs.detail = v, true
		m.jobs.pending[0].Class = long
		m.jobs.failed[0].Class = long
		if out := m.View(); !strings.Contains(out, "class: "+long) {
			t.Fatalf("unlimited %s job detail truncated: %q", v, out)
		}
	}
}

func TestViewConfirmFooter(t *testing.T) {
	for _, w := range []int{40, 80} {
		m := matrixModel(w, 24)
		m.active, m.jobs.view = TabJobs, "failed"
		m.jobs.confirm = &confirmAction{desc: "retry all", need: "RETRY ALL", typed: "ABC"}
		lines := strings.Split(m.View(), "\n")
		if !strings.Contains(lines[len(lines)-1], "> ABC") || lipgloss.Width(lines[len(lines)-1]) > w {
			t.Fatalf("bad confirm footer at %d: %q", w, lines[len(lines)-1])
		}
	}
	m := matrixModel(0, 0)
	m.active, m.jobs.view = TabJobs, "failed"
	m.jobs.confirm = &confirmAction{desc: "retry all", need: "RETRY ALL", typed: "ABC"}
	if !strings.Contains(m.View(), "type RETRY ALL then enter") {
		t.Fatal("unlimited confirmation truncated")
	}
	m = matrixModel(40, 24)
	m.active = TabJobs
	m.jobs.confirm = &confirmAction{desc: strings.Repeat("description ", 12)}
	if !strings.Contains(m.View(), "y=yes, any other key=cancel") {
		t.Fatal("single confirmation suffix missing")
	}
}

func TestViewHelpClipped(t *testing.T) {
	m := matrixModel(80, 10)
	m.showHelp = true
	lines := strings.Split(m.View(), "\n")
	if !strings.Contains(lines[len(lines)-3], "more lines") {
		t.Fatalf("help clipping indicator missing: %q", m.View())
	}
	if !strings.HasPrefix(lines[len(lines)-3], "… ") || len(lines) > 10 {
		t.Fatalf("bad help clipping indicator: %q", m.View())
	}
	m.height = 50
	v := m.View()
	if strings.Contains(v, "more lines") || !strings.Contains(v, "Jobs") {
		t.Fatalf("large help is unexpectedly clipped: %q", v)
	}
}

func TestViewTwoPaneAndEscapeSanitization(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	defer lipgloss.SetColorProfile(old)
	for _, w := range []int{120, 160} {
		m := matrixModel(w, 24)
		m.active, m.jobs.detail = TabJobs, true
		v := m.View()
		found := false
		for _, line := range strings.Split(v, "\n") {
			if (strings.Contains(line, "ready") || strings.Contains(line, "PendingClass")) && strings.Contains(line, "│") {
				found = true
			}
			if lipgloss.Width(line) > w {
				t.Fatalf("two-pane line too wide: %q", line)
			}
		}
		headerAtLeft := false
		for _, line := range strings.Split(v, "\n") {
			if strings.Contains(line, "ID") && strings.Contains(line, "STATE") {
				headerAtLeft = true
			}
		}
		if !found || !headerAtLeft {
			t.Fatalf("jobs two pane not aligned: %q", v)
		}
		m.active = TabConnections
		connectionsAtLeft := false
		for _, line := range strings.Split(m.View(), "\n") {
			if strings.Contains(line, "STATUS") && strings.Contains(line, "TARGET") {
				connectionsAtLeft = true
			}
		}
		if !connectionsAtLeft {
			t.Fatalf("connections table header indented: %q", m.View())
		}
	}
	m := matrixModel(80, 24)
	m.active, m.detail = TabLogs, true
	m.entries[0].Trace = []string{"\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\"}
	m.cursor = 0
	m.conns.results[0] = conns.Result{Name: "x\x1b[2J", Target: "y\x1b[2J", Err: "z\x1b[2J"}
	m.jobs.failed[0].Class = "x\x1b[31m"
	v := m.View()
	if strings.Contains(v, "\x1b") || !strings.Contains(v, "link") {
		t.Fatalf("escape sanitization failed: %q", v)
	}
	m.active, m.detail = TabConnections, false
	if v = m.View(); strings.Contains(v, "\x1b") {
		t.Fatalf("connection escape sanitization failed: %q", v)
	}
	m.active, m.jobs.view = TabJobs, "failed"
	if v = m.View(); strings.Contains(v, "\x1b") {
		t.Fatalf("job escape sanitization failed: %q", v)
	}
}
