package tui

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"larascope/internal/conns"
	"larascope/internal/jobs"
	"larascope/internal/logs"
)

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case " ", "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func press(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(keyMsg(key))
	return updated.(Model), cmd
}

func TestPressKeyTypes(t *testing.T) {
	for _, tc := range []struct {
		key     string
		keyType tea.KeyType
		text    string
	}{
		{"enter", tea.KeyEnter, "enter"},
		{"esc", tea.KeyEsc, "esc"},
		{"tab", tea.KeyTab, "tab"},
		{"shift+tab", tea.KeyShiftTab, "shift+tab"},
		{"space", tea.KeySpace, " "},
	} {
		msg := keyMsg(tc.key)
		if msg.Type != tc.keyType || msg.String() != tc.text {
			t.Errorf("keyMsg(%q) = type %v, string %q; want type %v, string %q", tc.key, msg.Type, msg.String(), tc.keyType, tc.text)
		}
	}
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case msg := <-result:
		if _, ok := msg.(tea.BatchMsg); ok {
			t.Fatalf("demo command returned tea.BatchMsg")
		}
		return msg
	case <-time.After(time.Second):
		t.Fatal("demo command did not finish within one second")
		return nil
	}
}

func strippedView(m Model) string { return ansi.Strip(m.View()) }

// isolatePath makes any exec of php/artisan fail and proves it by asserting php is unresolvable.
func isolatePath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	if p, err := exec.LookPath("php"); err == nil {
		t.Fatalf("php resolvable in isolated PATH: %s", p)
	}
}

// walkKeys presses keys in order on m, running any returned cmd and feeding its msg back.
// Fails if a cmd yields anything but artisanDoneMsg containing "demo:", or Update returns a follow-up cmd,
// or status contains "executable file not found". Returns the final model.
func walkKeys(t *testing.T, m Model, keys []string) Model {
	t.Helper()
	for _, key := range keys {
		var cmd tea.Cmd
		m, cmd = press(t, m, key)
		if cmd == nil {
			continue
		}
		msg := runCmd(t, cmd)
		done, ok := msg.(artisanDoneMsg)
		if !ok || !strings.Contains(done.out, "demo:") {
			t.Fatalf("key %s produced %#v", key, msg)
		}
		updated, next := m.Update(msg)
		m = updated.(Model)
		if next != nil || strings.Contains(m.jobs.status, "executable file not found") {
			t.Fatalf("key %s performed live work: %#v, %q", key, next, m.jobs.status)
		}
	}
	return m
}

// demoTypeText types s one key at a time through press (real key types).
func demoTypeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		key := string(r)
		if r == ' ' {
			key = " "
		}
		var cmd tea.Cmd
		m, cmd = press(t, m, key)
		if cmd != nil {
			t.Fatalf("typing %q returned command", key)
		}
	}
	return m
}

// walkDemo drives navigation then each action with real acceptance and real cancellation.
func walkDemo(t *testing.T, m Model) Model {
	t.Helper()
	m = walkKeys(t, m, []string{"r", "f", "1", "2", "3", "tab", "shift+tab", "2"})
	m.active = TabJobs
	if m.jobs.view != "failed" {
		m = walkKeys(t, m, []string{"p"})
	}
	for _, k := range []string{"R", "d"} {
		m = walkKeys(t, m, []string{k, "esc"})
		if m.jobs.confirm != nil {
			t.Fatalf("%s esc did not cancel", k)
		}
		m = walkKeys(t, m, []string{k})
		if m.jobs.confirm == nil || m.jobs.confirm.need != "" {
			t.Fatalf("%s did not open single confirm", k)
		}
		m = walkKeys(t, m, []string{"y"})
		if m.jobs.confirm != nil || !strings.Contains(m.jobs.status, "demo") {
			t.Fatalf("%s y not accepted: %q", k, m.jobs.status)
		}
	}
	for _, tc := range []struct{ key, need string }{{"A", "RETRY ALL"}, {"F", "FLUSH"}} {
		m = walkKeys(t, m, []string{tc.key, "esc"})
		if m.jobs.confirm != nil {
			t.Fatalf("%s esc did not cancel", tc.key)
		}
		m = walkKeys(t, m, []string{tc.key})
		if m.jobs.confirm == nil || m.jobs.confirm.need != tc.need {
			t.Fatalf("%s confirm = %#v", tc.key, m.jobs.confirm)
		}
		m = demoTypeText(t, m, tc.need)
		m = walkKeys(t, m, []string{"enter"})
		if m.jobs.confirm != nil || !strings.Contains(m.jobs.status, "demo") {
			t.Fatalf("%s typed not accepted: %q", tc.key, m.jobs.status)
		}
	}
	return m
}

