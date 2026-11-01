package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Align int

const (
	AlignLeft Align = iota
	AlignRight
)

type Column struct {
	Title    string
	Min, Max int
	Flex     bool
	Align    Align
	Priority int
}
type Cell struct {
	Text  string
	Style *lipgloss.Style
	Mark  string
}
type Table struct {
	Cols          []Column
	Rows          [][]Cell
	Cursor, Width int
}

// Render renders a compact, width-aware table. Cell text must already be safe plain text.
func (t Table) Render(height int) (lines []string, start, end int) {
	visible := make([]int, len(t.Cols))
	for i := range visible {
		visible[i] = i
	}
	needed := func() int {
		n := 2 + max(0, len(visible)-1)*2
		for _, i := range visible {
			n += t.Cols[i].Min
		}
		return n
	}
	for t.Width > 0 && needed() > t.Width {
		drop := -1
		for p, i := range visible {
			c := t.Cols[i]
			if !c.Flex && c.Priority > 0 && (drop < 0 || c.Priority > t.Cols[visible[drop]].Priority) {
				drop = p
			}
		}
		if drop < 0 {
			break
		}
		visible = append(visible[:drop], visible[drop+1:]...)
	}
	widths := make([]int, len(t.Cols))
	for _, ci := range visible {
		c := t.Cols[ci]
		w := lipgloss.Width(c.Title)
		for _, row := range t.Rows {
			if ci < len(row) {
				w = max(w, lipgloss.Width(row[ci].Text))
			}
		}
		w = max(w, c.Min)
		if c.Max > 0 {
			w = min(w, c.Max)
		}
		widths[ci] = w
	}
	if t.Width > 0 {
		total := func() int {
			n := 2 + max(0, len(visible)-1)*2
			for _, ci := range visible {
				n += widths[ci]
			}
			return n
		}
		for total() > t.Width && len(visible) > 0 {
			over := total() - t.Width
			flex := -1
			for _, ci := range visible {
				if t.Cols[ci].Flex {
					flex = ci
					break
				}
			}
			if flex >= 0 {
				widths[flex] = max(t.Cols[flex].Min, widths[flex]-over)
				over = total() - t.Width
			}
			for over > 0 {
				best := -1
				for _, ci := range visible {
					c := t.Cols[ci]
					floor := max(1, min(c.Min, 3))
					if c.Flex || widths[ci] <= floor {
						continue
					}
					if best < 0 || widths[ci] > widths[best] || (widths[ci] == widths[best] && c.Priority > t.Cols[best].Priority) {
						best = ci
					}
				}
				if best < 0 {
					break
				}
				widths[best]--
				over--
			}
			if total() <= t.Width {
				break
			}
			drop := -1
			for p, ci := range visible {
				c := t.Cols[ci]
				if c.Priority <= 0 {
					continue
				}
				if drop < 0 || c.Priority > t.Cols[visible[drop]].Priority || (c.Priority == t.Cols[visible[drop]].Priority && !c.Flex && t.Cols[visible[drop]].Flex) {
					drop = p
				}
			}
			if drop < 0 {
				if len(visible) > 1 {
					// Required columns are only required while they can coexist.
					// Keep Flex when possible, then keep the leftmost remaining column.
					for p := len(visible) - 1; p >= 0; p-- {
						if !t.Cols[visible[p]].Flex {
							drop = p
							break
						}
					}
					if drop < 0 {
						drop = len(visible) - 1
					}
					visible = append(visible[:drop], visible[drop+1:]...)
					continue
				}
				available := max(1, t.Width-2-max(0, len(visible)-1)*2)
				for _, ci := range visible {
					widths[ci] = 1
				}
				widths[visible[len(visible)-1]] = max(1, available-(len(visible)-1))
				break
			}
			visible = append(visible[:drop], visible[drop+1:]...)
		}
	}
	cell := func(text string, c Column, w int, style *lipgloss.Style, mark string, selected bool) string {
		text = truncateStrict(text, w)
		if c.Align == AlignRight {
			text = strings.Repeat(" ", max(0, w-lipgloss.Width(text))) + text
		} else {
			text = padRight(text, w)
		}
		if !selected && mark != "" {
			text = highlight(text, mark)
		}
		if style != nil {
			text = style.Render(text)
		}
		return text
	}
	header := make([]string, len(visible))
	for n, ci := range visible {
		header[n] = cell(strings.ToUpper(t.Cols[ci].Title), t.Cols[ci], widths[ci], nil, "", false)
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(colMuted).Bold(true).Render("  "+strings.Join(header, "  ")))
	indices := make([]int, len(t.Rows))
	for i := range indices {
		indices[i] = i
	}
	start, end = window(indices, clampCursor(t.Cursor, len(t.Rows)), height)
	for _, ri := range indices[start:end] {
		selected := ri == t.Cursor
		parts := make([]string, len(visible))
		for n, ci := range visible {
			var x Cell
			if ci < len(t.Rows[ri]) {
				x = t.Rows[ri][ci]
			}
			if selected {
				parts[n] = cell(x.Text, t.Cols[ci], widths[ci], nil, "", true)
			} else {
				parts[n] = cell(x.Text, t.Cols[ci], widths[ci], x.Style, x.Mark, false)
			}
		}
		line := "  " + strings.Join(parts, "  ")
		if selected {
			line = selectedStyle.Render("▸ " + strings.Join(parts, "  "))
		}
		lines = append(lines, line)
	}
	return lines, start, end
}
