// Package logs provides parsing support for Laravel log files.
package logs

import (
	"regexp"
	"strings"
	"time"
)

var headerPattern = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})(?:\.\d+)?(?:[+-]\d{2}:?\d{2})?\] ([A-Za-z0-9_.-]+)\.([A-Za-z]+): (.*)$`)

// Header is the parsed metadata and message from one Laravel log header line.
type Header struct {
	Time    time.Time
	Env     string
	Level   string
	Message string
}

// ParseHeader parses one Laravel log header line.
func ParseHeader(line string) (Header, bool) {
	line = strings.TrimSuffix(line, "\r")

	matches := headerPattern.FindStringSubmatch(line)
	if matches == nil {
		return Header{}, false
	}

	timestamp, err := time.Parse("2006-01-02 15:04:05", matches[1])
	if err != nil {
		return Header{}, false
	}

	return Header{
		Time:    timestamp,
		Env:     matches[2],
		Level:   strings.ToUpper(matches[3]),
		Message: matches[4],
	}, true
}
