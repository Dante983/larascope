package artisan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandDefaultsWhitespacePHP(t *testing.T) {
	t.Setenv("LARASCOPE_PHP", "  ")
	cmd := (Runner{PHP: "   ", Root: "/tmp"}).command(context.Background(), "queue:retry", "abc")
	if cmd.Args[0] != "php" || cmd.Dir != "/tmp" || strings.Join(cmd.Args, " ") != "php artisan queue:retry abc" {
		t.Fatalf("command = path=%q dir=%q args=%q", cmd.Path, cmd.Dir, cmd.Args)
	}
	cmd = (Runner{PHP: "docker compose exec app php"}).command(context.Background(), "x")
	if got := strings.Join(cmd.Args, " "); got != "docker compose exec app php artisan x" {
		t.Fatalf("args = %q", got)
	}
}
func TestRunnerValidationAndRun(t *testing.T) {
	r := Runner{}
	for _, id := range []string{"1; rm -rf", "", "../x"} {
		if _, e := r.Retry(context.Background(), id); e == nil {
			t.Errorf("Retry(%q) accepted", id)
		}
	}
	script := filepath.Join(t.TempDir(), "php")
	if e := os.WriteFile(script, []byte("#!/bin/sh\necho worked\n"), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("LARASCOPE_PHP", script)
	for _, id := range []string{"123", "9b7a44d1-b555-4fa7-9c4a-9a9d8a994333"} {
		if _, e := r.Forget(context.Background(), id); e != nil {
			t.Errorf("Forget(%q): %v", id, e)
		}
	}
	if out, e := r.Run(context.Background(), "about"); e != nil || !strings.Contains(out, "worked") {
		t.Fatalf("Run = %q, %v", out, e)
	}
	t.Setenv("LARASCOPE_PHP", filepath.Join(t.TempDir(), "missing"))
	if _, e := r.Run(context.Background()); e == nil || e.Error() != "php not found; set LARASCOPE_PHP" {
		t.Fatalf("missing error = %v", e)
	}
}

func TestJobIDValidationDoesNotExecutePHP(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	script := filepath.Join(t.TempDir(), "php")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LARASCOPE_PHP", script)
	r := Runner{}
	reject := []string{"", "-", "--", "-1", "--force", "-abc", "--all", "all", " 1", "1 ", "1\n", "1;rm", "../x", "abc", "9b7a44d1-b555-4fa7-9c4a", "-9b7a44d1-b555-4fa7-9c4a-9a9d8a994333", "123456789012345678901", "１２３"}
	for _, id := range reject {
		if _, err := r.Retry(context.Background(), id); err == nil || err.Error() != "invalid job id" {
			t.Errorf("Retry(%q) error = %v", id, err)
		}
		if _, err := r.Forget(context.Background(), id); err == nil || err.Error() != "invalid job id" {
			t.Errorf("Forget(%q) error = %v", id, err)
		}
	}
	if _, err := r.Retry(context.Background()); err == nil || err.Error() != "invalid job id" {
		t.Errorf("Retry() error = %v", err)
	}
	if _, err := r.Retry(context.Background(), "1", "--x"); err == nil || err.Error() != "invalid job id" {
		t.Errorf("Retry with invalid member error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("invalid IDs executed PHP: stat error = %v", err)
	}
}

func TestJobIDValidationRunsCanonicalArguments(t *testing.T) {
	const uuid = "9b7a44d1-b555-4fa7-9c4a-9a9d8a994333"
	for _, tc := range []struct {
		name string
		run  func(Runner) (string, error)
		want string
	}{
		{"retry numeric", func(r Runner) (string, error) { return r.Retry(context.Background(), "123") }, "artisan queue:retry 123"},
		{"forget UUID", func(r Runner) (string, error) { return r.Forget(context.Background(), uuid) }, "artisan queue:forget " + uuid},
		{"retry uppercase UUID", func(r Runner) (string, error) { return r.Retry(context.Background(), strings.ToUpper(uuid)) }, "artisan queue:retry " + strings.ToUpper(uuid)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := filepath.Join(t.TempDir(), "args")
			script := filepath.Join(t.TempDir(), "php")
			if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" >> "+args+"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("LARASCOPE_PHP", script)
			if _, err := tc.run(Runner{}); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(args)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(string(got)); got != tc.want {
				t.Fatalf("arguments = %q, want %q", got, tc.want)
			}
		})
	}
}
