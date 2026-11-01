package version

import (
	"strings"
	"testing"
)

func TestStringDefaults(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("Version = %q, want dev", Version)
	}
	if !strings.Contains(String(), "larascope") {
		t.Fatalf("String() = %q, want larascope", String())
	}
}

func TestStringIncludesCommit(t *testing.T) {
	oldCommit := Commit
	Commit = "abc123"
	t.Cleanup(func() { Commit = oldCommit })
	if !strings.Contains(String(), "commit abc123") {
		t.Fatalf("String() = %q, want commit", String())
	}
}
