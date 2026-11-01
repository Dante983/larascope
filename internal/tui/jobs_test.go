package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"larascope/internal/config"
	"larascope/internal/jobs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testUUID = "9b7a44d1-b555-4fa7-9c4a-9a9d8a994333"

func jobModel() Model {
	return Model{active: TabJobs, settings: config.Settings{Root: "/x"}, jobs: jobsState{view: "pending", pending: []jobs.PendingRow{{ID: 1, Class: "Pending"}}, failed: []jobs.FailedRow{{ID: 2, UUID: "uuid", Class: "Failed", Exception: "boom"}}}}
}
func TestJobsControlsAndConfirmation(t *testing.T) {
	if New().jobs.view != "pending" {
		t.Fatal("default view")
	}
	m := jobModel()
	m, _ = updateWithKey(t, m, runeKey("p"))
	if m.jobs.view != "failed" {
		t.Fatal("p")
	}
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.jobs.detail || !strings.Contains(m.View(), "boom") {
		t.Fatal("detail")
	}
	m, _ = updateWithKey(t, m, runeKey("R"))
	if m.jobs.confirm == nil || !strings.Contains(m.jobs.confirm.desc, "retry Failed uuid") {
		t.Fatal("retry confirm")
	}
	m, cmd := updateWithKey(t, m, runeKey("x"))
	if m.jobs.confirm != nil || cmd != nil {
		t.Fatal("cancel")
	}
	m, _ = updateWithKey(t, m, runeKey("A"))
	if m.jobs.confirm == nil || m.jobs.confirm.need != "RETRY ALL" {
		t.Fatal("all")
	}
	m, _ = updateWithKey(t, m, runeKey("y"))
	if m.jobs.confirm == nil {
		t.Fatal("y accepted destructive confirm")
	}
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("RETRY ALL")})
	m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.jobs.confirm != nil {
		t.Fatal("typed confirmation")
	}
}
func TestJobsLoadClampAndRedaction(t *testing.T) {
	m := jobModel()
	m.jobs.pendingCursor = 5
	m.jobs.failedCursor = 5
	m.settings.DBPassword = "s3cr3t"
	updated, _ := m.Update(jobsLoadedMsg{pending: []jobs.PendingRow{{}}, failed: []jobs.FailedRow{{}}, err: errors.New("access s3cr3t")})
	m = updated.(Model)
	if m.jobs.pendingCursor != 0 || m.jobs.failedCursor != 0 || strings.Contains(m.View(), "s3cr3t") {
		t.Fatal("load handling")
	}
	updated, cmd := m.Update(artisanDoneMsg{out: "failed s3cr3t"})
	if cmd == nil || strings.Contains(updated.(Model).jobs.status, "s3cr3t") {
		t.Fatal("artisan status")
	}
}

func jobModelN(nPending, nFailed int) Model {
	m := Model{active: TabJobs, settings: config.Settings{Root: "/x"}, jobs: jobsState{view: "pending"}}
	for i := 0; i < nPending; i++ {
		m.jobs.pending = append(m.jobs.pending, jobs.PendingRow{ID: int64(i), Class: "P" + string(rune('0'+i))})
	}
	for i := 0; i < nFailed; i++ {
		m.jobs.failed = append(m.jobs.failed, jobs.FailedRow{ID: int64(i), UUID: "uuid", Class: "F" + string(rune('0'+i)), Exception: "boom"})
	}
	return m
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			key = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		}
		m, _ = updateWithKey(t, m, key)
	}
	return m
}

func TestJobsNavigationClampAndIndependentCursors(t *testing.T) {
	m := jobModelN(3, 3)
	for range 5 {
		m, _ = updateWithKey(t, m, runeKey("j"))
	}
	if m.jobs.pendingCursor != 2 || !strings.Contains(m.View(), "P2") {
		t.Fatal("pending clamp")
	}
	for range 5 {
		m, _ = updateWithKey(t, m, runeKey("k"))
	}
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyUp})
	for range 2 {
		m, _ = updateWithKey(t, m, runeKey("j"))
	}
	m, _ = updateWithKey(t, m, runeKey("p"))
	m, _ = updateWithKey(t, m, runeKey("j"))
	if m.jobs.failedCursor != 1 || !strings.Contains(m.View(), "F1") {
		t.Fatal("failed cursor")
	}
	m, _ = updateWithKey(t, m, runeKey("p"))
	if m.jobs.pendingCursor != 2 {
		t.Fatal("pending cursor was not retained")
	}
}

