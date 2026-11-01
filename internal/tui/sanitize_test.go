package tui

import "testing"

func TestSanitizeRemovesTerminalAndControlInput(t *testing.T) {
	in := "hello\x1b[31m red\x1b]0;title\x07\x1b]8;;http://x\x1b\\\x1b[2J\r\n\tworld\x00\x7f\u0085界\x1b"
	if got, want := sanitize(in), "hello red\n    world界"; got != want {
		t.Fatalf("sanitize() = %q, want %q", got, want)
	}
	if got := sanitizeLine("a\nb"); got != "a b" {
		t.Fatalf("sanitizeLine() = %q", got)
	}
}
