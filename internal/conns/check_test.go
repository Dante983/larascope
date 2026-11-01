package conns

import (
	"context"
	"errors"
	"github.com/Dante983/larascope/internal/config"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRunAndFormatting(t *testing.T) {
	r := Run(context.Background(), []Check{{Name: "ok", Run: func(context.Context) error { return nil }}, {Name: "bad", Run: func(context.Context) error { return errors.New("s3cr3t failed") }}}, "s3cr3t")
	if len(r) != 2 || !r[0].OK || r[1].OK || r[1].Err != "*** failed" || r[0].Latency <= 0 {
		t.Fatalf("results = %#v", r)
	}
	for d, want := range map[time.Duration]string{0: "<1ms", 5 * time.Millisecond: "5ms", 1500 * time.Millisecond: "1.5s"} {
		if got := FormatLatency(d); got != want {
			t.Errorf("FormatLatency(%v)=%q", d, got)
		}
	}
}
func TestFromSettingsAndTCP(t *testing.T) {
	if got := FromSettings(config.Settings{}); len(got) != 0 {
		t.Fatalf("empty checks = %#v", got)
	}
	checks := FromSettings(config.Settings{DBConnection: "mysql", RedisHost: "", QueueConnection: "redis", RabbitHost: "host"})
	if len(checks) != 3 || checks[0].Name != "MySQL" || checks[1].Name != "Redis" || checks[2].Name != "RabbitMQ" {
		t.Fatalf("checks=%#v", checks)
	}
	pg := FromSettings(config.Settings{DBConnection: "pgsql", DBHost: "host"})
	if len(pg) != 1 || pg[0].Name != "PostgreSQL" || pg[0].Target != "host:5432" {
		t.Fatalf("postgres checks=%#v", pg)
	}
	if got := FromSettings(config.Settings{DBConnection: "sqlite"}); len(got) != 0 {
		t.Fatalf("sqlite checks=%#v", got)
	}
	bad := config.Settings{DBConnection: "pgsql", DBHost: "127.0.0.1", DBPort: "1", DBDatabase: "app", DBUsername: "user", DBPassword: "p@ss w/rd"}
	r := Run(context.Background(), FromSettings(bad), bad.Secrets()...)
	if len(r) != 1 || r[0].Err == "" || strings.Contains(r[0].Err, bad.DBPassword) {
		t.Fatalf("postgres result=%#v", r)
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if e := tcp("x", l.Addr().String()).Run(context.Background()); e != nil {
		t.Fatal(e)
	}
}
