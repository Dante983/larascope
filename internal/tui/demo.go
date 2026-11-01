package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"larascope/internal/conns"
	"larascope/internal/jobs"
	"larascope/internal/logs"
)

func NewDemo() Model {
	m := New().WithLogs(demoLogs(), nil)
	m.demo = true
	m.jobs.loaded = true
	m.jobs.pending, m.jobs.counts, m.jobs.failed = demoJobs()
	m.conns.results = demoConns()
	m.conns.checked = time.Date(2026, 1, 15, 9, 2, 11, 0, time.UTC)
	return m
}

func demoLogs() []logs.Entry {
	type template struct {
		level, message, frame string
		count                 int
	}
	templates := []template{
		{"ERROR", "SQLSTATE[HY000] [2002] Connection refused", "#0 /vendor/laravel/framework/src/Illuminate/Database/Connection.php(793): PDO->__construct()", 6},
		{"ERROR", "Invoice delivery failed for order #%d", "#0 /app/Jobs/SendInvoice.php(42): App\\Mail\\Invoice->send()", 5},
		{"ERROR", `Class "App\\Services\\Pdf" not found`, "#0 /app/Http/Controllers/InvoiceController.php(88): App\\Services\\Invoice->pdf()", 2},
		{"WARNING", "Slow query (%d ms): select * from orders where user_id = %d", "#0 /vendor/laravel/framework/src/Illuminate/Database/Connection.php(429): Illuminate\\Database\\Connection->select()", 7},
		{"WARNING", "Cache miss rate above threshold", "#0 /app/Monitoring/CacheMonitor.php(51): App\\Monitoring\\CacheMonitor->check()", 3},
		{"INFO", "User %d logged in from 192.168.1.%d", "#0 /app/Http/Controllers/AuthController.php(34): App\\Http\\Controllers\\AuthController->login()", 10},
		{"INFO", "Queued job App\\Jobs\\SendInvoiceEmail", "#0 /app/Jobs/DispatchInvoices.php(29): Illuminate\\Bus\\Dispatcher->dispatch()", 8},
		{"DEBUG", "Cache hit for key 'orders:%d'", "#0 /app/Cache/OrderCache.php(19): Illuminate\\Cache\\Repository->get()", 5},
	}
	base := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	// These irregular one-to-seven-minute intervals keep the whole sample in
	// the 09:00–10:40 window rather than looking like a synthetic cycle.
	gaps := []int{1, 3, 2, 1, 2, 1, 7, 2, 1, 1, 2, 1, 3, 2, 1, 2, 1, 3, 2, 1, 2, 1, 4, 2, 1, 3, 2, 1, 2, 1, 3, 2, 1, 2, 1, 5, 2, 1, 3, 1, 2, 1, 3, 2, 1, 2}
	var out []logs.Entry
	minute := 0
	for ti, x := range templates {
		for n := 0; n < x.count; n++ {
			minute += gaps[len(out)%len(gaps)]
			message := x.message
			switch ti {
			case 1:
				message = fmt.Sprintf(x.message, 1042+n*17)
			case 3:
				message = fmt.Sprintf(x.message, 842+n*31, 17+n*3)
			case 5:
				message = fmt.Sprintf(x.message, 21+n*4, 5+n)
			case 7:
				message = fmt.Sprintf(x.message, 17+n*11)
			}
			env := "local"
			if len(out)%13 == 4 || len(out)%17 == 6 {
				env = "staging"
			}
			out = append(out, logs.Entry{Header: logs.Header{Time: base.Add(time.Duration(minute) * time.Minute), Env: env, Level: x.level, Message: message}, Trace: []string{x.frame}})
		}
	}
	// Three intentionally distinct templates exercise long, Unicode, and multiline display.
	out = append(out,
		logs.Entry{Header: logs.Header{Time: base.Add(97 * time.Minute), Env: "local", Level: "INFO", Message: "Reconciliation completed: " + strings.Repeat("invoice totals matched across all ledger partitions; ", 6)}, Trace: []string{"#0 /app/Console/ReconcileLedger.php(73): App\\Console\\ReconcileLedger->handle()"}},
		logs.Entry{Header: logs.Header{Time: base.Add(99 * time.Minute), Env: "local", Level: "INFO", Message: "Imported customers 東京 from regional archive"}, Trace: []string{"#0 /app/Imports/CustomerImport.php(64): App\\Imports\\CustomerImport->run()"}},
		logs.Entry{Header: logs.Header{Time: base.Add(100 * time.Minute), Env: "staging", Level: "INFO", Message: "Webhook response body follows\nstatus=202\nqueued for later delivery"}, Trace: []string{"#0 /app/Http/WebhookController.php(112): App\\Http\\WebhookController->store()"}},
	)
	// Correlation fixtures: one UUID match, one exception match, and one time-only decoy.
	out = append(out,
		logs.Entry{Header: logs.Header{Time: base.Add(-5 * time.Second), Env: "local", Level: "ERROR", Message: `Job failed {"job":"App\\Jobs\\SendInvoiceEmail","uuid":"e5d00000-0000-4000-8000-000000000001","failed_job_id":100} []`}},
		logs.Entry{Header: logs.Header{Time: time.Date(2026, 1, 14, 7, 59, 40, 0, time.UTC), Env: "local", Level: "ERROR", Message: "Illuminate\\Database\\QueryException: Deadlock found when trying to get lock (Connection: mysql) in /app/Jobs/Worker.php:81"}},
		logs.Entry{Header: logs.Header{Time: time.Date(2026, 1, 13, 6, 59, 30, 0, time.UTC), Env: "local", Level: "WARNING", Message: "Worker retry scheduled"}},
	)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}

