package logs

import "strings"

// Entry is one log record: a header and its continuation lines.
type Entry struct {
	Header
	Trace []string // continuation lines, in order, "\r" trimmed, no trailing newline
}

// ParseEntries groups lines into entries. Lines before the first valid header are dropped.
func ParseEntries(lines []string) []Entry {
	var entries []Entry

	for _, line := range lines {
		header, ok := ParseHeader(line)
		if ok {
			entries = append(entries, Entry{Header: header})
		} else if len(entries) > 0 {
			entries[len(entries)-1].Trace = append(entries[len(entries)-1].Trace, strings.TrimSuffix(line, "\r"))
		}
	}

	for i := range entries {
		for len(entries[i].Trace) > 0 && entries[i].Trace[len(entries[i].Trace)-1] == "" {
			entries[i].Trace = entries[i].Trace[:len(entries[i].Trace)-1]
		}
	}

	return entries
}
