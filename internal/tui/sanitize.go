package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// sanitize removes terminal control sequences and control characters from text
// originating outside the UI. Newlines are retained for detail views.
func sanitize(s string) string {
	s = ansi.Strip(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString("    ")
		case r == '\n':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			// Includes carriage returns and any unrecognised escape bytes.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sanitizeLine(s string) string { return strings.ReplaceAll(sanitize(s), "\n", " ") }
