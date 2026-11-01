# Larascope

Terminal UI for Laravel: logs, queue jobs and service connections in one place.

Run Larascope locally from a Laravel project to inspect:

- **Logs** — parsed Laravel/Monolog log entries.
- **Jobs** — database-backed pending and failed jobs.
- **Connections** — database, Redis, and RabbitMQ health checks.

## Features

### Logs (tab 1)

Larascope reads `storage/logs/laravel.log`, falling back to the newest `storage/logs/laravel-*.log` daily log. It reads up to the last 5 MiB of the file, parses Monolog entries and multi-line stack traces, and offers an entry detail view. Missing and empty files have clear states, including `No log file found (storage/logs/laravel.log)`.

Filter levels with `l`, search text with `/`, group identical entries with counts using `s`, reload with `r`, or enable follow mode with `f`. Follow mode polls every two seconds.

### Jobs (tab 2)

Press `p` to switch between two database views:

- **Pending** reads the Laravel `jobs` table: ready, delayed, and reserved rows, including queue, class, and attempt count. It also shows per-queue and total ready/delayed/reserved counts.
- **Failed** reads `failed_jobs`: id, queue, UUID, connection, class, failure time, and the first 20 exception lines in the detail pane.

Both queries load the newest 100 rows by id. The failed-job UI marks a full result as `(first 100)`. Press `enter` for details. Optional `failed_jobs` columns are detected; a missing `failed_jobs` table is shown as an empty list. A missing `jobs` table is shown as a database error.

In the failed view only, Larascope can run these commands from the project root via `php artisan`:

| Key | Command | Confirmation |
| --- | --- | --- |
| `R` | `queue:retry <id-or-uuid>` | `y` |
| `d` | `queue:forget <id-or-uuid>` | `y` |
| `A` | `queue:retry all` | type `RETRY ALL`, then `enter` |
| `F` | `queue:flush` | type `FLUSH`, then `enter` |

Use `esc` or `q` to cancel. Job identifiers are validated as numeric ids or UUIDs. Commands time out after 60 seconds, retain at most 4,000 output bytes in the status line, and reload jobs afterward.

### Correlation

Correlation stays local and read-only. In the Jobs **Failed** view, press `L` to show likely related log entries with a confidence and reason. In a log detail view, press `J` to jump to the best matching failed job. Matching uses log-context IDs first, then conservative exception or job-class matches; time is only a supporting signal.

### Connections (tab 3)

Checks run concurrently with a three-second timeout and report latency. They auto-refresh every 10 seconds while this tab is active; `r` refreshes manually.

- **Database:** a real MySQL/MariaDB or PostgreSQL ping.
- **Redis:** a TCP dial when `REDIS_HOST` is set or `QUEUE_CONNECTION=redis`. Because Larascope defaults `REDIS_HOST` to `127.0.0.1`, this check normally always appears.
- **RabbitMQ:** a TCP dial when `RABBITMQ_HOST` is set (default port `5672`).

Redis and RabbitMQ checks verify only that the TCP port accepts connections.

## Install, build, and run

Requirements:

- Go 1.27+ (per `go.mod`)
- PHP on `PATH` for live Jobs actions
- A terminal, since Larascope uses a full-screen alternate-screen TUI

```sh
git clone <repo-url>
cd larascope
go build -o larascope ./cmd/larascope
# Or: go install ./cmd/larascope
```

Run it from a Laravel project (or any subdirectory):

```sh
cd /path/to/laravel-project
/path/to/larascope
```

Larascope walks upward from the current directory to the first directory with a regular `artisan` file. If none is found, the UI shows `Not a Laravel project (no artisan found) · try: larascope --demo`.

## CLI

The only application flag is `-demo` (`--demo` also works):

```sh
go run ./cmd/larascope -demo
```

Demo mode uses offline sample data and can run anywhere; it needs no Laravel project, database, Artisan executable, or network access. Reload, follow, and connection refresh are disabled. Job actions report `demo: no action performed`. `-h` prints Go's default usage; there is no version flag.

## Keyboard controls

| Area | Keys |
| --- | --- |
| Global | `tab` / `shift+tab` next/previous tab; `1` / `2` / `3` select tab; `?` help; `q` or `ctrl+c` quit |
| Logs | `up`/`k`, `down`/`j` move; `g`/`G` first/last in list; `pgup`/`pgdown` page; `enter` detail; `esc` back; `l` level; `/` search (`enter` apply, `esc` clear); `s` group; `f` follow; `r` reload |
| Log detail | `up`/`down` move to previous/next entry; `enter` or `esc` back; `J` matching failed job |
| Jobs | `p` pending/failed; arrows or `j`/`k` move; `enter` detail; `esc` back; `L` related logs (Failed); `r` reload; `R`, `d`, `A`, `F` actions above |
| Connections | arrows or `j`/`k` move; `r` refresh |

## Configuration

