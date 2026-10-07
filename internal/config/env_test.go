package config

import (
	"reflect"
	"testing"
)

func TestParseEnv(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			name: "basic assignments",
			src:  "A=b\nEMPTY=\n  SPACED = b ",
			want: map[string]string{"A": "b", "EMPTY": "", "SPACED": "b"},
		},
		{
			name: "comments blanks and export",
			src:  "# comment\n\n export A=b\n  # another comment",
			want: map[string]string{"A": "b"},
		},
		{
			name: "quoted and unquoted comments",
			src:  "DOUBLE=\"x y # z\"\nSINGLE='x # y'\nCOMMENT=x # c\nHASH=x#y",
			want: map[string]string{"DOUBLE": "x y # z", "SINGLE": "x # y", "COMMENT": "x", "HASH": "x#y"},
		},
		{
			name: "double quoted escapes",
			src:  "QUOTE=\"a\\\"b\"\nLINES=\"l1\\nl2\"\nSLASH=\"a\\\\b\"",
			want: map[string]string{"QUOTE": "a\"b", "LINES": "l1\nl2", "SLASH": "a\\b"},
		},
		{
			name: "unterminated quotes",
			src:  "DOUBLE=\"unterminated\nSINGLE='unterminated",
			want: map[string]string{"DOUBLE": "unterminated", "SINGLE": "unterminated"},
		},
		{
			name: "invalid assignments",
			src:  "not an assignment\n=v",
			want: map[string]string{},
		},
		{
			name: "duplicates and CRLF",
			src:  "A=first\r\nA=last\r\nB=value\r\n",
			want: map[string]string{"A": "last", "B": "value"},
		},
		{
			name: "empty input",
			src:  "",
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseEnv(tt.src)
			if got == nil {
				t.Fatal("ParseEnv() returned a nil map")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseEnv() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
