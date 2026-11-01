package logs

import (
	"os"
	"path/filepath"
)

func LatestLogFile(root, channel string) string { // channel is retained for future channel-aware discovery
	base := filepath.Join(root, "storage", "logs", "laravel.log")
	if _, err := os.Stat(base); err == nil {
		return base
	}
	matches, _ := filepath.Glob(filepath.Join(root, "storage", "logs", "laravel-*.log"))
	var newest string
	var newestTime int64
	for _, p := range matches {
		if i, e := os.Stat(p); e == nil && i.ModTime().UnixNano() > newestTime {
			newest, newestTime = p, i.ModTime().UnixNano()
		}
	}
	if newest != "" {
		return newest
	}
	return base
}
