package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrNotLaravel = errors.New("no artisan file found")

// FindRoot walks from start upward and returns the first directory containing
// a regular file named "artisan". It returns ErrNotLaravel if the filesystem
// root is reached without a match.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	for {
		info, err := os.Stat(filepath.Join(dir, "artisan"))
		if err == nil && info.Mode().IsRegular() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotLaravel
		}
		dir = parent
	}
}

type Settings struct {
	Root            string
	LogChannel      string
	QueueConnection string
	DBConnection    string
	DBHost          string
	DBPort          string
	DBDatabase      string
	DBUsername      string
}

// SettingsFromEnv maps parsed environment values to Settings. Root is left
// empty and missing keys receive no defaults.
func SettingsFromEnv(env map[string]string) Settings {
	return Settings{
		LogChannel:      env["LOG_CHANNEL"],
		QueueConnection: env["QUEUE_CONNECTION"],
		DBConnection:    env["DB_CONNECTION"],
		DBHost:          env["DB_HOST"],
		DBPort:          env["DB_PORT"],
		DBDatabase:      env["DB_DATABASE"],
		DBUsername:      env["DB_USERNAME"],
	}
}

// Load finds the Laravel root from start and loads its .env settings.
func Load(start string) (Settings, error) {
	root, err := FindRoot(start)
	if err != nil {
		return Settings{}, err
	}

	settings := Settings{Root: root}
	src, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return settings, nil
		}
		return settings, fmt.Errorf("read .env: %w", err)
	}

	settings = SettingsFromEnv(ParseEnv(string(src)))
	settings.Root = root
	return settings, nil
}
