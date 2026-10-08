package logs

import (
	"reflect"
	"testing"
)

func TestParseEntries(t *testing.T) {
	headerOne := "[2024-01-15 10:23:45] local.error: first"
	headerTwo := "[2024-01-15 10:23:46] production.info: second"

	tests := []struct {
		name  string
		lines []string
		want  []Entry
	}{
		{
			name:  "two single-line entries",
			lines: []string{headerOne, headerTwo},
			want: []Entry{
				{Header: Header{Level: "ERROR", Message: "first"}},
				{Header: Header{Level: "INFO", Message: "second"}},
			},
		},
		{
			name: "stack trace belongs to preceding header",
			lines: []string{
				headerOne,
				"[stacktrace]",
				"#0 /a.php(1): f()",
				"#1 {main}",
				headerTwo,
			},
			want: []Entry{
				{
					Header: Header{Level: "ERROR", Message: "first"},
					Trace:  []string{"[stacktrace]", "#0 /a.php(1): f()", "#1 {main}"},
				},
				{Header: Header{Level: "INFO", Message: "second"}},
			},
		},
		{
			name:  "preamble is dropped",
			lines: []string{"Laravel log", headerOne},
			want:  []Entry{{Header: Header{Level: "ERROR", Message: "first"}}},
		},
		{
			name: "CRLF trace is trimmed",
			lines: []string{
				headerOne + "\r",
				"#0 /a.php(1): f()\r",
			},
			want: []Entry{{
				Header: Header{Level: "ERROR", Message: "first"},
				Trace:  []string{"#0 /a.php(1): f()"},
			}},
		},
		{
			name: "trailing blank lines are stripped and interior blank line is kept",
			lines: []string{
				headerOne,
				"#0 /a.php(1): f()",
				"",
				"#1 {main}",
				"",
				"",
			},
			want: []Entry{{
				Header: Header{Level: "ERROR", Message: "first"},
				Trace:  []string{"#0 /a.php(1): f()", "", "#1 {main}"},
			}},
		},
		{
			name:  "invalid header date is continuation",
			lines: []string{headerOne, "[2024-13-15 10:23:45] local.error: invalid"},
			want: []Entry{{
				Header: Header{Level: "ERROR", Message: "first"},
				Trace:  []string{"[2024-13-15 10:23:45] local.error: invalid"},
			}},
		},
		{
			name:  "empty and nil input",
			lines: nil,
			want:  nil,
		},
		{
			name:  "empty input",
			lines: []string{},
			want:  nil,
		},
		{
			name:  "only preamble",
			lines: []string{"Laravel log", "#0 {main}"},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEntries(tt.lines)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseEntries(%q) returned %d entries, want %d", tt.lines, len(got), len(tt.want))
			}

			for i := range got {
				if got[i].Level != tt.want[i].Level || got[i].Message != tt.want[i].Message || !reflect.DeepEqual(got[i].Trace, tt.want[i].Trace) {
					t.Errorf("ParseEntries(%q)[%d] = %#v, want %#v", tt.lines, i, got[i], tt.want[i])
				}
			}
		})
	}
}