func assertDemoUnchanged(t *testing.T, name string, m Model, logsB []logs.Entry, pendB []jobs.PendingRow, countsB []jobs.QueueCount, failedB []jobs.FailedRow, connsB []conns.Result) {
	t.Helper()
	if !reflect.DeepEqual(m.entries, logsB) || !reflect.DeepEqual(m.jobs.pending, pendB) || !reflect.DeepEqual(m.jobs.counts, countsB) || !reflect.DeepEqual(m.jobs.failed, failedB) || !reflect.DeepEqual(m.conns.results, connsB) {
		t.Fatalf("%s mutated demo fixture data", name)
	}
}

func TestDemoViewsAndSafeRefresh(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 10}} {
		m := NewDemo()
		m.width, m.height = size[0], size[1]
		for _, tab := range []Tab{TabLogs, TabJobs, TabConnections} {
			m.active = tab
			view := m.View()
			if !strings.Contains(view, "DEMO") {
				t.Fatalf("demo label missing at %v", size)
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("wide demo line: %q", line)
				}
			}
		}
	}
	m := NewDemo()
	for _, key := range []string{"r", "f", "2", "3"} {
		var cmd tea.Cmd
		m, cmd = press(t, m, key)
		if cmd != nil {
			t.Fatalf("demo %q returned live command", key)
		}
	}
}

func TestDemoJobsAreLeftAlignedAndRefreshNoticeIsGlobal(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 10}} {
		for _, jobView := range []string{"pending", "failed"} {
			m := NewDemo()
			m.width, m.height, m.active, m.jobs.view = size[0], size[1], TabJobs, jobView
			line := strings.Split(m.View(), "\n")[2]
			if !strings.HasPrefix(line, "Jobs [") || strings.HasPrefix(line, " ") {
				t.Fatalf("jobs body centered at %v/%s: %q", size, jobView, line)
			}
		}
	}
	for _, tab := range []Tab{TabLogs, TabJobs, TabConnections} {
		m := NewDemo()
		m.active = tab
		for _, key := range []string{"r", "f"} {
			updated, cmd := m.Update(runeKey(key))
			m = updated.(Model)
			if cmd != nil || !strings.Contains(m.View(), "refresh disabled") {
				t.Fatalf("%v/%s did not show demo refresh notice", tab, key)
			}
		}
	}
}

func TestDemoFixturesHaveNoSecrets(t *testing.T) {
	for _, e := range demoLogs() {
		for _, value := range append([]string{e.Message}, e.Trace...) {
			s := strings.ToLower(value)
			if strings.Contains(s, "password") || strings.Contains(s, "secret") || strings.Contains(s, "token") || strings.Contains(s, "key=") {
				t.Fatal("secret-looking demo fixture")
			}
		}
	}
	pending, _, failed := demoJobs()
	for _, row := range pending {
		if strings.Contains(strings.ToLower(row.Queue+row.Class), "secret") {
			t.Fatal("unsafe job fixture")
		}
	}
	for _, row := range failed {
		if strings.Contains(strings.ToLower(row.Exception), "secret") {
			t.Fatal("unsafe failed fixture")
		}
	}
	for _, row := range demoConns() {
		if strings.Contains(strings.ToLower(row.Name+row.Target+row.Err), "secret") {
			t.Fatal("unsafe connection fixture")
		}
	}
	groups := logs.GroupEntries(demoLogs())
	if len(groups) != 11 || groups[0].Count != 10 {
		t.Fatalf("groups = %d, top = %d", len(groups), groups[0].Count)
	}
	total := 0
	for _, group := range groups {
		total += group.Count
	}
	if total != len(demoLogs()) {
		t.Fatalf("grouped total %d, logs %d", total, len(demoLogs()))
	}
}

