package conns

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Dante983/larascope/internal/config"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Result struct {
	Name, Target string
	OK           bool
	Latency      time.Duration
	Err          string
}
type Check struct {
	Name, Target string
	Run          func(context.Context) error
}

func Run(ctx context.Context, cs []Check, secrets ...string) []Result {
	out := make([]Result, len(cs))
	var wg sync.WaitGroup
	for i, c := range cs {
		wg.Add(1)
		go func(i int, c Check) {
			defer wg.Done()
			x, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			st := time.Now()
			e := c.Run(x)
			out[i] = Result{c.Name, c.Target, e == nil, time.Since(st), config.Redact(errString(e), secrets...)}
		}(i, c)
	}
	wg.Wait()
	return out
}
func FormatLatency(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
func errString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
func FromSettings(s config.Settings) []Check {
	var out []Check
	switch s.Driver() {
	case "mysql":
		out = append(out, dbCheck("MySQL", "mysql", s.DSN(), configHost(s.DBHost, "127.0.0.1")+":"+configHost(s.DBPort, "3306")))
	case "pgsql":
		out = append(out, dbCheck("PostgreSQL", "pgx", s.PgDSN(), configHost(s.DBHost, "127.0.0.1")+":"+configHost(s.DBPort, "5432")))
	}
	redis := s.RedisHost != "" || s.QueueConnection == "redis"
	if redis {
		db, _ := strconv.Atoi(strings.TrimSpace(s.RedisDB))
		if db < 0 {
			db = 0
		}
		host := configHost(s.RedisHost, "127.0.0.1")
		out = append(out, redisCheck("Redis", redisOpts{
			Addr:       net.JoinHostPort(host, configHost(s.RedisPort, "6379")),
			Username:   s.RedisUsername,
			Password:   s.RedisPassword,
			DB:         db,
			TLS:        s.RedisTLS(),
			ServerName: host,
		}))
	}
	if s.ClickHouseHost != "" {
		scheme := "http"
		if s.ClickHouseTLS() {
			scheme = "https"
		}
		out = append(out, clickhouseCheck("ClickHouse", clickhouseOpts{
			Scheme: scheme, Host: s.ClickHouseHost, Port: s.ClickHousePort,
			Username: s.ClickHouseUsername, Password: s.ClickHousePassword,
		}))
	}
	if s.RabbitHost != "" {
		out = append(out, tcp("RabbitMQ", s.RabbitHost+":"+configHost(s.RabbitPort, "5672")))
	}
	return out
}
func dbCheck(name, driver, dsn, target string) Check {
	return Check{name, target, func(c context.Context) error {
		d, e := sql.Open(driver, dsn)
		if e != nil {
			return e
		}
		defer d.Close()
		return d.PingContext(c)
	}}
}
func configHost(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
func tcp(n, t string) Check {
	return Check{n, t, func(c context.Context) error {
		d := net.Dialer{}
		x, e := d.DialContext(c, "tcp", t)
		if e == nil {
			x.Close()
		}
		return e
	}}
}