func TestJobsNavigationEmptyLists(t *testing.T) {
	m := jobModelN(0, 0)
	for _, k := range []tea.KeyMsg{runeKey("j"), runeKey("k"), {Type: tea.KeyEnter}, runeKey("R"), runeKey("d"), runeKey("A"), runeKey("F"), runeKey("p")} {
		m, _ = updateWithKey(t, m, k)
	}
	if m.jobs.detail || m.jobs.confirm != nil || !strings.Contains(m.View(), "No failed jobs") {
		t.Fatal("empty failed handling")
	}
	m, _ = updateWithKey(t, m, runeKey("p"))
	if !strings.Contains(m.View(), "No pending jobs") {
		t.Fatal("empty pending handling")
	}
}

func TestJobsDetailToggleEscAndViewSwitch(t *testing.T) {
	m := jobModel()
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.jobs.detail || !strings.Contains(m.View(), "class: Pending") {
		t.Fatal("pending detail")
	}
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.jobs.detail {
		t.Fatal("detail toggle")
	}
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.jobs.detail {
		t.Fatal("escape")
	}
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = updateWithKey(t, m, runeKey("p"))
	if m.jobs.detail {
		t.Fatal("switch did not clear detail")
	}
	m.jobs.failed[0].Exception = strings.Join(func() []string {
		a := make([]string, 30)
		for i := range a {
			a[i] = "line" + string(rune('0'+i%10))
		}
		return a
	}(), "\n")
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	v := m.View()
	if !strings.Contains(v, "uuid: uuid") || !strings.Contains(v, "exception:") || !strings.Contains(v, "line9") {
		t.Fatal("failed detail")
	}
	if strings.Count(v, "line") > 20 {
		t.Fatal("exception was not capped")
	}
}

func TestJobsActionsGuarded(t *testing.T) {
	m := jobModel()
	for _, k := range []string{"R", "d", "A", "F"} {
		m, _ = updateWithKey(t, m, runeKey(k))
		if m.jobs.confirm != nil {
			t.Fatalf("%s confirmed pending", k)
		}
	}
	m, _ = updateWithKey(t, m, runeKey("p"))
	m.settings.Root = ""
	for _, k := range []string{"R", "d", "A", "F"} {
		m, _ = updateWithKey(t, m, runeKey(k))
		if m.jobs.confirm != nil {
			t.Fatalf("%s confirmed without root", k)
		}
	}
	if !strings.Contains(m.View(), "No jobs detected yet") {
		t.Fatal("missing no jobs message")
	}
}

func TestJobsSingleConfirmFlow(t *testing.T) {
	m := jobModel()
	m, _ = updateWithKey(t, m, runeKey("p"))
	m, _ = updateWithKey(t, m, runeKey("R"))
	if m.jobs.confirm == nil || m.jobs.confirm.desc != "retry Failed uuid" || m.jobs.confirm.need != "" {
		t.Fatal("retry confirmation")
	}
	m, cmd := updateWithKey(t, m, runeKey("y"))
	if cmd == nil || m.jobs.confirm != nil {
		t.Fatal("yes")
	}
	m, _ = updateWithKey(t, m, runeKey("R"))
	m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || m.jobs.confirm != nil {
		t.Fatal("escape cancel")
	}
	m, _ = updateWithKey(t, m, runeKey("R"))
	m, cmd = updateWithKey(t, m, runeKey("q"))
	if cmd != nil || m.jobs.confirm != nil {
		t.Fatal("q cancel")
	}
	m, _ = updateWithKey(t, m, runeKey("R"))
	m, cmd = updateWithKey(t, m, runeKey("n"))
	if cmd != nil || m.jobs.confirm != nil {
		t.Fatal("other cancel")
	}
	m, _ = updateWithKey(t, m, runeKey("R"))
	m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl-c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl-c did not quit")
	}
	m.jobs.confirm = nil
	m, _ = updateWithKey(t, m, runeKey("d"))
	if !strings.HasPrefix(m.jobs.confirm.desc, "forget Failed uuid") {
		t.Fatal("forget")
	}
	m.jobs.confirm = nil
	m.jobs.failed[0].UUID = ""
	m, _ = updateWithKey(t, m, runeKey("R"))
	if !strings.Contains(m.jobs.confirm.desc, " 2") {
		t.Fatal("numeric fallback")
	}
	active := m.active
	m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.active != active || m.jobs.confirm != nil {
		t.Fatal("tab confirmation handling")
	}
}