func TestDemoFooterHints(t *testing.T) {
	m := NewDemo()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = updated.(Model)

	lines := strings.Split(strippedView(m), "\n")
	footer := strings.Join(lines[len(lines)-2:], "\n")
	if strings.Contains(footer, "follow") || strings.Contains(footer, "f:") || !strings.Contains(footer, "search") || !strings.Contains(footer, "group") {
		t.Fatalf("demo logs footer = %q", footer)
	}

	m.active = TabConnections
	footer = strings.Join(strings.Split(strippedView(m), "\n")[len(strings.Split(strippedView(m), "\n"))-2:], "\n")
	if strings.Contains(footer, "refresh") {
		t.Fatalf("demo connections footer = %q", footer)
	}

	m.active = TabLogs
	m, _ = press(t, m, "?")
	help := strippedView(m)
	for _, unwanted := range []string{"f: follow", "r: refresh", "r: reload log file"} {
		if strings.Contains(help, unwanted) {
			t.Fatalf("demo help contains %q: %q", unwanted, help)
		}
	}
	if !strings.Contains(help, "disabled in demo") {
		t.Fatalf("demo help lacks disabled notice: %q", help)
	}

	live := New().WithLogs(demoLogs(), nil)
	live.width, live.height = 120, 30
	liveFooter := strings.Join(strings.Split(ansi.Strip(live.View()), "\n")[len(strings.Split(ansi.Strip(live.View()), "\n"))-2:], "\n")
	if !strings.Contains(liveFooter, "f: follow") {
		t.Fatalf("live logs footer = %q", liveFooter)
	}
}

func TestDemoNoConfirmOutsideJobs(t *testing.T) {
	isolatePath(t)
	for _, tc := range []struct {
		name  string
		setup func(Model) Model
	}{
		{"logs", func(m Model) Model { return m }},
		{"connections", func(m Model) Model { m.active = TabConnections; return m }},
		{"jobs pending", func(m Model) Model { m.active = TabJobs; return m }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"R", "d", "A", "F"} {
				m, cmd := press(t, tc.setup(NewDemo()), key)
				if m.jobs.confirm != nil || cmd != nil {
					t.Fatalf("%s on %s opened confirmation or command", key, tc.name)
				}
			}
		})
	}
	m := NewDemo()
	m, _ = press(t, m, "enter")
	var cmd tea.Cmd
	for _, key := range []string{"R", "F"} {
		m, cmd = press(t, m, key)
		if m.jobs.confirm != nil || cmd != nil {
			t.Fatalf("%s opened confirmation in logs detail", key)
		}
	}
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "/")
	m, cmd = press(t, m, "R")
	if m.jobs.confirm != nil || cmd != nil || m.searchInput != "R" {
		t.Fatal("R did not remain a search character")
	}
	m, cmd = press(t, m, "F")
	if m.jobs.confirm != nil || cmd != nil || m.searchInput != "RF" {
		t.Fatal("F did not remain a search character")
	}
}

func confirmDemoAction(t *testing.T, key string) Model {
	t.Helper()
	isolatePath(t)
	m := NewDemo()
	m.active = TabJobs
	return confirmDemoActionOn(t, m, key)
}

