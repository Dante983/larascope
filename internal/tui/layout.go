package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

// truncate treats w <= 0 as unlimited; use truncateStrict for computed budgets.
func truncate(s string, w int) string {
	if w <= 0 || ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// truncateStrict treats w <= 0 as no available space.
func truncateStrict(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return truncate(s, w)
}
func padRight(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
func (m Model) contentWidth() int {
	if m.width == 0 {
		return 100
	}
	return max(20, m.width-2)
}

// clipWidth is the width for truncation/wrapping; 0 means unlimited (m.width == 0).
func (m Model) clipWidth() int {
	if m.width == 0 {
		return 0
	}
	return m.contentWidth()
}
func (m Model) listHeight() int {
	if m.height == 0 {
		return maxLogLines
	}
	overhead := 1
	if m.searching {
		overhead++
	}
	return max(1, m.bodyHeight()-overhead)
}
func (m Model) bodyHeight() int {
	if m.height == 0 {
		return 1 << 30
	}
	return max(1, m.height-4)
}
func (m Model) twoPane() bool { return m.width >= 120 }
func window(list []int, cursor, h int) (int, int) {
	if len(list) == 0 {
		return 0, 0
	}
	if h < 1 {
		h = 1
	}
	if len(list) <= h {
		return 0, len(list)
	}
	pos := 0
	for i, x := range list {
		if x == cursor {
			pos = i
			break
		}
	}
	start := max(0, min(pos-h/2, len(list)-h))
	return start, min(len(list), start+h)
}

func clipLines(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = truncate(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

// clipBody keeps at most h lines and reports any omitted lines.
func clipBody(s string, h int) string {
	if h <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= h {
		return s
	}
	if h == 1 {
		return fmt.Sprintf("… %d more lines", len(lines))
	}
	return strings.Join(append(lines[:h-1], fmt.Sprintf("… %d more lines", len(lines)-(h-1))), "\n")
}

func wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Hardwrap(s, w, true)
}
