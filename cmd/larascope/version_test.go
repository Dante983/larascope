package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "--version")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go run --version: %v", err)
	}
	if !strings.HasPrefix(string(output), "larascope ") {
		t.Fatalf("version output = %q, want prefix %q", output, "larascope ")
	}
}