func confirmDemoActionOn(t *testing.T, m Model, key string) Model {
	t.Helper()
	isolatePath(t)
	if m.jobs.view != "failed" {
		m, _ = press(t, m, "p")
	}
	m, _ = press(t, m, key)
	if m.jobs.confirm == nil || !strings.Contains(m.jobs.confirm.desc, "[demo]") {
		t.Fatalf("%s did not open demo confirmation", key)
	}
	if m.jobs.confirm.need == "" {
		var cmd tea.Cmd
		m, cmd = press(t, m, "y")
		if cmd == nil {
			t.Fatalf("%s confirmation returned nil command", key)
		}
		msg := runCmd(t, cmd)
		done, ok := msg.(artisanDoneMsg)
		if !ok || !strings.Contains(done.out, "demo:") {
			t.Fatalf("%s result = %#v", key, msg)
		}
		updated, follow := m.Update(msg)
		m = updated.(Model)
		if follow != nil || !strings.Contains(m.jobs.status, "demo") {
			t.Fatalf("%s completion = %#v, %q", key, follow, m.jobs.status)
		}
		return m
	}
	m = demoTypeText(t, m, m.jobs.confirm.need)
	var cmd tea.Cmd
	m, cmd = press(t, m, "enter")
	if cmd == nil {
		t.Fatalf("%s typed confirmation returned nil command", key)
	}
	msg := runCmd(t, cmd)
	done, ok := msg.(artisanDoneMsg)
	if !ok || !strings.Contains(done.out, "demo:") {
		t.Fatalf("%s result = %#v", key, msg)
	}
	updated, follow := m.Update(msg)
	m = updated.(Model)
	if follow != nil || !strings.Contains(m.jobs.status, "demo") {
		t.Fatalf("%s completion = %#v, %q", key, follow, m.jobs.status)
	}
	return m
}

func TestDemoDestructiveFlows(t *testing.T) {
	isolatePath(t)
	for _, key := range []string{"R", "d"} {
		m := NewDemo()
		m.active = TabJobs
		m, _ = press(t, m, "p")
		m, _ = press(t, m, key)
		if m.jobs.confirm == nil || m.jobs.confirm.need != "" || !strings.Contains(m.jobs.confirm.desc, "[demo]") {
			t.Fatalf("%s confirmation = %#v", key, m.jobs.confirm)
		}
		var cmd tea.Cmd
		m, cmd = press(t, m, "x")
		if m.jobs.confirm != nil || cmd != nil {
			t.Fatalf("%s cancel failed", key)
		}
		m, _ = press(t, m, key)
		m, cmd = press(t, m, "esc")
		if m.jobs.confirm != nil || cmd != nil {
			t.Fatalf("%s escape cancel failed", key)
		}
		_ = confirmDemoAction(t, key)
	}
	for _, tc := range []struct{ key, need string }{{"A", "RETRY ALL"}, {"F", "FLUSH"}} {
		m := NewDemo()
		m.active = TabJobs
		m, _ = press(t, m, "p")
		m, _ = press(t, m, tc.key)
		m = demoTypeText(t, m, "wrong")
		var cmd tea.Cmd
		m, cmd = press(t, m, "enter")
		if m.jobs.confirm == nil || cmd != nil || !strings.Contains(m.jobs.status, "type "+tc.need+" to confirm") {
			t.Fatalf("%s accepted wrong confirmation", tc.key)
		}
		m, cmd = press(t, m, "esc")
		if m.jobs.confirm != nil || cmd != nil {
			t.Fatalf("%s escape failed", tc.key)
		}
		_ = confirmDemoAction(t, tc.key)
	}

	for _, tab := range []Tab{TabLogs, TabJobs, TabConnections} {
		m := NewDemo()
		m.active = tab
		m = walkDemo(t, m)
	}
}

