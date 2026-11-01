// Package correlate matches already-loaded Laravel logs and failed jobs locally.
package correlate

import (
	"encoding/json"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Dante983/larascope/internal/jobs"
	"github.com/Dante983/larascope/internal/logs"
)

type LogIDs struct {
	UUIDs                []string
	FailedJobIDs         []int64
	JobIDs               []int64
	Classes              []string
	ExcClass, ExcMessage string
}
type JobIDs struct {
	UUID                 string
	ID                   int64
	Class                string
	JobClass             string
	ExcClass, ExcMessage string
}

var (
	uuidRE       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	fqcnRE       = regexp.MustCompile(`^\\?[A-Za-z_][A-Za-z0-9_]*(\\[A-Za-z_][A-Za-z0-9_]*)+$`)
	fqcnTokenRE  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\\[A-Za-z_][A-Za-z0-9_]*)+`)
	objectExcRE  = regexp.MustCompile(`\(([A-Za-z_\\][A-Za-z0-9_\\]*)\(code: -?\d+\): (.*?)(?: at /|\)?$)`)
	prefixExcRE  = regexp.MustCompile(`^([A-Za-z_\\][A-Za-z0-9_\\]*(?:\\[A-Za-z0-9_]+)+): (.*)$`)
	jobExcRE     = regexp.MustCompile(`^([A-Za-z_\\][A-Za-z0-9_\\]*)(?:\(code: -?\d+\))?: (.*?)(?: in /\S+:\d+)?$`)
	suffixPathRE = regexp.MustCompile(` in /\S+:\d+$`)
)

func normClass(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), `\`) }
func addString(a []string, s string, max int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return a
	}
	for _, x := range a {
		if x == s {
			return a
		}
	}
	if len(a) < max {
		return append(a, s)
	}
	return a
}
func addID(a []int64, v int64) []int64 {
	for _, x := range a {
		if x == v {
			return a
		}
	}
	if len(a) < 10 {
		return append(a, v)
	}
	return a
}

// parseID converts a decoded JSON value to a positive int64.
// It returns false for unsafe, lossy, or non-positive values.
func parseID(v any) (int64, bool) {
	switch x := v.(type) {
	case json.Number:
		s := x.String()
		if s == "" || strings.ContainsAny(s, ".eE+-") {
			return 0, false
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || x != math.Trunc(x) || x <= 0 || x > 1<<53 {
			return 0, false
		}
		return int64(x), true
	case string:
		s := strings.TrimSpace(x)
		if len(s) == 0 || len(s) > 18 {
			return 0, false
		}
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return 0, false
			}
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
func normMsg(s string) string {
	return capRunes(strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " "), 200)
}

// msgEqual accepts equal messages and a >=20-rune prefix, for Laravel's added context suffixes.
func msgEqual(a, b string) bool {
	a, b = normMsg(a), normMsg(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if utf8.RuneCountInString(a) > utf8.RuneCountInString(b) {
		a, b = b, a
	}
	return utf8.RuneCountInString(a) >= 20 && strings.HasPrefix(b, a)
}

// lastJSONObjects peels up to two trailing Monolog context/extra objects safely.
func lastJSONObjects(msg string) []map[string]any {
	if len(msg) > 8192 {
		msg = msg[len(msg)-8192:]
	}
	var out []map[string]any
	for len(out) < 2 {
		msg = strings.TrimSpace(msg)
		if strings.HasSuffix(msg, "[]") {
			msg = strings.TrimSpace(strings.TrimSuffix(msg, "[]"))
			continue
		}
		found := false
		tries := 0
		for i := len(msg) - 1; i >= 0 && tries < 20; i-- {
			if msg[i] != '{' {
				continue
			}
			tries++
			var m map[string]any
			dec := json.NewDecoder(strings.NewReader(msg[i:]))
			dec.UseNumber()
			var extra any
			if dec.Decode(&m) == nil && dec.Decode(&extra) == io.EOF {
				out = append(out, m)
				msg = msg[:i]
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return out
}
func setExc(l *LogIDs, class, msg string) {
	if l.ExcClass == "" {
		l.ExcClass = normClass(class)
		l.ExcMessage = capRunes(firstLine(msg), 200)
	}
}
func ExtractLogIDs(e logs.Entry) LogIDs {
	var out LogIDs
	var visit func(map[string]any, int)
	visit = func(m map[string]any, depth int) {
		if depth > 3 {
			return
		}
		for key, v := range m {
			k := strings.ToLower(key)
			switch x := v.(type) {
			case string:
				if (k == "uuid" || k == "job_uuid" || k == "jobuuid" || k == "failed_job_uuid" || k == "job_id") && uuidRE.MatchString(x) {
					out.UUIDs = addString(out.UUIDs, strings.ToLower(x), 10)
				}
				if (k == "job" || k == "class" || k == "displayname" || k == "job_class" || k == "command") && fqcnRE.MatchString(x) {
					out.Classes = addString(out.Classes, normClass(x), 10)
				}
				if k == "exception" {
					if q := objectExcRE.FindStringSubmatch(x); q != nil {
						setExc(&out, q[1], q[2])
					}
				}
			case map[string]any:
				if k == "job" {
					for name, y := range x {
						nk := strings.ToLower(name)
						if s, ok := y.(string); ok {
							if nk == "uuid" && uuidRE.MatchString(s) {
								out.UUIDs = addString(out.UUIDs, strings.ToLower(s), 10)
							}
							if (nk == "name" || nk == "class" || nk == "displayname") && fqcnRE.MatchString(s) {
								out.Classes = addString(out.Classes, normClass(s), 10)
							}
						}
						if nk == "id" {
							if n, ok := parseID(y); ok {
								out.JobIDs = addID(out.JobIDs, n)
							}
						}
					}
				}
				visit(x, depth+1)
			}
			if k == "failed_job_id" {
				if n, ok := parseID(v); ok {
					out.FailedJobIDs = addID(out.FailedJobIDs, n)
				}
			}
			if k == "job_id" {
				if n, ok := parseID(v); ok {
					out.JobIDs = addID(out.JobIDs, n)
				}
			}
		}
	}
	for _, m := range lastJSONObjects(e.Message) {
		visit(m, 0)
	}
	prefix := e.Message
	if i := strings.Index(prefix, " {"); i >= 0 {
		prefix = prefix[:i]
	}
	prefix = strings.TrimSpace(suffixPathRE.ReplaceAllString(firstLine(prefix), ""))
	if q := prefixExcRE.FindStringSubmatch(prefix); q != nil {
		setExc(&out, q[1], q[2])
	}
	for _, x := range fqcnTokenRE.FindAllString(firstLine(prefix), -1) {
		out.Classes = addString(out.Classes, normClass(x), 10)
	}
	for _, line := range e.Trace[:min(30, len(e.Trace))] {
		if strings.HasPrefix(line, "[stacktrace]") {
			continue
		}
		if q := objectExcRE.FindStringSubmatch(line); q != nil {
			setExc(&out, q[1], q[2])
			break
		}
		if q := prefixExcRE.FindStringSubmatch(line); q != nil {
			setExc(&out, q[1], q[2])
			break
		}
	}
	return out
}
func ExtractJobIDs(f jobs.FailedRow) JobIDs {
	out := JobIDs{ID: f.ID}
	p, err := jobs.DecodePayload(f.Payload)
	if err == nil {
		out.JobClass = p.Job
		if f.UUID == "" {
			out.UUID = strings.ToLower(p.UUID)
		}
		if f.Class == "" || f.Class == "?" {
			out.Class = normClass(p.DisplayName)
		}
	}
	if f.UUID != "" {
		out.UUID = strings.ToLower(f.UUID)
	}
	if f.Class != "" && f.Class != "?" {
		out.Class = normClass(f.Class)
	}
	line := firstLine(f.Exception)
	if q := jobExcRE.FindStringSubmatch(line); q != nil {
		out.ExcClass = normClass(q[1])
		out.ExcMessage = capRunes(firstLine(q[2]), 200)
	}
	return out
}