func demoJobs() ([]jobs.PendingRow, []jobs.QueueCount, []jobs.FailedRow) {
	base := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	type pendingFixture struct {
		queue, class, state string
		attempts            int
	}
	data := []pendingFixture{
		{"default", "App\\Jobs\\SyncInventory", "ready", 0}, {"emails", "App\\Jobs\\SendInvoiceEmail", "reserved", 2},
		{"reports", "App\\Jobs\\GenerateMonthlyReport", "delayed", 1}, {"webhooks", "App\\Jobs\\ProcessWebhook", "ready", 0},
		{"emails", "App\\Jobs\\SendInvoiceEmail", "delayed", 3}, {"default", "App\\Jobs\\GenerateMonthlyReport", "ready", 1},
		{"webhooks", "App\\Jobs\\ProcessWebhook", "reserved", 1}, {"reports", "App\\Jobs\\SyncInventory", "ready", 0},
		{"default", "App\\Jobs\\SendInvoiceEmail", "delayed", 2}, {"emails", "App\\Jobs\\ProcessWebhook", "ready", 0},
		{"reports", "App\\Jobs\\GenerateMonthlyReport", "reserved", 3}, {"webhooks", "App\\Jobs\\SyncInventory", "ready", 1},
		{"default", "App\\Jobs\\ProcessWebhook", "ready", 0}, {"emails", "App\\Jobs\\GenerateMonthlyReport", "delayed", 2},
	}
	pending := make([]jobs.PendingRow, len(data))
	counts := map[string]*jobs.QueueCount{}
	for i, x := range data {
		pending[i] = jobs.PendingRow{ID: int64(i + 1), Queue: x.queue, Class: x.class, State: x.state, Attempts: x.attempts, CreatedAt: base.Add(-time.Duration(i) * time.Minute), AvailableAt: base.Add(time.Duration(i+1) * time.Minute)}
		if x.state == "reserved" {
			r := base.Add(-time.Duration(i) * time.Minute)
			pending[i].ReservedAt = &r
		}
		if counts[x.queue] == nil {
			counts[x.queue] = &jobs.QueueCount{Queue: x.queue}
		}
		switch x.state {
		case "ready":
			counts[x.queue].Ready++
		case "delayed":
			counts[x.queue].Delayed++
		case "reserved":
			counts[x.queue].Reserved++
		}
	}
	queueNames := make([]string, 0, len(counts))
	for q := range counts {
		queueNames = append(queueNames, q)
	}
	sort.Strings(queueNames)
	queueCounts := make([]jobs.QueueCount, 0, len(queueNames))
	for _, q := range queueNames {
		queueCounts = append(queueCounts, *counts[q])
	}
	exceptions := []string{"Swift_TransportException: Connection could not be established", "Illuminate\\Database\\QueryException: Deadlock found when trying to get lock", "GuzzleHttp\\Exception\\ConnectException: cURL error 28: Connection timed out"}
	failed := make([]jobs.FailedRow, 9)
	for i := range failed {
		failed[i] = jobs.FailedRow{ID: int64(100 + i), UUID: fmt.Sprintf("e5d%d0000-0000-4000-8000-%012d", i, i+1), Connection: "database", Queue: data[(i*3+1)%len(data)].queue, Class: data[(i*5+1)%len(data)].class, FailedAt: base.AddDate(0, 0, -i).Add(-time.Duration(i) * time.Hour), Exception: exceptions[i%len(exceptions)] + "\n#0 /app/Jobs/Worker.php(81): App\\Jobs\\Worker->handle()", Payload: "{}"}
	}
	return pending, queueCounts, failed
}

func demoConns() []conns.Result {
	return []conns.Result{{Name: "database", Target: "mysql", OK: true, Latency: 4 * time.Millisecond}, {Name: "redis", Target: "redis", OK: true, Latency: time.Millisecond}, {Name: "queue", Target: "database", OK: true}, {Name: "cache", Target: "redis", OK: true, Latency: 2 * time.Millisecond}, {Name: "mail", Target: "smtp", Err: "dial tcp 127.0.0.1:1025: connection refused"}, {Name: "s3", Target: "s3", OK: true, Latency: 840 * time.Millisecond}}
}
