package correlate

import (
	"larascope/internal/jobs"
	"larascope/internal/logs"
	"testing"
	"time"
)

func TestScoring(t *testing.T) {
	now := time.Now()
	f := jobs.FailedRow{ID: 7, UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Class: "App\\Jobs\\X", FailedAt: now, Exception: "App\\Boom: nope"}
	e := logs.Entry{Header: logs.Header{Time: now.Add(-time.Second), Level: "INFO", Message: `x {"uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"} []`}}
	m, ok := ScorePair(e, ExtractLogIDs(e), f, ExtractJobIDs(f))
	if !ok || m.Confidence != ConfHigh {
		t.Fatalf("%+v %v", m, ok)
	}
	e.Level = "WARNING"
	e.Message = "nothing"
	if _, ok := ScorePair(e, ExtractLogIDs(e), f, ExtractJobIDs(f)); ok {
		t.Fatal("time-only matched")
	}
}
func TestOrderingAndLimits(t *testing.T) {
	now := time.Now()
	f := jobs.FailedRow{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", FailedAt: now}
	es := []logs.Entry{{Header: logs.Header{Time: now.Add(-time.Second), Level: "INFO", Message: `{"uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`}}, {Header: logs.Header{Time: now.Add(-2 * time.Second), Level: "INFO", Message: `{"uuid":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`}}}
	x := LogsForJob(es, f, 1)
	if len(x) != 1 || x[0].LogIndex != 0 {
		t.Fatalf("%+v", x)
	}
}
