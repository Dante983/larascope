package correlate

import (
	"fmt"
	"larascope/internal/jobs"
	"larascope/internal/logs"
	"sort"
	"strings"
	"time"
)

type Confidence int

const (
	ConfNone Confidence = iota
	ConfLow
	ConfMedium
	ConfHigh
)

func (c Confidence) String() string { return []string{"none", "low", "medium", "high"}[c] }

type Match struct {
	LogIndex, JobIndex int
	Confidence         Confidence
	Score              int
	Reason             string
}

const TimeWindow = 2 * time.Minute

func containsS(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}
func containsI(a []int64, n int64) bool {
	for _, x := range a {
		if x == n {
			return true
		}
	}
	return false
}
func warning(level string) bool {
	switch strings.ToUpper(level) {
	case "WARNING", "ERROR", "CRITICAL", "ALERT", "EMERGENCY":
		return true
	}
	return false
}
func generic(c string) bool {
	switch c {
	case "Exception", "RuntimeException", "Error", "Throwable", "InvalidArgumentException", "LogicException":
		return true
	}
	return false
}
func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
func timeReason(d time.Duration) string {
	s := int64(absDur(d).Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds apart", s)
	}
	return fmt.Sprintf("%dm%ds apart", s/60, s%60)
}
func ScorePair(l logs.Entry, li LogIDs, f jobs.FailedRow, ji JobIDs) (Match, bool) {
	m := Match{Score: 0}
	rs := []string{}
	id := false
	nonID := 0
	if ji.UUID != "" && containsS(li.UUIDs, ji.UUID) {
		m.Score += 100
		rs = append(rs, "uuid in log context")
		id = true
	} else if containsI(li.FailedJobIDs, f.ID) {
		m.Score += 90
		rs = append(rs, "failed_job_id in log context")
		id = true
	} else if containsI(li.JobIDs, f.ID) && ji.Class != "" && containsS(li.Classes, ji.Class) {
		m.Score += 60
		rs = append(rs, "job_id + class")
		id = true
	}
	if li.ExcClass != "" && li.ExcClass == ji.ExcClass {
		if msgEqual(li.ExcMessage, ji.ExcMessage) {
			nonID += 50
			rs = append(rs, "exception class+message")
		} else if !generic(li.ExcClass) {
			nonID += 15
			rs = append(rs, "exception class")
		}
	}
	if ji.Class != "" && containsS(li.Classes, ji.Class) {
		nonID += 20
		rs = append(rs, "job class mentioned")
	}
	if id || warning(l.Level) {
		m.Score += nonID
	} else {
		rs = rs[:0]
	}
	if m.Score == 0 {
		return m, false
	}
	if !f.FailedAt.IsZero() && !l.Time.IsZero() {
		d := f.FailedAt.Sub(l.Time)
		if absDur(d) <= TimeWindow {
			if d >= 0 {
				m.Score += 10
			} else {
				m.Score += 5
			}
			rs = append(rs, timeReason(d))
		}
	}
	m.Reason = strings.Join(rs, " + ")
	if id {
		m.Confidence = ConfHigh
	} else if m.Score >= 50 {
		m.Confidence = ConfMedium
	} else if m.Score >= 25 {
		m.Confidence = ConfLow
	}
	return m, m.Confidence >= ConfLow
}
func LogsForJob(es []logs.Entry, f jobs.FailedRow, limit int) []Match {
	ji := ExtractJobIDs(f)
	out := []Match{}
	for i, e := range es {
		if m, ok := ScorePair(e, ExtractLogIDs(e), f, ji); ok {
			m.LogIndex = i
			m.JobIndex = -1
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		di, dj := absDur(es[out[i].LogIndex].Time.Sub(f.FailedAt)), absDur(es[out[j].LogIndex].Time.Sub(f.FailedAt))
		if di != dj {
			return di < dj
		}
		return out[i].LogIndex < out[j].LogIndex
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
func JobsForLog(e logs.Entry, fs []jobs.FailedRow, limit int) []Match {
	li := ExtractLogIDs(e)
	out := []Match{}
	for i, f := range fs {
		if m, ok := ScorePair(e, li, f, ExtractJobIDs(f)); ok {
			m.LogIndex = -1
			m.JobIndex = i
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		di, dj := absDur(e.Time.Sub(fs[out[i].JobIndex].FailedAt)), absDur(e.Time.Sub(fs[out[j].JobIndex].FailedAt))
		if di != dj {
			return di < dj
		}
		return out[i].JobIndex < out[j].JobIndex
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