func TestJobsTypedConfirmFlow(t *testing.T) {
	for _, tc := range []struct{ key, need, desc string }{{"A", "RETRY ALL", "retry all 3 failed jobs"}, {"F", "FLUSH", "permanently deletes 3 failed jobs"}} {
		t.Run(tc.key, func(t *testing.T) {
			m := jobModelN(0, 3)
			m, _ = updateWithKey(t, m, runeKey("p"))
			m, _ = updateWithKey(t, m, runeKey(tc.key))
			if m.jobs.confirm == nil || !strings.Contains(m.jobs.confirm.desc, tc.desc) {
				t.Fatal("open")
			}
			m, _ = updateWithKey(t, m, runeKey("y"))
			if m.jobs.confirm.typed != "" {
				t.Fatal("y typed")
			}
			wrong := "FLUS"
			if tc.need == "RETRY ALL" {
				wrong = "RETRY AL"
			}
			m = typeText(t, m, wrong)
			if m.jobs.confirm.typed != wrong {
				t.Fatalf("typed = %q, want %q", m.jobs.confirm.typed, wrong)
			}
			m, cmd := updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil || m.jobs.confirm == nil || !strings.Contains(m.jobs.status, "type "+tc.need+" to confirm") {
				t.Fatal("wrong confirmation")
			}
			m = typeText(t, m, "X")
			m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
			m = typeText(t, m, tc.need[len(wrong):])
			m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil || m.jobs.confirm != nil {
				t.Fatal("typed confirmation")
			}
			m, _ = updateWithKey(t, m, runeKey(tc.key))
			if !strings.Contains(m.View(), "type "+tc.need+" then enter") {
				t.Fatal("footer")
			}
			m, _ = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
			if strings.Contains(m.View(), "type "+tc.need+" then enter") {
				t.Fatal("footer remained")
			}
			m, _ = updateWithKey(t, m, runeKey(tc.key))
			m, cmd = updateWithKey(t, m, runeKey("q"))
			if cmd != nil || m.jobs.confirm != nil {
				t.Fatal("q cancel")
			}
			m, _ = updateWithKey(t, m, runeKey(tc.key))
			m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
			if cmd == nil {
				t.Fatal("ctrl-c")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("ctrl-c did not quit")
			}
		})
	}
}

func TestJobsHelpOverlayIgnoresJobKeys(t *testing.T) {
	m := jobModelN(0, 2)
	m.jobs.view = "failed"
	m.showHelp = true
	m.jobs.failedCursor = 0
	for _, k := range []string{"R", "j", "p"} {
		m, _ = updateWithKey(t, m, runeKey(k))
	}
	if m.jobs.confirm != nil || m.jobs.view != "failed" || m.jobs.failedCursor != 0 {
		t.Fatal("help intercepted job key")
	}
}

func TestJobsConfirmRunsArtisan(t *testing.T) {
	for _, tc := range []struct{ key, typed, want string }{{"R", "y", "artisan queue:retry " + testUUID}, {"d", "y", "artisan queue:forget " + testUUID}, {"A", "RETRY ALL", "artisan queue:retry all"}, {"F", "FLUSH", "artisan queue:flush"}} {
		t.Run(tc.key, func(t *testing.T) {
			args, script := fakeArtisanPHP(t)
			t.Setenv("LARASCOPE_PHP", script)
			m := jobModelN(0, 1)
			m.jobs.failed[0].UUID = testUUID
			m.settings.Root = t.TempDir()
			m, _ = updateWithKey(t, m, runeKey("p"))
			m, _ = updateWithKey(t, m, runeKey(tc.key))
			var cmd tea.Cmd
			if tc.typed == "y" {
				m, cmd = updateWithKey(t, m, runeKey("y"))
			} else {
				m = typeText(t, m, tc.typed)
				m, cmd = updateWithKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			}
			if cmd == nil {
				t.Fatal("nil artisan command")
			}
			msg := cmd()
			done, ok := msg.(artisanDoneMsg)
			if !ok {
				t.Fatal("not artisan done")
			}
			if done.err != nil || !strings.Contains(done.out, "fake artisan ok") {
				t.Fatalf("artisan result = %#v", done)
			}
			updated, reload := m.Update(msg)
			if reload == nil || !strings.Contains(updated.(Model).jobs.status, "fake artisan ok") {
				t.Fatal("completion")
			}
			if last := lastArtisanArgs(t, args); last != tc.want {
				t.Fatalf("args = %q, want %q", last, tc.want)
			}
		})
	}
}

