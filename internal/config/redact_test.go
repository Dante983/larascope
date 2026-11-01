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

func TestRedactPgPasswordForms(t *testing.T) {
	s := Settings{DBUsername: "user", DBPassword: "p@ss w/rd", DBDatabase: "app"}
	got := Redact("raw p@ss w/rd query p%40ss+w%2Frd path p@ss%20w%2Frd dsn "+s.PgDSN(), s.Secrets()...)
	if got != "raw *** query *** path *** dsn ***" {
		t.Fatalf("Redact = %q", got)
	}
}
