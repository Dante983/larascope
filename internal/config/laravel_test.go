package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindRoot(t *testing.T) {
	t.Run("artisan in start directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "artisan"), nil, 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := FindRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		if got != root {
			t.Errorf("FindRoot() = %q, want %q", got, root)
		}
	})

	t.Run("artisan in parent directory", func(t *testing.T) {
		root := t.TempDir()
		start := filepath.Join(root, "app", "Http")
		if err := os.MkdirAll(start, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "artisan"), nil, 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := FindRoot(start)
		if err != nil {
			t.Fatal(err)
		}
		if got != root {
			t.Errorf("FindRoot() = %q, want %q", got, root)
		}
	})

	t.Run("no artisan", func(t *testing.T) {
		_, err := FindRoot(t.TempDir())
		if !errors.Is(err, ErrNotLaravel) {
			t.Errorf("FindRoot() error = %v, want ErrNotLaravel", err)
		}
	})

	t.Run("artisan directory is ignored", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "artisan"), 0o755); err != nil {
			t.Fatal(err)
		}

		_, err := FindRoot(root)
		if !errors.Is(err, ErrNotLaravel) {
			t.Errorf("FindRoot() error = %v, want ErrNotLaravel", err)
		}
	})
}

func TestSettingsFromEnv(t *testing.T) {
	env := map[string]string{
		"LOG_CHANNEL":      "daily",
		"QUEUE_CONNECTION": "database",
		"DB_CONNECTION":    "mysql",
		"DB_HOST":          "127.0.0.1",
		"DB_PORT":          "3306",
		"DB_DATABASE":      "larascope",
		"DB_USERNAME":      "sail",
	}
	want := Settings{
		LogChannel:      "daily",
		QueueConnection: "database",
		DBConnection:    "mysql",
		DBHost:          "127.0.0.1",
		DBPort:          "3306",
		DBDatabase:      "larascope",
		DBUsername:      "sail",
	}
	if got := SettingsFromEnv(env); got != want {
		t.Errorf("SettingsFromEnv() = %#v, want %#v", got, want)
	}

	if got := SettingsFromEnv(map[string]string{}); got != (Settings{}) {
		t.Errorf("SettingsFromEnv(empty) = %#v, want zero Settings", got)
	}
}

func TestLoad(t *testing.T) {
	t.Run("loads environment settings", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "artisan"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		env := "LOG_CHANNEL=daily\nQUEUE_CONNECTION=database\nDB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=larascope\nDB_USERNAME=sail\n"
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte(env), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		want := Settings{Root: root, LogChannel: "daily", QueueConnection: "database", DBConnection: "mysql", DBHost: "127.0.0.1", DBPort: "3306", DBDatabase: "larascope", DBUsername: "sail"}
		if got != want {
			t.Errorf("Load() = %#v, want %#v", got, want)
		}
	})

	t.Run("missing environment file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "artisan"), nil, 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if got != (Settings{Root: root}) {
			t.Errorf("Load() = %#v, want Settings{Root: %q}", got, root)
		}
	})

	t.Run("no artisan", func(t *testing.T) {
		_, err := Load(t.TempDir())
		if !errors.Is(err, ErrNotLaravel) {
			t.Errorf("Load() error = %v, want ErrNotLaravel", err)
		}
	})
}
