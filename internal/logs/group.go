package logs

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	uuidRE  = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f-]{27}`)
	numRE   = regexp.MustCompile(`\d+`)
	quoteRE = regexp.MustCompile(`'[^']*'|"[^"]*"`)
	frameRE = regexp.MustCompile(`^#0 (.*)`)
)

type Filter struct {
	Level string
	Query string
}

func (f Filter) Apply(es []Entry) []int {
	var out []int
	for i, e := range es {
		if f.Level != "" && !strings.EqualFold(f.Level, e.Level) {
			continue
		}
		hay := e.Message + "\n" + strings.Join(e.Trace, "\n")
		if f.Query != "" && !strings.Contains(strings.ToLower(hay), strings.ToLower(f.Query)) {
			continue
		}
		out = append(out, i)
	}
	return out
}
func Fingerprint(e Entry) string {
	s := e.Message
	s = uuidRE.ReplaceAllString(s, "<uuid>")
	s = numRE.ReplaceAllString(s, "<n>")
	s = quoteRE.ReplaceAllString(s, "<s>")
	for _, x := range e.Trace {
		if m := frameRE.FindStringSubmatch(x); m != nil {
			s = m[1] + "|" + s
			break
		}
	}
	return e.Level + "|" + s
}

type Group struct {
	Fingerprint string
	Count       int
	First, Last time.Time
	Sample      Entry
	Indexes     []int
}

func GroupEntries(es []Entry) []Group {
	m := map[string]*Group{}
	for i, e := range es {
		k := Fingerprint(e)
		g := m[k]
		if g == nil {
			g = &Group{Fingerprint: k, First: e.Time, Last: e.Time, Sample: e}
			m[k] = g
		}
		g.Count++
		g.Indexes = append(g.Indexes, i)
		if e.Time.Before(g.First) {
			g.First = e.Time
		}
		if e.Time.After(g.Last) {
			g.Last = e.Time
		}
	}
	out := make([]Group, 0, len(m))
	for _, g := range m {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Last.After(out[j].Last)
		}
		return out[i].Count > out[j].Count
	})
	return out
}
