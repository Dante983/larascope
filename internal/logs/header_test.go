package logs

import (
	"testing"
	"time"
)

func TestParseHeader(t *testing.T) {
	expectedTime := time.Date(2024, time.January, 15, 10, 23, 45, 0, time.UTC)

	tests := []struct {
		name string
		line string
		want Header
		ok   bool
	}{
		{
			name: "header with JSON context",
			line: `[2024-01-15 10:23:45] production.ERROR: Boom {"userId":1} []`,
			want: Header{
				Time:    expectedTime,
				Env:     "production",
				Level:   "ERROR",
				Message: `Boom {"userId":1} []`,
			},
			ok: true,
		},
		{
			name: "message containing colon",
			line: "[2024-01-15 10:23:45] local.info: a: b",
			want: Header{
				Time:    expectedTime,
				Env:     "local",
				Level:   "INFO",
				Message: "a: b",
			},
			ok: true,
		},
		{
			name: "fractional seconds and timezone offset",
			line: "[2024-01-15 10:23:45.123456+00:00] local.WARNING: x",
			want: Header{
				Time:    expectedTime,
				Env:     "local",
				Level:   "WARNING",
				Message: "x",
			},
			ok: true,
		},
		{
			name: "stack frame",
			line: "#0 /var/www/x.php(12): foo()",
			ok:   false,
		},
		{
			name: "empty",
			line: "",
			ok:   false,
		},
		{
			name: "stacktrace marker",
			line: "[stacktrace]",
			ok:   false,
		},
		{
			name: "main stack frame",
			line: "#1 {main}",
			ok:   false,
		},
		{
			name: "CRLF",
			line: "[2024-01-15 10:23:45] local.error: failed\r",
			want: Header{
				Time:    expectedTime,
				Env:     "local",
				Level:   "ERROR",
				Message: "failed",
			},
			ok: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseHeader(tt.line)
			if ok != tt.ok {
				t.Fatalf("ParseHeader(%q) ok = %v, want %v", tt.line, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("ParseHeader(%q) = %#v, want %#v", tt.line, got, tt.want)
			}
		})
	}
}
