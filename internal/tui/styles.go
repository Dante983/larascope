package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colAccent = lipgloss.AdaptiveColor{Light: "125", Dark: "205"}
	colMuted  = lipgloss.AdaptiveColor{Light: "243", Dark: "241"}
	colOK     = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}
	colWarn   = lipgloss.AdaptiveColor{Light: "130", Dark: "214"}
	colErr    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
)

var selectedStyle = lipgloss.NewStyle().Reverse(true)

func levelStyle(level string) lipgloss.Style {
	switch strings.ToUpper(level) {
	case "ERROR", "CRITICAL", "ALERT", "EMERGENCY":
		return lipgloss.NewStyle().Foreground(colErr).Bold(true)
	case "WARNING":
		return lipgloss.NewStyle().Foreground(colWarn)
	case "NOTICE", "INFO":
		return lipgloss.NewStyle()
	case "DEBUG":
		return lipgloss.NewStyle().Foreground(colMuted)
	default:
		return lipgloss.NewStyle()
	}
}
func statusDot(ok bool) (string, lipgloss.Style) {
	if ok {
		return "● up", lipgloss.NewStyle().Foreground(colOK)
	}
	return "○ down", lipgloss.NewStyle().Foreground(colErr)
}
func metaLine(left, right string, w int) string {
	if w == 0 && right != "" {
		return lipgloss.NewStyle().Foreground(colMuted).Render(left + "  " + right)
	}
	if w > 0 && lipgloss.Width(left)+lipgloss.Width(right)+2 <= w {
		return lipgloss.NewStyle().Foreground(colMuted).Render(left + strings.Repeat(" ", w-lipgloss.Width(left)-lipgloss.Width(right)) + right)
	}
	return lipgloss.NewStyle().Foreground(colMuted).Render(left)
}
func badge(text string, c lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(" " + text + " ")
}
func keyHint(key, label string) string {
	return lipgloss.NewStyle().Bold(true).Render(key) + lipgloss.NewStyle().Foreground(colMuted).Render(": "+label)
}
func tabBar(active Tab, width int) string {
	if width == 0 {
		width = 40
	}
	if width < 20 {
		width = 20
	}
	parts := make([]string, len(tabNames))
	for i, name := range tabNames {
		t := fmtTab(i+1, name)
		if Tab(i) == active {
			parts[i] = lipgloss.NewStyle().Foreground(colAccent).Bold(true).Underline(true).Render(t)
		} else {
			parts[i] = lipgloss.NewStyle().Foreground(colMuted).Render(t)
		}
	}
	return strings.Join(parts, "  ") + "\n" + strings.Repeat("─", width)
}
func fmtTab(n int, name string) string { return string(rune('0'+n)) + " " + name }
func highlight(s, q string) string {
	if q == "" {
		return s
	}
	rs, qr := []rune(s), []rune(q)
	if len(qr) == 0 || len(qr) > len(rs) {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(rs); {
		if i+len(qr) <= len(rs) && strings.EqualFold(string(rs[i:i+len(qr)]), q) {
			out.WriteString(lipgloss.NewStyle().Foreground(colAccent).Underline(true).Render(string(rs[i : i+len(qr)])))
			i += len(qr)
			continue
		}
		out.WriteRune(rs[i])
		i++
	}
	return out.String()
}
