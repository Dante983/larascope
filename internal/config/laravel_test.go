package config

import (
	"errors"
	"net/url"
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

func TestDriverAndPgDSN(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"mysql", "mysql"}, {"MariaDB", "mysql"}, {"pgsql", "pgsql"}, {"postgres", "pgsql"}, {"postgresql", "pgsql"}, {"PGSQL", "pgsql"}, {" pgsql ", "pgsql"}, {"sqlite", ""}, {"", ""}} {
		if got := (Settings{DBConnection: tc.in}).Driver(); got != tc.want {
			t.Errorf("Driver(%q) = %q", tc.in, got)
		}
	}
	s := Settings{DBUsername: "user", DBPassword: "p@ss w/rd", DBDatabase: "app"}
	plain := Settings{DBHost: "db", DBPort: "5433", DBUsername: "user", DBPassword: "pass", DBDatabase: "app"}
	if got, want := plain.PgDSN(), "postgres://user:pass@db:5433/app?connect_timeout=3&default_transaction_read_only=on&sslmode=prefer"; got != want {
		t.Fatalf("PgDSN = %q, want %q", got, want)
	}
	u, err := url.Parse(s.PgDSN())
	if err != nil {
		t.Fatal(err)
	}
	pass, ok := u.User.Password()
	if !ok || pass != s.DBPassword || u.Host != "127.0.0.1:5432" || u.Path != "/app" {
		t.Fatalf("PgDSN parse = %#v password=%q", u, pass)
	}
	if u.Query().Get("sslmode") != "prefer" || u.Query().Get("connect_timeout") != "3" || u.Query().Get("default_transaction_read_only") != "on" {
		t.Fatalf("PgDSN query = %q", u.RawQuery)
	}
	got := (Settings{DBHost: "::1"}).PgDSN()
	ipv6, err := url.Parse(got)
	if err != nil || ipv6.Host != "[::1]:5432" {
		t.Fatalf("IPv6 PgDSN = %q", got)
	}
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
		DBUsername:      "sail", RedisHost: "127.0.0.1", RedisPort: "6379", RabbitPort: "5672",
	}
	if got := SettingsFromEnv(env); got != want {
		t.Errorf("SettingsFromEnv() = %#v, want %#v", got, want)
	}

	if got := SettingsFromEnv(map[string]string{}); got != (Settings{RedisHost: "127.0.0.1", RedisPort: "6379", RabbitPort: "5672"}) {
		t.Errorf("SettingsFromEnv(empty) = %#v, want defaults", got)
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
		want := Settings{Root: root, LogChannel: "daily", QueueConnection: "database", DBConnection: "mysql", DBHost: "127.0.0.1", DBPort: "3306", DBDatabase: "larascope", DBUsername: "sail", RedisHost: "127.0.0.1", RedisPort: "6379", RabbitPort: "5672"}
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
