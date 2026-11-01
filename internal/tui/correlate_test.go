package tui

import (
	"errors"
	"github.com/Dante983/larascope/internal/correlate"
	"github.com/Dante983/larascope/internal/jobs"
	"github.com/Dante983/larascope/internal/logs"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"time"
)

func key(m Model, k string) Model {
	if len([]rune(k)) != 1 {
		panic("key only accepts a single rune; use keyType for special keys")
	}
	x, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	return x.(Model)
}

func keyType(m Model, t tea.KeyType) Model {
	x, _ := m.Update(tea.KeyMsg{Type: t})
	return x.(Model)
}

// corrModel deliberately uses a normal Logs cursor that differs from the
// related-list cursor, making accidental generic-key fall-through observable.
func corrModel(t *testing.T) Model {
	t.Helper()
	m := NewDemo()
	if len(m.entries) < 4 {
		t.Fatal("demo must provide at least four log entries")
	}
	m.active, m.cursor, m.grouped, m.follow = TabLogs, 3, true, true
	m.corr = correlationState{
		active: true, job: m.jobs.failed[0], prevCursor: 3, detailLog: -1,
		matches: []correlate.Match{{LogIndex: 0}, {LogIndex: 1}, {LogIndex: 2}},
	}
	return m
}

func TestCorrelationKeyGuardSeparatesNormalCursor(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"k", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}},
		{"j", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}}, {"down", tea.KeyMsg{Type: tea.KeyDown}},
		{"page up", tea.KeyMsg{Type: tea.KeyPgUp}}, {"page down", tea.KeyMsg{Type: tea.KeyPgDown}},
		{"search", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}},
		{"level", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}},
		{"group", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}},
		{"follow", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}},
		{"unknown", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := corrModel(t)
			beforeCursor, beforeGrouped, beforeFollow := m.cursor, m.grouped, m.follow
			x, _ := m.Update(tc.msg)
			m = x.(Model)
			if m.cursor != beforeCursor || !m.corr.active || m.searching || m.grouped != beforeGrouped || m.follow != beforeFollow {
				t.Fatalf("key escaped correlation mode: %#v", m)
			}
		})
	}
}

func TestCorrelationUsesRealEnterAndEsc(t *testing.T) {
	m := corrModel(t)
	m = keyType(m, tea.KeyDown)
	m = keyType(m, tea.KeyEnter)
	if !m.detail || m.corr.detailLog != m.corr.matches[1].LogIndex || m.cursor != 3 {
		t.Fatalf("enter did not open selected correlation detail: %#v", m.corr)
	}
	m = keyType(m, tea.KeyEsc)
	if m.detail || !m.corr.active || m.corr.cursor != 1 || m.cursor != 3 {
		t.Fatalf("detail esc changed correlation state incorrectly: %#v", m)
	}
	m = keyType(m, tea.KeyEsc)
	if m.corr.active || m.cursor != 3 || m.detail {
		t.Fatalf("list esc did not restore normal logs state: %#v", m)
	}
}

func TestCorrelationRuneEnterIsNotEnter(t *testing.T) {
	m := corrModel(t)
	// This is ordinary typed text, not tea.KeyEnter.
	x, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	m = x.(Model)
	if m.detail || m.cursor != 3 || !m.corr.active {
		t.Fatalf("runes spelling enter opened detail: %#v", m)
	}
}
func TestDemoCorrelationNavigation(t *testing.T) {
	m := NewDemo()
	m.active = TabJobs
	m.jobs.view = "failed"
	m = key(m, "L")
	if m.active != TabLogs || !m.corr.active || len(m.corr.matches) == 0 || m.corr.matches[0].Confidence.String() != "high" {
		t.Fatalf("correlation %#v", m.corr)
	}
	if !strings.Contains(m.View(), "Related to failed job 100") {
		t.Fatal("missing correlation table")
	}
	m = keyType(m, tea.KeyEnter)
	m = key(m, "J")
	if m.active != TabJobs || !m.jobs.detail || m.jobs.failedCursor != 0 {
		t.Fatalf("jump failed: %#v", m.jobs)
	}
}
func TestCorrelationNoTimeOnly(t *testing.T) {
	m := NewDemo()
	m.active = TabJobs
	m.jobs.view = "failed"
	m.jobs.failedCursor = 2
	m = key(m, "L")
	if m.active != TabJobs || !strings.Contains(m.jobs.status, "no related log entries") {
		t.Fatalf("unexpected %q", m.jobs.status)
	}
}

