// Package config contains application configuration helpers.
package config

import (
	"bufio"
	"strings"
)

// ParseEnv parses dotenv text into key->value.
//
// Variable interpolation (for example, ${VAR}) is not supported.
func ParseEnv(src string) map[string]string {
	values := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(src))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, raw, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		values[key] = parseEnvValue(strings.TrimSpace(raw))
	}

	return values
}

func parseEnvValue(raw string) string {
	if raw == "" {
		return ""
	}

	switch raw[0] {
	case '"':
		return parseDoubleQuotedEnvValue(raw)
	case '\'':
		if end := strings.IndexByte(raw[1:], '\''); end >= 0 {
			return raw[1 : end+1]
		}
		return raw[1:]
	default:
		if comment := strings.Index(raw, " #"); comment >= 0 {
			raw = raw[:comment]
		}
		return strings.TrimSpace(raw)
	}
}

func parseDoubleQuotedEnvValue(raw string) string {
	var value strings.Builder
	for i := 1; i < len(raw); i++ {
		if raw[i] == '"' {
			return value.String()
		}

		if raw[i] == '\\' && i+1 < len(raw) {
			switch raw[i+1] {
			case '"':
				value.WriteByte('"')
				i++
				continue
			case 'n':
				value.WriteByte('\n')
				i++
				continue
			case '\\':
				value.WriteByte('\\')
				i++
				continue
			}
		}

		value.WriteByte(raw[i])
	}

	return raw[1:]
}
