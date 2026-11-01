package correlate

import (
	"encoding/json"
	"github.com/Dante983/larascope/internal/jobs"
	"github.com/Dante983/larascope/internal/logs"
	"math"
	"strings"
	"testing"
	"time"
)

func TestExtractLogIDs(t *testing.T) {
	e := logs.Entry{Header: logs.Header{Message: `failed {"uuid":"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA","failed_job_id":"12","job_id":13,"job":"App\\Jobs\\Send"} []`}}
	x := ExtractLogIDs(e)
	if len(x.UUIDs) != 1 || x.UUIDs[0] != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || !containsI(x.FailedJobIDs, 12) || !containsI(x.JobIDs, 13) || !containsS(x.Classes, "App\\Jobs\\Send") {
		t.Fatalf("%+v", x)
	}
}

func TestParseIDHardening(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want int64
		ok   bool
	}{
		{"number", json.Number("123"), 123, true},
		{"string", "123", 123, true},
		{"zero", json.Number("0"), 0, false},
		{"negative", json.Number("-5"), 0, false},
		{"decimal", json.Number("1.5"), 0, false},
		{"exponent", json.Number("1e3"), 0, false},
		{"max", json.Number("9223372036854775807"), math.MaxInt64, true},
		{"overflow", json.Number("9223372036854775808"), 0, false},
		{"huge", json.Number("99999999999999999999999"), 0, false},
		{"huge exponent", json.Number("1e400"), 0, false},
		{"long string", "1234567890123456789", 0, false},
		{"trimmed", " 42 ", 42, true},
		{"bad string", "12a", 0, false},
		{"bool", true, 0, false},
		{"null", nil, 0, false},
		{"array", []any{1}, 0, false},
		{"huge float", math.MaxFloat64, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseID(tc.v)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("parseID(%v) = %d, %v; want %d, %v", tc.v, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestExtractLogIDsRejectsUnsafeIDsButKeepsUUID(t *testing.T) {
	const uuid = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	for _, value := range []string{"9223372036854775808", "99999999999999999999999", "1e400", "1.5", "-5", "0"} {
		e := logs.Entry{Header: logs.Header{Message: `x {"uuid":"` + uuid + `","failed_job_id":` + value + `}`}}
		got := ExtractLogIDs(e)
		if len(got.FailedJobIDs) != 0 || len(got.UUIDs) != 1 || got.UUIDs[0] != uuid {
			t.Fatalf("%s: %+v", value, got)
		}
	}
}

func TestOverflowFixtureCannotCreateIDMatch(t *testing.T) {
	entries, err := logs.LoadFile("testdata/overflow_ids.log")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("parsed %d entries, want 5", len(entries))
	}
	for _, entry := range entries {
		ids := ExtractLogIDs(entry)
		if len(ids.FailedJobIDs) != 0 || len(ids.JobIDs) != 0 || len(ids.UUIDs) != 0 {
			t.Fatalf("unsafe ID was extracted: %+v", ids)
		}
	}
	for _, job := range []jobs.FailedRow{
		{ID: math.MaxInt64}, {ID: math.MaxInt64 - 1}, {ID: 1},
		{ID: 9223372036854775807, Class: "?"},
	} {
		if matches := LogsForJob(entries, job, 20); len(matches) != 0 {
			t.Fatalf("unsafe fixture matched job %d: %+v", job.ID, matches)
		}
	}
	nearby := jobs.FailedRow{ID: math.MaxInt64, Class: "App\\Jobs\\Overflow", FailedAt: entries[0].Time}
	for _, match := range LogsForJob(entries, nearby, 20) {
		if match.Confidence == ConfHigh || strings.Contains(strings.ToLower(match.Reason), "id") || strings.Contains(strings.ToLower(match.Reason), "uuid") {
			t.Fatalf("unsafe ID affected match: %+v", match)
		}
	}
	positive := logs.Entry{Header: logs.Header{Message: `ok {"failed_job_id":9223372036854775807}`}}
	matches := LogsForJob([]logs.Entry{positive}, jobs.FailedRow{ID: math.MaxInt64}, 20)
	if len(matches) != 1 || matches[0].Confidence != ConfHigh {
		t.Fatalf("safe max ID did not produce high match: %+v", matches)
	}
}

func TestExtractJobIDsNeverUsesCommand(t *testing.T) {
	x := ExtractJobIDs(jobs.FailedRow{ID: 1, Class: "?", Payload: `{"displayName":"App\\Jobs\\X","uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","data":{"command":"O:8:\"stdClass\":0:{}"}}`, Exception: "RuntimeException(code: 0): bad in /x.php:1"})
	if x.Class != "App\\Jobs\\X" || x.ExcClass != "RuntimeException" || x.ExcMessage != "bad" {
		t.Fatalf("%+v", x)
	}
}
func TestMalformedAndTrace(t *testing.T) {
	x := ExtractLogIDs(logs.Entry{Header: logs.Header{Message: `bad {"uuid":`}, Trace: []string{"[stacktrace]", "App\\X\\Boom: broken"}})
	if x.ExcClass != "App\\X\\Boom" || x.ExcMessage != "broken" {
		t.Fatalf("%+v", x)
	}
	_ = time.Time{}
}