func TestDemoFixturesImmutable(t *testing.T) {
	isolatePath(t)
	if !reflect.DeepEqual(demoLogs(), demoLogs()) {
		t.Fatal("demo fixtures are not deterministic")
	}
	logsBefore := demoLogs()
	pendingBefore, countsBefore, failedBefore := demoJobs()
	connsBefore := demoConns()
	var models []Model
	for _, tab := range []Tab{TabLogs, TabJobs, TabConnections} {
		m := NewDemo()
		m.active = tab
		assertDemoUnchanged(t, fmt.Sprintf("%v baseline", tab), m, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
		m = walkDemo(t, m)
		assertDemoUnchanged(t, fmt.Sprintf("%v walk", tab), m, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
		models = append(models, m)
	}
	m := NewDemo()
	m.active = TabJobs
	m, _ = press(t, m, "p")
	assertDemoUnchanged(t, "confirmation baseline", m, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
	mR := confirmDemoActionOn(t, m, "R")
	assertDemoUnchanged(t, "R confirmation", mR, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
	models = append(models, mR)
	md := confirmDemoActionOn(t, mR, "d")
	assertDemoUnchanged(t, "d confirmation", md, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
	models = append(models, md)
	mA := confirmDemoActionOn(t, md, "A")
	assertDemoUnchanged(t, "A confirmation", mA, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
	models = append(models, mA)
	mF := confirmDemoActionOn(t, mA, "F")
	assertDemoUnchanged(t, "F confirmation", mF, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
	models = append(models, mF)
	for i, m := range models {
		assertDemoUnchanged(t, fmt.Sprintf("model %d", i), m, logsBefore, pendingBefore, countsBefore, failedBefore, connsBefore)
	}
}

func TestDemoFixturesCoherent(t *testing.T) {
	pending, counts, failed := demoJobs()
	ready, delayed, reserved := 0, 0, 0
	queues := map[string]bool{}
	for _, row := range pending {
		queues[row.Queue] = true
		switch row.State {
		case "ready":
			ready++
		case "delayed":
			delayed++
		case "reserved":
			reserved++
		}
	}
	queueRows := 0
	for _, count := range counts {
		queueRows += count.Ready + count.Delayed + count.Reserved
	}
	if queueRows != len(pending) {
		t.Fatalf("queue counts = %d, pending = %d", queueRows, len(pending))
	}
	m := NewDemo()
	m.width, m.height, m.active = 120, 30, TabJobs
	jobsView := ansi.Strip(m.View())
	if !strings.Contains(jobsView, fmt.Sprintf("totals ready=%d delayed=%d reserved=%d  failed: %d", ready, delayed, reserved, len(failed))) {
		t.Fatalf("jobs totals missing: %q", jobsView)
	}
	m, _ = press(t, m, "p")
	if !strings.Contains(ansi.Strip(m.View()), fmt.Sprintf("of %d", len(failed))) {
		t.Fatal("failed count missing")
	}
	for _, row := range failed {
		if !queues[row.Queue] {
			t.Fatalf("failed queue %q is not in pending queues", row.Queue)
		}
	}

	entries := demoLogs()
	errors, warns := 0, 0
	for _, entry := range entries {
		if entry.Level == "ERROR" {
			errors++
		}
		if entry.Level == "WARNING" {
			warns++
		}
	}
	m = NewDemo()
	m.width, m.height = 120, 30
	if !strings.Contains(ansi.Strip(m.View()), fmt.Sprintf("%d entries · %d error · %d warn", len(entries), errors, warns)) {
		t.Fatal("log metadata missing")
	}
	up, down := 0, 0
	for _, conn := range demoConns() {
		if conn.OK {
			up++
		} else {
			down++
		}
		if conn.Name == "" {
			t.Fatal("empty connection name")
		}
	}
	m.active = TabConnections
	if !strings.Contains(ansi.Strip(m.View()), fmt.Sprintf("%d up · %d down", up, down)) {
		t.Fatal("connection metadata missing")
	}
}