func kt(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func kr(s string) tea.KeyMsg {
	if len([]rune(s)) != 1 {
		panic("kr only accepts one rune")
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func pressSeq(m Model, keys ...tea.KeyMsg) Model {
	for _, k := range keys {
		x, _ := m.Update(k)
		m = x.(Model)
	}
	return m
}

func sameCorr(a, b correlationState) bool {
	if a.active != b.active || a.job != b.job || a.cursor != b.cursor || a.detailLog != b.detailLog || a.prevCursor != b.prevCursor || a.prevDetail != b.prevDetail || len(a.matches) != len(b.matches) || len(a.byLog) != len(b.byLog) {
		return false
	}
	for i := range a.matches {
		if a.matches[i] != b.matches[i] {
			return false
		}
	}
	for k, v := range a.byLog {
		if bv, ok := b.byLog[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// realCorrModel builds a deterministic model with real correlation matches.
func realCorrModel(t *testing.T) Model {
	t.Helper()
	base := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	m := Model{active: TabLogs, logLoaded: true, width: 120, height: 30}
	m.jobs = jobsState{view: "failed", loaded: true, failed: []jobs.FailedRow{
		{ID: 7, UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Class: "App\\Jobs\\A", FailedAt: base},
		{ID: 8, UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Class: "App\\Jobs\\B", FailedAt: base},
	}}
	msg := []string{
		"NORMAL-ROW-MARKER no ids",
		`failed CORR-ROW-MARKER-1 {"failed_job_id":7}`,
		`failed CORR-ROW-MARKER-2 {"failed_job_id":8}`,
		`job CORR-ROW-MARKER-3 {"failed_job_id":7}`,
		"no ids but uuid-free text NOID-MARKER",
		"filler 5", "filler 6", "filler 7", "filler 8", "filler 9",
	}
	for i, s := range msg {
		m.entries = append(m.entries, logs.Entry{Header: logs.Header{Time: base.Add(time.Duration(i) * time.Second), Env: "local", Level: "ERROR", Message: s}})
	}
	m.recompute()
	m.cursor = 0
	m.jobs.failedCursor = 0
	m = m.enterCorrelation(m.jobs.failed[0])
	if !m.corr.active || len(m.corr.matches) < 2 {
		t.Fatalf("correlation fixture has no real matches: %#v", m.corr)
	}
	return m
}

func TestCorrListNavigation(t *testing.T) {
	base := corrModel(t)
	base.entries = append(base.entries, make([]logs.Entry, 10)...)
	base.corr.matches = make([]correlate.Match, 10)
	for i := range base.corr.matches {
		base.corr.matches[i].LogIndex = i % len(base.entries)
	}
	last, page := len(base.corr.matches)-1, base.listHeight()
	for _, tc := range []struct {
		name  string
		start int
		key   tea.KeyMsg
		want  int
	}{
		{"down", 0, kt(tea.KeyDown), 1}, {"up clamps", 0, kt(tea.KeyUp), 0},
		{"j", 0, kr("j"), 1}, {"k", 1, kr("k"), 0}, {"G", 0, kr("G"), last},
		{"g", last, kr("g"), 0}, {"page down", 0, kt(tea.KeyPgDown), min(last, page)},
		{"page up", last, kt(tea.KeyPgUp), max(0, last-page)}, {"page down clamps", last, kt(tea.KeyPgDown), last},
		{"ctrl+a", 3, tea.KeyMsg{Type: tea.KeyCtrlA}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			m.corr.cursor = tc.start
			before := m.cursor
			m = pressSeq(m, tc.key)
			if m.corr.cursor != tc.want || m.cursor != before || !m.corr.active {
				t.Fatalf("got corr=%d cursor=%d active=%v", m.corr.cursor, m.cursor, m.corr.active)
			}
		})
	}
}

func TestCorrSingleMatch(t *testing.T) {
	for _, k := range []tea.KeyMsg{kt(tea.KeyUp), kt(tea.KeyDown), kt(tea.KeyPgUp), kt(tea.KeyPgDown), kr("g"), kr("G")} {
		m := corrModel(t)
		m.corr.matches = m.corr.matches[:1]
		m = pressSeq(m, k)
		if m.corr.cursor != 0 {
			t.Fatalf("%s moved single cursor", k)
		}
	}
	m := corrModel(t)
	m.corr.matches = m.corr.matches[:1]
	m = pressSeq(m, kt(tea.KeyEnter))
	if !m.detail {
		t.Fatal("enter did not open detail")
	}
	m = pressSeq(m, kt(tea.KeyEsc))
	if m.detail {
		t.Fatal("esc did not close detail")
	}
}

func TestCorrEmptyMatchesClears(t *testing.T) {
	m := corrModel(t)
	m.corr.matches = nil
	m = pressSeq(m, kt(tea.KeyDown))
	if m.corr.active {
		t.Fatal("empty correlation was not cleared")
	}
}

func TestCorrHelpAndAllowList(t *testing.T) {
	m := corrModel(t)
	m = pressSeq(m, kr("?"))
	before := m.corr.cursor
	m = pressSeq(m, kr("j"), kt(tea.KeyDown))
	if !m.showHelp || m.corr.cursor != before || !m.corr.active {
		t.Fatal("help did not guard correlation")
	}
	m = pressSeq(m, kr("?"))
	if m.showHelp {
		t.Fatal("help did not close")
	}
	beforeLog := m.cursor
	m = pressSeq(m, kt(tea.KeyTab))
	if m.active == TabLogs || !m.corr.active || m.corr.cursor != before || m.cursor != beforeLog {
		t.Fatal("tab changed corr state")
	}
	m = pressSeq(m, kr("1"))
	if m.active != TabLogs || !strings.Contains(m.View(), "Related to failed job") {
		t.Fatal("logs did not retain correlation list")
	}
	for _, k := range []tea.KeyMsg{kr("q"), kt(tea.KeyCtrlC)} {
		x, cmd := m.Update(k)
		_ = x
		if cmd == nil {
			t.Fatalf("%s did not return quit command", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s command was not QuitMsg", k)
		}
	}
	beforeCorr, beforeCursor, beforeDetail := m.corr, m.cursor, m.detail
	m = pressSeq(m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if !sameCorr(m.corr, beforeCorr) || m.cursor != beforeCursor || m.detail != beforeDetail {
		t.Fatal("ctrl+a changed list state")
	}
	m = pressSeq(m, kt(tea.KeyEnter))
	beforeCorr, beforeCursor, beforeDetail = m.corr, m.cursor, m.detail
	m = pressSeq(m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if !sameCorr(m.corr, beforeCorr) || m.cursor != beforeCursor || m.detail != beforeDetail {
		t.Fatal("ctrl+a changed detail state")
	}
}

func TestCorrJumpFromList(t *testing.T) {
	newModel := func(t *testing.T) Model {
		m := realCorrModel(t)
		m.cursor = 3
		m = m.enterCorrelation(m.jobs.failed[0])
		return m
	}
	t.Run("matched", func(t *testing.T) {
		m := newModel(t)
		li := m.corr.matches[m.corr.cursor].LogIndex
		want := correlate.JobsForLog(m.entries[li], m.jobs.failed, 5)[0].JobIndex
		m = pressSeq(m, kr("J"))
		if m.active != TabJobs || m.jobs.failedCursor != want || m.jobs.detail || m.corr.active || m.cursor != 3 || m.detail || !strings.Contains(m.jobs.status, "correlated from log list:") {
			t.Fatalf("bad matched jump: %#v", m)
		}
	})
	for _, tc := range []struct {
		name    string
		setup   func(*Model)
		status  string
		remains bool
	}{
		{"fallback", func(m *Model) {
			m.corr.matches = append(m.corr.matches, correlate.Match{LogIndex: 4})
			m.corr.cursor = len(m.corr.matches) - 1
		}, "back to failed job 7", false},
		{"vanished", func(m *Model) {
			m.corr.matches = append(m.corr.matches, correlate.Match{LogIndex: 4})
			m.corr.cursor = len(m.corr.matches) - 1
			m.jobs.failed = append([]jobs.FailedRow(nil), m.jobs.failed[1:]...)
		}, "failed job no longer in list", false},
		{"empty", func(m *Model) { m.jobs.failed = nil }, "open Jobs tab first to load failed jobs", true},
		{"unloaded", func(m *Model) { m.jobs.loaded = false }, "open Jobs tab first to load failed jobs", true},
		{"invalid", func(m *Model) { m.corr.cursor = 99 }, "back to failed job 7", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(t)
			before := m
			tc.setup(&m)
			m = pressSeq(m, kr("J"))
			if tc.remains {
				if m.logNote != tc.status || !m.corr.active || m.active != TabLogs || m.cursor != before.cursor || m.detail != before.detail {
					t.Fatalf("bad unloaded jump: %#v", m)
				}
				return
			}
			if m.jobs.status != tc.status || m.corr.active {
				t.Fatalf("bad fallback: status=%q state=%#v", m.jobs.status, m)
			}
			if tc.name == "fallback" && m.jobs.failedCursor != 0 {
				t.Fatal("fallback did not select job 7")
			}
			if tc.name == "vanished" && (m.jobs.failedCursor < 0 || m.jobs.failedCursor >= len(m.jobs.failed)) {
				t.Fatal("invalid vanished cursor")
			}
		})
	}
}

func TestCorrJumpFromDetail(t *testing.T) {
	m := realCorrModel(t)
	prev := m.cursor
	m = pressSeq(m, kt(tea.KeyEnter), kr("J"))
	if m.active != TabJobs || !m.jobs.detail || !strings.Contains(m.jobs.status, "matched from log:") || m.corr.active || m.cursor != prev || m.detail {
		t.Fatalf("matched detail jump: %#v", m)
	}
	m = realCorrModel(t)
	m.corr.matches = []correlate.Match{{LogIndex: 4}}
	m = pressSeq(m, kt(tea.KeyEnter), kr("J"))
	if m.logNote != "no matching failed job" || !m.detail || !m.corr.active || m.active != TabLogs {
		t.Fatalf("unmatched detail jump: %#v", m)
	}
	m.corr.detailLog = 999
	beforeCorr, beforeActive := m.corr, m.active
	m = pressSeq(m, kr("J"))
	if !sameCorr(m.corr, beforeCorr) || m.active != beforeActive {
		t.Fatal("invalid detail log changed state")
	}
	m = realCorrModel(t)
	m = pressSeq(m, kt(tea.KeyEnter))
	m.jobs.loaded = false
	m = pressSeq(m, kr("J"))
	if m.logNote != "open Jobs tab first to load failed jobs" || !m.detail {
		t.Fatal("unloaded detail changed state")
	}
}

func TestCorrReloadAndEscClear(t *testing.T) {
	for _, detail := range []bool{false, true} {
		m := realCorrModel(t)
		if detail {
			m = pressSeq(m, kt(tea.KeyEnter))
		}
		x, _ := m.Update(logsLoadedMsg{entries: m.entries[:2]})
		m = x.(Model)
		if m.corr.active {
			t.Fatal("reload retained correlation")
		}
		_ = m.View()
		if m.detail && m.cursor >= len(m.entries) {
			t.Fatal("invalid detail cursor after reload")
		}
	}
	m := realCorrModel(t)
	x, _ := m.Update(logsLoadedMsg{err: errors.New("x")})
	if x.(Model).corr.active {
		t.Fatal("error reload retained correlation")
	}
	m = corrModel(t)
	before := m.cursor
	m = pressSeq(m, kr("r"))
	_ = m.View()
	if m.corr.active && m.cursor != before {
		t.Fatal("demo reload corrupted cursor")
	}
	m = realCorrModel(t)
	m = pressSeq(m, kt(tea.KeyEnter), kt(tea.KeyEsc), kt(tea.KeyEsc), kt(tea.KeyEsc))
	if m.active != TabLogs {
		t.Fatal("esc left logs")
	}
	m = realCorrModel(t)
	m.corr.prevCursor = 999
	m = pressSeq(m, kt(tea.KeyEsc))
	if m.cursor != len(m.entries)-1 {
		t.Fatal("esc did not clamp previous cursor")
	}
}

func TestCorrHints(t *testing.T) {
	cases := []struct {
		name, want string
		setup      func(*Model)
	}{
		{"jobs failed", "L: related logs", func(m *Model) { m.active = TabJobs; m.jobs.view = "failed" }},
		{"jobs pending", "", func(m *Model) { m.active = TabJobs; m.jobs.view = "pending" }},
		{"logs normal", "", func(m *Model) { m.active = TabLogs; m.corr = correlationState{} }},
		{"logs normal detail", "", func(m *Model) { m.active = TabLogs; m.corr = correlationState{}; m.detail = true }},
		{"correlation list", "enter: detail · J: job · esc: back", func(m *Model) {}},
		{"correlation detail", "J: job · esc: list", func(m *Model) { *m = pressSeq(*m, kt(tea.KeyEnter)) }},
	}
	for _, demo := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.name+"/demo="+map[bool]string{false: "false", true: "true"}[demo], func(t *testing.T) {
				var m Model
				if demo && strings.HasPrefix(tc.name, "correlation") {
					m = corrModel(t)
				} else if demo {
					m = NewDemo()
				} else {
					m = realCorrModel(t)
				}
				m.width, m.height = 120, 30
				tc.setup(&m)
				if got := m.correlationHints(); got != tc.want {
					t.Fatalf("got %q want %q", got, tc.want)
				}
				v := m.View()
				if tc.want != "" && !strings.Contains(v, tc.want) {
					t.Fatalf("view misses hint %q", tc.want)
				}
				if tc.name == "logs normal detail" && !strings.Contains(v, "J: failed job") {
					t.Fatal("normal detail footer misses J")
				}
				if tc.name == "jobs pending" && strings.Contains(v, "L: related logs") {
					t.Fatal("pending jobs advertised related logs")
				}
			})
		}
	}
	for _, demo := range []bool{false, true} {
		var m Model
		if demo {
			m = NewDemo()
		} else {
			m = realCorrModel(t)
		}
		m.width, m.height, m.showHelp = 120, 30, true
		v := m.View()
		for _, s := range []string{"jump to matching failed job", "show related log entries"} {
			if !strings.Contains(v, s) {
				t.Fatalf("help misses %q", s)
			}
		}
		if strings.Contains(v, "disabled in demo") != demo {
			t.Fatalf("demo help mismatch demo=%v", demo)
		}
	}
}

func TestCorrDetailContent(t *testing.T) {
	m := realCorrModel(t)
	// Select the deterministic row carrying marker 3 (the second job-7 match).
	for m.corr.matches[m.corr.cursor].LogIndex != 3 {
		m = pressSeq(m, kt(tea.KeyDown))
	}
	m = pressSeq(m, kt(tea.KeyEnter))
	v := m.View()
	if !strings.Contains(v, "CORR-ROW-MARKER-3") || strings.Contains(v, "NORMAL-ROW-MARKER") || m.shownEntryIndex() != m.corr.detailLog {
		t.Fatalf("wrong correlated detail: %q", v)
	}
	beforeLog, beforeCorr, beforeCursor := m.corr.detailLog, m.corr.cursor, m.cursor
	for _, k := range []tea.KeyMsg{kt(tea.KeyDown), kr("j"), kt(tea.KeyPgDown), kr("g"), kr("G")} {
		m = pressSeq(m, k)
		if m.corr.detailLog != beforeLog || m.corr.cursor != beforeCorr || m.cursor != beforeCursor {
			t.Fatalf("%s changed detail selection", k)
		}
	}
	if strings.Contains(v, "related failed job:") && !strings.Contains(v, "#7") {
		t.Fatal("related job line chose wrong job")
	}
	m = pressSeq(m, kt(tea.KeyEsc))
	if !strings.Contains(m.View(), "Related to failed job 7") {
		t.Fatal("closing detail did not return to list")
	}
}

func TestCorrSanitization(t *testing.T) {
	m := realCorrModel(t)
	m.entries[1].Message = "ok\x1b[31mRED\x07\r\x00 {\"failed_job_id\":7}"
	m.jobs.failed[0].Class = "App\\Jobs\\X\x1b[31m\x07"
	// Keep the correlation list active even though the changed class no longer
	// describes the fixture; rendering must still sanitize all user strings.
	m.corr.matches = []correlate.Match{{LogIndex: 1}}
	m.corr.cursor = 0
	views := []string{m.View()}
	m = pressSeq(m, kt(tea.KeyEnter))
	views = append(views, m.View())
	m = pressSeq(m, kr("J"))
	views = append(views, m.View())
	m = realCorrModel(t)
	m.jobs.loaded = false
	m = pressSeq(m, kr("J"))
	views = append(views, m.View())
	for _, v := range views {
		clean := ansi.Strip(v)
		if strings.ContainsAny(clean, "\x1b\x07\r\x00") || strings.Contains(clean, "[31m") {
			t.Fatalf("unsafe output %q", clean)
		}
	}
}

func TestCorrResponsive(t *testing.T) {
	for _, width := range []int{40, 60, 80, 120, 200} {
		for _, height := range []int{10, 12, 24, 50} {
			t.Run("size", func(t *testing.T) {
				m := realCorrModel(t)
				for i := 0; i < 10; i++ {
					m.entries = append(m.entries, logs.Entry{Header: logs.Header{Time: time.Now(), Level: "ERROR", Message: "filler"}})
					m.corr.matches = append(m.corr.matches, correlate.Match{LogIndex: len(m.entries) - 1})
				}
				last := len(m.entries) - 1
				m.entries[last].Message = "LAST-MARKER"
				x, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
				m = x.(Model)
				v := m.View()
				lines := strings.Split(v, "\n")
				if len(lines) > height {
					t.Fatalf("%dx%d rendered %d lines", width, height, len(lines))
				}
				for _, line := range lines {
					if lipgloss.Width(line) > width {
						t.Fatalf("line overflows %d: %q", width, line)
					}
				}
				if !strings.Contains(v, "Related to failed job") {
					t.Fatal("list title missing")
				}
				if width >= 60 && !strings.Contains(v, "enter: detail") {
					t.Fatal("hint missing")
				}
				m = pressSeq(m, kr("G"))
				v = m.View()
				if height >= 12 && width >= 80 && !strings.Contains(v, "LAST-MARKER") {
					t.Fatal("last marker not visible")
				}
				m = pressSeq(m, kt(tea.KeyEnter))
				_ = m.View()
			})
		}
	}
	m := realCorrModel(t)
	m.width, m.height = 30, 8
	if !strings.Contains(m.View(), "Terminal too small") {
		t.Fatal("small terminal guard absent")
	}
	m = pressSeq(m, kt(tea.KeyEsc))
	if m.corr.active {
		t.Fatal("esc did not work under small guard")
	}
}
