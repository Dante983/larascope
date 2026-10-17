package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	Root                   string
	LogChannel             string
	QueueConnection        string
	DBConnection           string
	DBHost                 string
	DBPort                 string
	DBDatabase             string
	DBUsername             string
	DBPassword             string
	DBSocket               string
	RedisHost, RedisPort   string
	RabbitHost, RabbitPort string
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
		DBUsername:      env["DB_USERNAME"], DBPassword: env["DB_PASSWORD"], DBSocket: env["DB_SOCKET"],
		RedisHost: valueOr(env["REDIS_HOST"], "127.0.0.1"), RedisPort: valueOr(env["REDIS_PORT"], "6379"),
		RabbitHost: env["RABBITMQ_HOST"], RabbitPort: valueOr(env["RABBITMQ_PORT"], "5672"),
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

func valueOr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
func (s Settings) DSN() string {
	host, port := valueOr(s.DBHost, "127.0.0.1"), valueOr(s.DBPort, "3306")
	return s.DBUsername + ":" + s.DBPassword + "@tcp(" + host + ":" + port + ")/" + s.DBDatabase + "?timeout=3s&readTimeout=3s&parseTime=true"
}
func Redact(v string, secrets ...string) string {
	for _, x := range secrets {
		if x != "" {
			v = strings.ReplaceAll(v, x, "***")
			v = strings.ReplaceAll(v, url.QueryEscape(x), "***")
		}
	}
	return v
}
