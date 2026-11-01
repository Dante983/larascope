package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func lastSGRIsReset(line string) bool {
	all := sgrRe.FindAllString(line, -1)
	if len(all) == 0 {
		return true
	}
	last := all[len(all)-1]
	return last == "\x1b[0m" || last == "\x1b[m"
}

func TestTableAlignmentDroppingAndWindowing(t *testing.T) {
	table := Table{Width: 60, Cursor: 1, Cols: []Column{{Title: "ID", Min: 2, Align: AlignRight}, {Title: "ENV", Min: 3, Priority: 2}, {Title: "MESSAGE", Min: 10, Flex: true}}, Rows: [][]Cell{{{Text: "1"}, {Text: "東京"}, {Text: "a long message"}}, {{Text: "22"}, {Text: "local"}, {Text: "another message"}}}}
	lines, start, end := table.Render(1)
	if start != 1 || end != 2 || !strings.Contains(lines[1], "▸") {
		t.Fatalf("window/marker: %q, %d-%d", lines, start, end)
	}
	for _, line := range lines {
		if lipgloss.Width(line) != lipgloss.Width(lines[0]) {
			t.Fatalf("unaligned line %q", line)
		}
	}
	table.Width = 20
	lines, _, _ = table.Render(2)
	if strings.Contains(lines[0], "ENV") {
		t.Fatal("low-priority column was not dropped")
	}
}

func TestTableStrictBudgetAndSelectedStyledCell(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	for _, width := range []int{20, 30, 40, 60, 80, 120} {
		table := Table{Width: width, Cols: []Column{{Title: "FIRST", Min: 10}, {Title: "SECOND", Min: 10, Priority: 2}, {Title: "THIRD", Min: 10, Priority: 1}}, Rows: [][]Cell{{{Text: "東京東京東京", Style: &style}, {Text: strings.Repeat("long ", 10)}, {Text: "combining e\u0301 😀"}}}}
		lines, _, _ := table.Render(1)
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow: %q", width, line)
			}
		}
	}
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	table := Table{Width: 40, Cursor: 0, Cols: []Column{{Title: "LEVEL", Min: 5}, {Title: "MESSAGE", Min: 10, Flex: true}}, Rows: [][]Cell{{{Text: "ERROR", Style: &style}, {Text: "cell text"}}}}
	lines, _, _ := table.Render(1)
	line := lines[1]
	if !strings.HasPrefix(line, "\x1b[") || strings.Count(line, "▸") != 1 || ansi.Strip(line) == line || ansi.StringWidth(line) != ansi.StringWidth(lines[0]) || strings.Count(line, "\x1b[0m") != 1 {
		t.Fatalf("selected styled row was not one inverse plain row: %q", line)
	}
	if got, _, _ := (Table{Width: 5, Cols: []Column{{Title: "MESSAGE", Min: 10, Flex: true}}, Rows: [][]Cell{{{Text: "abcdef"}}}}).Render(1); ansi.StringWidth(got[0]) > 5 || ansi.StringWidth(got[1]) > 5 {
		t.Fatal("flex minimum overflowed budget")
	}
}

func TestTableUnlimitedAndFlexTruncation(t *testing.T) {
	table := Table{Cols: []Column{{Title: "MESSAGE", Min: 3, Flex: true}}, Rows: [][]Cell{{{Text: "abcdef"}}}}
	lines, _, _ := table.Render(1)
	if !strings.Contains(lines[1], "abcdef") {
		t.Fatal("unlimited table truncated")
	}
	table.Width = 5
	lines, _, _ = table.Render(1)
	if !strings.Contains(lines[1], "…") {
		t.Fatalf("flex was not truncated: %q", lines)
	}
}

func TestTableANSIAndRuneWidthBudget(t *testing.T) {
	for _, width := range []int{20, 30, 40} {
		table := Table{
			Width:  width,
			Cursor: 0,
			Cols:   []Column{{Title: "COLOR", Min: 3}, {Title: "UNICODE", Min: 4}, {Title: "TEXT", Min: 4, Flex: true}},
			Rows:   [][]Cell{{{Text: "\x1b[31mred\x1b[0m"}, {Text: "東京"}, {Text: "e\u0301 🚀"}}},
		}
		lines, _, _ := table.Render(1)
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow: %q", width, line)
			}
			if !lastSGRIsReset(line) {
				t.Fatalf("unterminated SGR at width %d: %q", width, line)
			}
		}
	}

	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	table := Table{Width: 40, Cursor: 0, Cols: []Column{{Title: "CELL", Min: 4, Flex: true}}, Rows: [][]Cell{{{Text: "東京 e\u0301 🚀", Style: &style}}}}
	lines, _, _ := table.Render(1)
	if strings.Count(lines[1], "▸") != 1 || !strings.Contains(ansi.Strip(lines[1]), "▸ 東京 e\u0301 🚀") {
		t.Fatalf("selected styled cell changed text: %q", lines[1])
	}
}