Larascope reads only `<project-root>/.env`; it does not read `config/*.php` or execute PHP to discover configuration. A missing `.env` loads no settings, but Logs still work. The parser supports quotes, `export`, inline ` #` comments, and `\n`, `\"`, and `\\` escapes in double-quoted values. It does not expand `${VAR}`.

| Key | Purpose / default |
| --- | --- |
| `DB_CONNECTION` | `mysql`, `mariadb`, `pgsql`, `postgres`, or `postgresql` |
| `DB_HOST`, `DB_PORT` | database host; defaults `127.0.0.1`, port `3306` for MySQL or `5432` for PostgreSQL |
| `DB_DATABASE`, `DB_USERNAME`, `DB_PASSWORD` | database connection settings |
| `DB_SOCKET` | parsed, but not used; Unix sockets are unsupported |
| `LOG_CHANNEL` | loaded but not used for log-file discovery |
| `QUEUE_CONNECTION` | loaded; influences the Redis health check only |
| `REDIS_HOST`, `REDIS_PORT` | Redis TCP target; defaults `127.0.0.1:6379` |
| `RABBITMQ_HOST`, `RABBITMQ_PORT` | RabbitMQ TCP target; port defaults to `5672` |

Set `LARASCOPE_PHP` to choose the PHP command. It is split on whitespace and runs with the project root as its working directory; for example, `docker compose exec app php` is passed as a host command plus arguments. The default is `php`.

## Database support

`mysql` and `mariadb` use the MySQL driver with three-second timeouts. `pgsql`, `postgres`, and `postgresql` use pgx with `sslmode=prefer`, `connect_timeout=3`, and a read-only session setting. PostgreSQL requires `DB_DATABASE` up front; MySQL/MariaDB also report `DB_DATABASE not set` when loading failed jobs if it is empty. Other values, including SQLite and SQL Server, show `unsupported DB_CONNECTION … (mysql, mariadb, pgsql supported)`.

The Jobs tab assumes Laravel's database queue tables (`jobs` and `failed_jobs`). Redis, SQS, and RabbitMQ queue drivers are not inspected; the pending view can only reflect rows in `jobs`. Pending and failed data use `SELECT` queries only; mutations occur only through the confirmed Artisan commands above.

## Safety and privacy

- Larascope reads `.env`, log files, database `SELECT`s, and `information_schema` column metadata. It never writes files.
- It never unserializes PHP payloads; it only decodes the JSON envelope to get job metadata such as the class/name. Correlation never parses serialized payload commands and runs only on already-loaded local data.
- The four confirmed Artisan commands are the only writes Larascope initiates.
- Database passwords and DSNs are redacted from displayed errors, and terminal control characters in log and job text are sanitized before rendering.
- There is no telemetry. Other than configured database, Redis, and RabbitMQ hosts, Larascope makes no network calls (demo mode makes none).

Logs and failed-job exceptions may contain sensitive data and are displayed in the terminal. Credentials from `.env` are used directly. Prefer local or development environments, and use a read-only database user where appropriate; the failed-job Artisan actions can mutate the database targeted by `.env`.

## Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| `Not a Laravel project (no artisan found)` | Run inside a Laravel project or use `-demo`. |
| `No log file found` | Create logs or check permissions for `storage/logs/laravel.log` or `laravel-*.log`. |
| Unsupported `DB_CONNECTION` | Use MySQL/MariaDB or PostgreSQL. |
| `DB_DATABASE not set` | Set `DB_DATABASE` in `.env` (required for PostgreSQL and for MySQL/MariaDB failed jobs). |
| Database connection refused or times out | Check host/port. Docker/Sail hosts such as `mysql` may not resolve from the host; use `127.0.0.1` and a forwarded port. |
| `php not found; set LARASCOPE_PHP` | Install PHP or set `LARASCOPE_PHP`. |
| Garbled layout | Enlarge the terminal and use a 256-color terminal. |
| Demo refresh says disabled | Expected: live reload/follow/refresh are unavailable in demo mode. |

## Limitations

- No SQLite, SQL Server, Unix-socket databases, `${VAR}` interpolation, config caching, or multiple connection names.
- Only the first 100 newest jobs are loaded.
- Log discovery ignores `LOG_CHANNEL`: it prefers `laravel.log`, then the newest `laravel-*.log`; other log files are not inspected.
- No mouse support, export, remote profiles, SSH, or pending-job actions. Correlation covers only the last 5 MiB log tail and first 100 failed jobs; timestamps are naive wall-clock values, so app and database timezone differences can reduce time support.
- Redis and RabbitMQ are TCP reachability checks only, not queue inspection.
- Windows is untested.

## Development and testing

Layout: `cmd/larascope` (main and flags), `internal/config` (.env, root discovery, redaction), `internal/logs`, `internal/jobs`, `internal/artisan`, `internal/conns`, and `internal/tui` (Bubble Tea model, tables, demo, styles).

```sh
go build ./...
go test ./...
go vet ./...
gofmt -l .
go run ./cmd/larascope -demo
```

The stack uses Bubble Tea, Lip Gloss, `go-sql-driver/mysql`, and pgx. Tests do not require a database or PHP.
