package jobs

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"strings"
	"time"
)

var ErrNoTable = errors.New("jobs table not found")

type PendingRow struct {
	ID                     int64
	Queue                  string
	Attempts               int
	Class                  string
	AvailableAt, CreatedAt time.Time
	ReservedAt             *time.Time
	State                  string
}
type QueueCount struct {
	Queue                    string
	Ready, Delayed, Reserved int
}
type FailedRow struct {
	ID                       int64
	UUID                     string
	Connection, Queue, Class string
	FailedAt                 time.Time
	Exception                string
	Payload                  string
}
type Source interface {
	Pending(context.Context, int) ([]PendingRow, []QueueCount, error)
	Failed(context.Context, int) ([]FailedRow, error)
}
type dialect struct {
	countsSQL    string
	countsArgs   func() []any
	pendingSQL   string
	columnsSQL   string
	checkDBSQL   string
	failedSelect func(map[string]bool) string
}

type DBSource struct {
	db *sql.DB
	d  dialect
}

type MySQLSource = DBSource

var mysqlDialect = dialect{
	countsSQL:    "SELECT queue, SUM(reserved_at IS NOT NULL), SUM(reserved_at IS NULL AND available_at<=UNIX_TIMESTAMP()), SUM(reserved_at IS NULL AND available_at>UNIX_TIMESTAMP()) FROM jobs GROUP BY queue",
	pendingSQL:   "SELECT id, queue, attempts, reserved_at, available_at, created_at, payload FROM jobs ORDER BY id DESC LIMIT ?",
	columnsSQL:   "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='failed_jobs'",
	checkDBSQL:   "SELECT DATABASE()",
	failedSelect: func(c map[string]bool) string { return buildFailedSelectFor(c, "?") },
}

var pgDialect = dialect{
	countsSQL:    "SELECT queue, COUNT(*) FILTER (WHERE reserved_at IS NOT NULL), COUNT(*) FILTER (WHERE reserved_at IS NULL AND available_at <= $1::bigint), COUNT(*) FILTER (WHERE reserved_at IS NULL AND available_at > $1::bigint) FROM jobs GROUP BY queue",
	countsArgs:   func() []any { return []any{time.Now().Unix()} },
	pendingSQL:   "SELECT id, queue, attempts, reserved_at, available_at, created_at, payload FROM jobs ORDER BY id DESC LIMIT $1",
	columnsSQL:   "SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'failed_jobs'",
	checkDBSQL:   "SELECT current_database()",
	failedSelect: func(c map[string]bool) string { return buildFailedSelectFor(c, "$1") },
}

func openDB(driver, dsn string, d dialect) (*DBSource, error) {
	db, e := sql.Open(driver, dsn)
	if e == nil {
		db.SetMaxOpenConns(1)
	}
	return &DBSource{db: db, d: d}, e
}

func OpenMySQL(dsn string) (*DBSource, error) {
	return openDB("mysql", dsn, mysqlDialect)
}

func OpenPostgres(dsn string) (*DBSource, error) {
	return openDB("pgx", dsn, pgDialect)
}

func Open(driver, dsn string) (*DBSource, error) {
	switch driver {
	case "mysql":
		return OpenMySQL(dsn)
	case "pgsql":
		return OpenPostgres(dsn)
	default:
		return nil, errors.New("unsupported driver")
	}
}