func TestJobsConfirmCapturesTarget(t *testing.T) {
	args, script := fakeArtisanPHP(t)
	t.Setenv("LARASCOPE_PHP", script)
	m := jobModelN(0, 1)
	m.jobs.failed[0].UUID = testUUID
	m.settings.Root = t.TempDir()
	m, _ = updateWithKey(t, m, runeKey("p"))
	m, _ = updateWithKey(t, m, runeKey("R"))
	m.jobs.failed[0].UUID = "11111111-1111-1111-1111-111111111111"
	m.jobs.failed = append(m.jobs.failed, jobs.FailedRow{UUID: "22222222-2222-2222-2222-222222222222"})
	m.jobs.failedCursor = 1
	m, cmd := updateWithKey(t, m, runeKey("y"))
	if cmd == nil {
		t.Fatal("nil artisan command")
	}
	if done := cmd().(artisanDoneMsg); done.err != nil {
		t.Fatal(done.err)
	}
	if last := lastArtisanArgs(t, args); !strings.HasSuffix(last, testUUID) {
		t.Fatalf("args = %q, want target %q", last, testUUID)
	}
}

func TestJobsConfirmRejectsInvalidID(t *testing.T) {
	args, script := fakeArtisanPHP(t)
	t.Setenv("LARASCOPE_PHP", script)
	m := jobModelN(0, 1)
	m.jobs.failed[0].UUID = "--force"
	m.jobs.failed[0].ID = 0
	m.settings.Root = t.TempDir()
	m, _ = updateWithKey(t, m, runeKey("p"))
	m, _ = updateWithKey(t, m, runeKey("R"))
	m, cmd := updateWithKey(t, m, runeKey("y"))
	if cmd == nil {
		t.Fatal("nil artisan command")
	}
	done, ok := cmd().(artisanDoneMsg)
	if !ok || done.err == nil || !strings.Contains(done.err.Error(), "invalid job id") {
		t.Fatalf("artisan result = %#v", done)
	}
	if _, err := os.Stat(args); !os.IsNotExist(err) {
		t.Fatalf("invalid ID executed PHP: stat error = %v", err)
	}
}

func fakeArtisanPHP(t *testing.T) (args, script string) {
	t.Helper()
	args = filepath.Join(t.TempDir(), "args")
	script = filepath.Join(t.TempDir(), "php")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" >> "+args+"\necho \"fake artisan ok\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return args, script
}

func lastArtisanArgs(t *testing.T, args string) string {
	t.Helper()
	b, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

func TestJobsLoadedClampWhenShrinks(t *testing.T) {
	m := jobModelN(3, 3)
	m.jobs.pendingCursor, m.jobs.failedCursor = 2, 2
	updated, _ := m.Update(jobsLoadedMsg{pending: []jobs.PendingRow{{}}, failed: []jobs.FailedRow{{}}})
	m = updated.(Model)
	if m.jobs.pendingCursor != 0 || m.jobs.failedCursor != 0 {
		t.Fatal("shrink clamp")
	}
	updated, _ = m.Update(jobsLoadedMsg{})
	m = updated.(Model)
	m.jobs.detail = true
	_ = m.View()
	if m.jobs.pendingCursor != 0 || m.jobs.failedCursor != 0 {
		t.Fatal("empty clamp")
	}
}

func TestJobsDriverErrors(t *testing.T) {
	pg := config.Settings{DBConnection: "pgsql", DBDatabase: "app"}
	if msg := loadJobsCmd(pg)().(jobsLoadedMsg); msg.err == nil || strings.Contains(msg.err.Error(), "unsupported") {
		t.Fatalf("pgsql message=%#v", msg)
	}
	empty := config.Settings{DBConnection: "pgsql"}
	if msg := loadJobsCmd(empty)().(jobsLoadedMsg); msg.err == nil || msg.err.Error() != "DB_DATABASE not set" {
		t.Fatalf("empty pg message=%#v", msg)
	}
	m := Model{settings: config.Settings{Root: "/x", DBConnection: "sqlite"}, jobs: jobsState{err: errors.New("unsupported")}}
	if got := m.jobsBody(); !strings.Contains(got, "Jobs supports MySQL/MariaDB/PostgreSQL only") {
		t.Fatal(got)
	}
	m = Model{settings: config.Settings{Root: "/x", DBConnection: "pgsql", DBPassword: "secret"}, jobs: jobsState{err: errors.New("access secret")}}
	if strings.Contains(m.jobsBody(), "secret") {
		t.Fatal("password displayed")
	}
}
