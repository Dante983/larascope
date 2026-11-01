package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestTruncateAndWindow(t *testing.T) {
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Fatalf("ascii truncate = %q", got)
	}
	if got := truncate("界界", 3); got != "界…" {
		t.Fatalf("wide truncate = %q", got)
	}
	if got := truncate("abc", 0); got != "abc" {
		t.Fatalf("zero truncate = %q", got)
	}
	if got := truncateStrict("abc", 0); got != "" {
		t.Fatalf("strict zero truncate = %q", got)
	}
	if got := truncateStrict("abc", -3); got != "" {
		t.Fatalf("strict negative truncate = %q", got)
	}
	if got := truncateStrict("abc", 3); got != "abc" {
		t.Fatalf("strict fitting truncate = %q", got)
	}
	if got := truncateStrict(styledString(), 4); lipgloss.Width(got) > 4 {
		t.Fatalf("strict ANSI truncate = %q", got)
	}
	styled := "\x1b[31mabcdefgh\x1b[0m"
	got := truncate(styled, 4)
	if lipgloss.Width(got) > 4 || !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("ANSI truncate = %q", got)
	}
	list := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	for _, tc := range []struct{ cursor, start, end int }{{0, 0, 4}, {5, 3, 7}, {9, 6, 10}} {
		s, e := window(list, tc.cursor, 4)
		if s != tc.start || e != tc.end {
			t.Errorf("window(%d) = %d,%d", tc.cursor, s, e)
		}
	}
	for _, h := range []int{0, -1} {
		s, e := window(list, 5, h)
		if e-s != 1 {
			t.Errorf("window h=%d returned %d rows", h, e-s)
		}
	}
	if s, e := window(nil, 0, 0); s != 0 || e != 0 {
		t.Errorf("empty window = %d,%d", s, e)
	}
	m := New()
	if m.contentWidth() != 100 || m.listHeight() != maxLogLines {
		t.Fatal("zero-size defaults changed")
	}
	if m.clipWidth() != 0 {
		t.Fatal("zero width should be unlimited")
	}
	m.width = 80
	if m.clipWidth() != m.contentWidth() {
		t.Fatal("clip width should use content width")
	}
}

func styledString() string { return "\x1b[31mabcdefgh\x1b[0m" }

func TestClipBody(t *testing.T) {
	lines := strings.Join([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, "\n")
	if got := clipBody(lines, 10); got != lines {
		t.Fatalf("fitting body changed: %q", got)
	}
	if got := clipBody(lines, 4); got != "1\n2\n3\n… 7 more lines" {
		t.Fatalf("clipped body = %q", got)
	}
	if got := clipBody(lines, 1); got != "… 10 more lines" {
		t.Fatalf("one-line body = %q", got)
	}
	for _, h := range []int{0, -1} {
		if got := clipBody(lines, h); got != lines {
			t.Fatalf("h=%d changed body: %q", h, got)
		}
	}
}

func TestDemoViewLayoutBounds(t *testing.T) {
	for _, size := range [][2]int{{40, 10}, {80, 24}, {120, 30}} {
		for _, tab := range []Tab{TabLogs, TabJobs, TabConnections, TabOverview} {
			m := NewDemo()
			m.width, m.height, m.active = size[0], size[1], tab
			for _, variant := range []string{"plain", "help"} {
				viewModel := m
				if variant == "help" {
					viewModel.showHelp = true
				}
				lines := strings.Split(viewModel.View(), "\n")
				if len(lines) > size[1] {
					t.Fatalf("%v/%v has %d lines, height %d", size, tab, len(lines), size[1])
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatalf("%v/%v/%s overflows: %q", size, tab, variant, line)
					}
				}
			}
		}
		m := NewDemo()
		m.width, m.height = size[0], size[1]
		m.detail = true
		for _, line := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("logs detail at %v overflows: %q", size, line)
			}
		}
	}
	m := NewDemo()
	m.width, m.height = 40, 10
	lines := strings.Split(m.View(), "\n")
	for _, line := range lines[len(lines)-2:] {
		if ansi.StringWidth(line) > 40 {
			t.Fatalf("demo footer overflows: %q", line)
		}
	}
}
