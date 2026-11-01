package config

import "testing"

func TestRedactEscapedAndDSNDefaults(t *testing.T) {
	if got := Redact("a s3cr3t p%40ss", "s3cr3t", "p@ss", ""); got != "a *** ***" {
		t.Fatalf("Redact = %q", got)
	}
	if got := (Settings{}).DSN(); got != ":@tcp(127.0.0.1:3306)/?timeout=3s&readTimeout=3s&parseTime=true" {
		t.Fatalf("DSN = %q", got)
	}
}
