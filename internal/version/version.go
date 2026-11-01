// Package version provides build metadata for the Larascope executable.
package version

import "runtime/debug"

// Set via -ldflags "-X github.com/Dante983/larascope/internal/version.Version=... .Commit=... .Date=..."
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns the human-readable build version.
func String() string {
	return "larascope " + resolve() + " (commit " + Commit + ", built " + Date + ")"
}

func resolve() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
