package logs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	t.Run("parses entries and stack trace", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "laravel.log")
		contents := strings.Join([]string{
			"[2024-01-15 10:23:45] local.error: first",
			"[stacktrace]",
			"#0 /a.php(1): f()",
			"[2024-01-15 10:23:46] production.info: second",
		}, "\n")
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("LoadFile() returned %d entries, want 2", len(entries))
		}
		if entries[0].Level != "ERROR" || entries[0].Message != "first" {
			t.Errorf("first entry = %#v, want ERROR/first", entries[0])
		}
		if len(entries[0].Trace) != 2 {
			t.Errorf("first entry trace = %#v, want 2 lines", entries[0].Trace)
		}
		if entries[1].Level != "INFO" || entries[1].Message != "second" {
			t.Errorf("second entry = %#v, want INFO/second", entries[1])
		}
	})

	t.Run("returns not exist error for missing file", func(t *testing.T) {
		entries, err := LoadFile(filepath.Join(t.TempDir(), "missing.log"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("LoadFile() error = %v, want errors.Is(err, os.ErrNotExist)", err)
		}
		if entries != nil {
			t.Errorf("LoadFile() entries = %#v, want nil", entries)
		}
	})

	t.Run("returns no entries for empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.log")
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("LoadFile() returned %d entries, want 0", len(entries))
		}
	})

	t.Run("parses final line without trailing newline", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "no-final-newline.log")
		contents := "[2024-01-15 10:23:45] local.info: first\n" +
			"[2024-01-15 10:23:46] local.error: last"
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 || entries[1].Message != "last" {
			t.Errorf("LoadFile() entries = %#v, want final entry with message last", entries)
		}
	})

	t.Run("preserves long continuation line", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "long-line.log")
		longLine := strings.Repeat("x", 200*1024)
		contents := "[2024-01-15 10:23:45] local.error: first\n" + longLine
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || len(entries[0].Trace) != 1 || entries[0].Trace[0] != longLine {
			t.Errorf("LoadFile() did not preserve the %d-byte continuation line", len(longLine))
		}
	})
}
