# larascope: Laravel Ops TUI (working name)

An all-in-one terminal UI for Laravel development: **logs, jobs, queues and connections in one place**, with correlation between them. Local-first, with optional read-only profiles for other environments.

Assumed stack: **Go + Bubble Tea** (Bubbles, Lip Gloss). Single static binary, run from a Laravel project root.

---

## 1. Goals

- Replace `tail -f laravel.log` and ad-hoc SQL on `jobs` / `failed_jobs` with one interactive tool.
- Make local dev faster: see errors grouped, see queue state, retry failed jobs, check that services are up.
- Be able to point the same tool at another environment (dev/staging/prod) for testing and monitoring, **read-only by default**.
- Unique angle: **correlation**. A log entry links to the job that threw it, and a failed job links back to its log lines.

## 2. Non-goals (for now)

- Not a web dashboard, not a Horizon replacement.
- No SSH tunnelling or remote log shipping in v1.
- No writing to production by default.

---

## 3. Findings about the target environment

| Item | Finding |
|---|---|
| Framework | Laravel **11.25.0** (modern `failed_jobs` schema, payload format, `queue:retry` works normally) |
| Local queue driver | **database** (`jobs` table in MySQL) |
| Production queue driver | **RabbitMQ** |
| Queue code | Only `php-amqplib` installed. Custom RabbitMQ driver **vendored in the app**: `app/Extensions/Queue/RabbitMQ/` (Connector, Queue, Job, ServiceProvider) plus a custom `app/Extensions/Queue/Worker.php` |
| Failed jobs | `failed_jobs` table in **MySQL** (`config/queue.php` failed block) |
| Other AMQP usage | Raw (non-job) messages: `RabbitMQListener`, `TranscriptionRabbitMQService`, `ConsumeTranscriptionResults`, Metis / LaunchMoreBrands services |
| Local RabbitMQ | Docker container, vhost `/`, **currently empty** (no queues/bindings) because local uses the database driver |
| Config style | Legacy-looking `config/queue.php` (`login` key, `QUEUE_DRIVER` env names) on a modern Laravel |
| Production access | **No SSH.** Only network/API access if reachable (private LAN host, likely VPN) |
| Log files | Unknown: `single` vs `daily` still to confirm |

### Security follow-up (separate from the tool)

`config/queue.php` contains a real-looking default RabbitMQ password and host, and the same appears in commented `.env` lines. Rotate that credential and remove secrets from config defaults.

---

## 4. Existing tools (for reference)

| Area | Tool | Notes |
|---|---|---|
| RabbitMQ TUI | rabbitui (Rust) | Management API client; original appears unmaintained, a fork exists, early stage |
| RabbitMQ TUI | rbtop (Go) | htop-style: browse, purge, delete, peek, Docker control |
| RabbitMQ TUI | mqtop (Python) | Queue depth and rates, Kubernetes port-forward helper |
| Message bus | buswatch (Rust) | Aimed at a specific bus setup, not general admin |
| Laravel logs TUI | vtail (Solo) | Tail with vendor-frame collapsing; v0.1.0, very new |
| Laravel dev TUI | Solo for Laravel | Runs Vite, logs, queues in tabs; not a log/queue analyzer |
| Laravel logs web | OPcodes Log Viewer | Browser UI, not terminal |

**Gap we target:** exception grouping with counts, failed-job triage with retry, queue + log correlation, DLQ/message inspection, all in one tool. Verify competitors' READMEs/issues before investing heavily in RabbitMQ features.

---

## 5. Architecture

```
larascope/
  cmd/larascope/        main.go, flags
  internal/
    config/             profiles, .env discovery, larascope.toml
    tui/                root model, tab bar, shared styles, help overlay
    logs/               tailer, parser, fingerprinter, trace renderer
    jobs/               JobSource interface + implementations
    queues/             QueueSource interface + RabbitMQ implementation
    conns/              health checks (MySQL, RabbitMQ, Redis, ClickHouse, Docker)
    artisan/            wrapper to run `php artisan ...` safely
    correlate/          links between log entries, jobs and queues
```

### Key interfaces

- `FailedJobSource`: list / get / delete / retry (MySQL `failed_jobs`; detect available columns instead of assuming)
- `PendingJobSource`: counts and listing for ready / delayed / reserved (database driver first)
- `QueueSource`: depth, unacked, rates, peek, move (RabbitMQ Management API)
- `HealthCheck`: name, check(), latency, status

### Profiles

- `local` is auto-detected from `.env` in the current directory.
- Extra profiles (`dev`, `staging`, `prod`) live in `larascope.toml`: host, credentials (env var references, not plaintext), and a `read_only` flag.
- Switch profile with a keypress. Remote profiles default to **read-only**; write actions are disabled unless explicitly enabled per profile.

### Design rules

- **Retry through `php artisan queue:retry`**, never by hand-rebuilding payloads. Use direct re-publish only for DLQ moves between RabbitMQ queues.
- **Never unserialize PHP.** Decode the JSON envelope (`displayName`, `job`, `attempts`, `data`) and show the serialized command as text.
- **Handle two message kinds** in RabbitMQ: Laravel job payloads (rendered nicely) and raw custom messages (pretty JSON or raw text).
- **Empty states are first-class** (no queues, no failed jobs, no log file).
- **Confirm before destructive actions** (retry all, delete, purge, move).
- Fingerprint exceptions by class + file:line + normalized message (strip IDs, numbers, UUIDs).

