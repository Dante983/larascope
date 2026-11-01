package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLevelStyleAndHighlight(t *testing.T) {
	for _, level := range []string{"ERROR", "critical", "WARNING", "INFO", "DEBUG", "unknown"} {
		_ = levelStyle(level).Render(level)
	}
	// These must treat case and regexp metacharacters literally; rendering may
	// omit ANSI sequences under tests, so this is primarily a no-panic check.
	_ = highlight("Hello hello", "HEL")
	_ = highlight("a[b]", "[")
	if got := highlight("plain", ""); got != "plain" {
		t.Fatalf("empty query = %q", got)
	}
}

func TestHighlightUnicodeSafe(t *testing.T) {
	for _, tc := range []struct{ s, q string }{{"İstanbul", "i"}, {"Kelvin K", "k"}, {"ǅ x", "ǆ"}, {"a.+*", ".+"}, {"short", "much longer"}} {
		if got := ansi.Strip(highlight(tc.s, tc.q)); got != tc.s {
			t.Fatalf("highlight(%q, %q) stripped = %q", tc.s, tc.q, got)
		}
	}
}
