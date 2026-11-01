package jobs

import (
	"strings"
	"testing"
)

func TestClassifyFailedSelectAndPayload(t *testing.T) {
	if classify(nil, 2, 1) != "delayed" || classify(nil, 1, 1) != "ready" {
		t.Fatal("classify")
	}
	r := int64(1)
	if classify(&r, 1, 2) != "reserved" {
		t.Fatal("reserved")
	}
	if got := buildFailedSelect(map[string]bool{"id": true, "uuid": true}); !strings.Contains(got, "id,uuid") {
		t.Fatal(got)
	}
	if _, e := DecodePayload("bad"); e == nil {
		t.Fatal("bad payload accepted")
	}
	if p, e := DecodePayload(`{"displayName":"Job"}`); e != nil || p.DisplayName != "Job" {
		t.Fatalf("payload=%#v err=%v", p, e)
	}
}

func TestPostgresDialectAndOpen(t *testing.T) {
	cols := map[string]bool{"id": true, "uuid": true}
	if got := buildFailedSelectFor(cols, "$1"); !strings.HasSuffix(got, "LIMIT $1") {
		t.Fatal(got)
	}
	if got := buildFailedSelect(cols); !strings.HasSuffix(got, "LIMIT ?") {
		t.Fatal(got)
	}
	for _, sql := range []string{pgDialect.countsSQL, pgDialect.pendingSQL, pgDialect.columnsSQL, pgDialect.checkDBSQL, pgDialect.failedSelect(cols)} {
		if !strings.HasPrefix(sql, "SELECT") {
			t.Fatalf("non-read-only SQL: %s", sql)
		}
	}
	if !strings.Contains(pgDialect.countsSQL, "FILTER (WHERE") || !strings.Contains(pgDialect.countsSQL, "$1") || strings.Contains(pgDialect.countsSQL, "UNIX_TIMESTAMP") || !strings.Contains(pgDialect.columnsSQL, "current_schema()") {
		t.Fatal("invalid PostgreSQL dialect")
	}
	if _, err := Open("sqlite", ""); err == nil {
		t.Fatal("sqlite accepted")
	}
	s, err := Open("pgsql", "postgres://localhost/app")
	if err != nil || s == nil {
		t.Fatalf("Open pgsql = %v, %v", s, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