---

## 6. Feature spec by tab

### 6.1 Logs

- Tail `storage/logs/*.log`; support `single` and `daily`; pick up rotation
- Parse Monolog line format: `[timestamp] channel.LEVEL: message {context} {extra}`
- Join multi-line stack traces to their entry
- Group identical exceptions with counts (`QueryException x214`), expandable
- Filters: level, channel, text search, time window
- Collapse vendor frames in traces (toggle)
- Follow mode, pause, jump to top/bottom
- Open an entry to see the full trace and context JSON
- Stretch: jump to related job when context contains a job id

### 6.2 Jobs

- **Pending (database driver):** per-queue counts for ready / delayed / reserved, attempts, available_at
- **Failed (MySQL `failed_jobs`):** class, queue, connection, failed_at, exception, decoded payload
- Actions: retry one / selected / all of a class, delete one / selected / flush (all behind confirm and read-only profile checks)
- Detect `failed_jobs` columns (`uuid`, `exception`) rather than hardcoding

### 6.3 Queues (RabbitMQ, optional)

- Queue list: ready, unacked, consumers, rates (Management API)
- Peek messages: headers, properties, `x-death`, body
- DLQ replay: move messages from a dead-letter queue back to the source queue
- Purge with confirmation
- Needs the management plugin reachable (usually port 15672); store its URL separately from the AMQP port

### 6.4 Connections

- Ping: MySQL, RabbitMQ (AMQP + Management API), Redis, ClickHouse, with latency and up/down
- Read targets from the profile / `.env`
- Optional: Docker containers and listening ports, MySQL running queries, ClickHouse running queries
- Ideas to borrow: see how `app/Services/HealthcheckService.php` already checks services

### 6.5 Correlation (later)

- Log entry to job (by job id / uuid in context)
- Failed job to its log lines (by time window + exception match)
- Job to queue (by queue name)

---

## 7. Build order

### Phase 0: Prep (before coding)
- [ ] Confirm log setup: `single` vs `daily`
- [ ] Get 3+ sample `laravel.log` entries (normal, error with stack trace, anything with JSON context or a custom formatter)
- [ ] Confirm which env var `config/queue.php` `'default'` reads
- [ ] Read `RabbitMQQueue.php`, `RabbitMQJob.php`, `Worker.php` (naming, delay handling, failure/reject behavior)
- [ ] Generate local RabbitMQ test traffic (throwaway job dispatched with `->onConnection('rabbitmq')`, some failing)
- [ ] Confirm management plugin and port 15672 are available locally
- [ ] Rotate the exposed RabbitMQ credential

### Phase 1: Skeleton
- [ ] Go module, Bubble Tea root model, tab bar, help overlay
- [ ] Config: `.env` discovery, `larascope.toml` profiles, read-only flag
- [ ] Shared styles, key bindings, empty-state components

### Phase 2: Logs tab
- [ ] File discovery and tailer (single + daily, rotation)
- [ ] Monolog parser incl. multi-line traces, context JSON
- [ ] Fingerprinting and grouping
- [ ] List view, detail view, filters, search, follow mode
- [ ] Vendor-frame collapsing
- [ ] Tests against real sample entries

### Phase 3: Jobs tab (database source)
- [ ] MySQL connection from `.env`
- [ ] `jobs` pending view (ready / delayed / reserved)
- [ ] `failed_jobs` list and detail with payload decoding and column detection
- [ ] Retry / delete via `artisan` wrapper, with confirmation

### Phase 4: Connections tab
- [ ] Health check interface and checks for MySQL, RabbitMQ, Redis, ClickHouse
- [ ] Latency display and auto-refresh
- [ ] Optional Docker/ports panel

### Phase 5: Queues tab (RabbitMQ)
- [ ] Management API client (list queues, rates)
- [ ] Peek with Laravel-job vs raw-message rendering
- [ ] Purge with confirmation
- [ ] DLQ replay (once queue/DLX naming is understood from the driver)

### Phase 6: Remote profiles and polish
- [ ] Profile switching, read-only enforcement everywhere
- [ ] Error handling for unreachable hosts and timeouts
- [ ] Config docs, README, release builds

### Phase 7: Correlation
- [ ] Link log entries, failed jobs and queues
- [ ] Cross-tab navigation keys

---

## 8. Risks and open questions

- **Custom driver behavior** is unknown until `RabbitMQQueue.php` / `RabbitMQJob.php` / `Worker.php` are reviewed; DLQ replay depends on it.
- **Production reachability:** private LAN host; remote use needs VPN and read-only users (RabbitMQ `monitoring` tag, read-only MySQL user).
- **Production logs** cannot be read without file access; remote logs are out of scope for v1.
- **Retry through artisan** requires PHP available where the tool runs (local: fine; Docker: may need `docker compose exec`). Decide how the wrapper locates PHP.
- **Competitors** may add features; check before investing heavily in the RabbitMQ tab.

## 9. Definition of done for v1

Logs, database-backed Jobs, and Connections tabs working against local dev, with read-only remote profile support for MySQL and RabbitMQ health checks. RabbitMQ Queues tab and correlation follow as v1.x.