func (s *DBSource) Close() error { return s.db.Close() }
func classify(r *int64, a, now int64) string {
	if r != nil {
		return "reserved"
	}
	if a > now {
		return "delayed"
	}
	return "ready"
}
func buildFailedSelect(c map[string]bool) string {
	return buildFailedSelectFor(c, "?")
}
func buildFailedSelectFor(c map[string]bool, placeholder string) string {
	fields := []string{"id"}
	for _, x := range []string{"uuid", "connection", "queue", "payload", "exception", "failed_at"} {
		if c[x] {
			fields = append(fields, x)
		}
	}
	return "SELECT " + strings.Join(fields, ",") + " FROM failed_jobs ORDER BY id DESC LIMIT " + placeholder
}
func (s *DBSource) Pending(ctx context.Context, limit int) ([]PendingRow, []QueueCount, error) {
	rows, e := s.db.QueryContext(ctx, s.d.pendingSQL, limit)
	if e != nil {
		return nil, nil, e
	}
	var out []PendingRow
	now := time.Now().Unix()
	for rows.Next() {
		var id int64
		var q, p string
		var at int
		var r sql.NullInt64
		var av, cr int64
		if e = rows.Scan(&id, &q, &at, &r, &av, &cr, &p); e != nil {
			return nil, nil, e
		}
		var rp *int64
		if r.Valid {
			x := r.Int64
			rp = &x
		}
		pl, _ := DecodePayload(p)
		out = append(out, PendingRow{id, q, at, pl.DisplayName, time.Unix(av, 0), time.Unix(cr, 0), func() *time.Time {
			if rp == nil {
				return nil
			}
			x := time.Unix(*rp, 0)
			return &x
		}(), classify(rp, av, now)})
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return nil, nil, e
	}
	if e = rows.Close(); e != nil {
		return nil, nil, e
	}
	var countsArgs []any
	if s.d.countsArgs != nil {
		countsArgs = s.d.countsArgs()
	}
	counts, e := s.db.QueryContext(ctx, s.d.countsSQL, countsArgs...)
	if e != nil {
		return out, nil, e
	}
	defer counts.Close()
	var cs []QueueCount
	for counts.Next() {
		var c QueueCount
		if e = counts.Scan(&c.Queue, &c.Reserved, &c.Ready, &c.Delayed); e != nil {
			return out, nil, e
		}
		cs = append(cs, c)
	}
	if e = counts.Err(); e != nil {
		return out, nil, e
	}
	return out, cs, nil
}
func (s *DBSource) Failed(ctx context.Context, limit int) ([]FailedRow, error) {
	var database sql.NullString
	if e := s.db.QueryRowContext(ctx, s.d.checkDBSQL).Scan(&database); e != nil {
		return nil, e
	}
	if !database.Valid || database.String == "" {
		return nil, errors.New("DB_DATABASE not set")
	}
	r, e := s.db.QueryContext(ctx, s.d.columnsSQL)
	if e != nil {
		return nil, e
	}
	cols := map[string]bool{}
	for r.Next() {
		var c string
		if e = r.Scan(&c); e != nil {
			r.Close()
			return nil, e
		}
		cols[c] = true
	}
	if e = r.Err(); e != nil {
		r.Close()
		return nil, e
	}
	if e = r.Close(); e != nil {
		return nil, e
	}
	if !cols["id"] {
		return nil, ErrNoTable
	}
	rows, e := s.db.QueryContext(ctx, s.d.failedSelect(cols), limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []FailedRow
	for rows.Next() {
		var f FailedRow
		var uuid, connection, queue, payload, exception sql.NullString
		var failedAt sql.NullTime
		vals := []any{&f.ID}
		for _, c := range []string{"uuid", "connection", "queue", "payload", "exception", "failed_at"} {
			if cols[c] {
				switch c {
				case "uuid":
					vals = append(vals, &uuid)
				case "connection":
					vals = append(vals, &connection)
				case "queue":
					vals = append(vals, &queue)
				case "payload":
					vals = append(vals, &payload)
				case "exception":
					vals = append(vals, &exception)
				case "failed_at":
					vals = append(vals, &failedAt)
				}
			}
		}
		if e = rows.Scan(vals...); e != nil {
			return nil, e
		}
		f.UUID, f.Connection, f.Queue = uuid.String, connection.String, queue.String
		f.Payload, f.Exception = payload.String, exception.String
		if failedAt.Valid {
			f.FailedAt = failedAt.Time
		}
		if len(f.Exception) > 4000 {
			f.Exception = f.Exception[:4000]
		}
		if p, e := DecodePayload(payload.String); e == nil {
			f.Class = p.DisplayName
		} else {
			f.Class = "?"
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
