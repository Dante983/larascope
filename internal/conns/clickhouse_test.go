package conns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func clickhouseServerOpts(t *testing.T, srv *httptest.Server) clickhouseOpts {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	return clickhouseOpts{Host: host, Port: port}
}

// unrelatedCAPool returns a pool holding a freshly generated self-signed CA
// that has no relationship to httptest's built-in certificate.
func unrelatedCAPool(t *testing.T) *x509.CertPool {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "unrelated test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return pool
}

func TestClickHousePingHealthyAndAuth(t *testing.T) {
	for _, tc := range []struct {
		name     string
		username string
		password string
	}{
		{name: "without auth"},
		{name: "with auth", username: "user", password: "pass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/ping" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				username, password, ok := r.BasicAuth()
				if tc.username == "" && ok {
					t.Errorf("unexpected Basic auth: %q:%q", username, password)
				}
				if tc.username != "" && (!ok || username != tc.username || password != tc.password) {
					t.Errorf("Basic auth = %q:%q present=%v", username, password, ok)
				}
				_, _ = w.Write([]byte("Ok.\n"))
			}))
			defer srv.Close()
			o := clickhouseServerOpts(t, srv)
			o.Username, o.Password = tc.username, tc.password
			if err := clickhousePing(context.Background(), o); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClickHousePingErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		want string
	}{
		{name: "auth failure", code: http.StatusUnauthorized, want: "authentication failed"},
		{name: "server error", code: http.StatusInternalServerError, want: "HTTP 500"},
		{name: "wrong body", code: http.StatusOK, body: "not ok", want: "unexpected"},
		{name: "redirect", code: http.StatusFound, want: "HTTP 302"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			o := clickhouseServerOpts(t, srv)
			if tc.name == "auth failure" {
				o.Password = "secret"
			}
			err := clickhouseCheck("ClickHouse", o).Run(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
			if o.Password != "" && strings.Contains(err.Error(), o.Password) {
				t.Fatalf("error leaks password: %v", err)
			}
		})
	}
}

func TestClickHousePingTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer func() {
		srv.CloseClientConnections()
		srv.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := clickhousePing(ctx, clickhouseServerOpts(t, srv))
	if err == nil || time.Since(start) >= 2*time.Second {
		t.Fatalf("error = %v after %v", err, time.Since(start))
	}
}

func TestClickHousePingTLS(t *testing.T) {
	newServer := func(t *testing.T) (*httptest.Server, *atomic.Int32) {
		t.Helper()
		var hits atomic.Int32
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			_, _ = w.Write([]byte("Ok."))
		}))
		return srv, &hits
	}

	t.Run("trusted", func(t *testing.T) {
		srv, _ := newServer(t)
		defer srv.Close()
		host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
		if err != nil {
			t.Fatal(err)
		}
		pool := x509.NewCertPool()
		pool.AddCert(srv.Certificate())
		o := clickhouseOpts{Scheme: "https", Host: host, Port: port, rootCAs: pool}
		if err := clickhousePing(context.Background(), o); err != nil {
			t.Fatalf("trusted TLS ping: %v", err)
		}
	})

	t.Run("untrusted", func(t *testing.T) {
		srv, hits := newServer(t)
		defer srv.Close()
		host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
		if err != nil {
			t.Fatal(err)
		}
		const password = "tls-secret"
		err = clickhousePing(context.Background(), clickhouseOpts{Scheme: "https", Host: host, Port: port, Username: "user", Password: password})
		if err == nil {
			t.Fatal("untrusted TLS ping succeeded")
		}
		if hits.Load() != 0 {
			t.Fatalf("handler hits = %d, want 0", hits.Load())
		}
		if strings.Contains(err.Error(), password) {
			t.Fatalf("error leaks password: %v", err)
		}
	})

	t.Run("wrong-ca", func(t *testing.T) {
		srv, hits := newServer(t)
		defer srv.Close()
		host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
		if err != nil {
			t.Fatal(err)
		}
		pool := unrelatedCAPool(t)
		if err := clickhousePing(context.Background(), clickhouseOpts{Scheme: "https", Host: host, Port: port, rootCAs: pool}); err == nil {
			t.Fatal("wrong CA TLS ping succeeded")
		}
		if hits.Load() != 0 {
			t.Fatalf("handler hits = %d, want 0", hits.Load())
		}
	})
}

func TestClickHouseCheckRedactsPassword(t *testing.T) {
	const password = "s3cr3t-pw"
	const username = "user"
	encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(password))
	}))
	defer srv.Close()
	o := clickhouseServerOpts(t, srv)
	o.Username, o.Password = username, password
	result := Run(context.Background(), []Check{clickhouseCheck("ClickHouse", o)}, password)
	if len(result) != 1 || strings.Contains(result[0].Err, password) || strings.Contains(result[0].Err, encoded) {
		t.Fatalf("result = %#v", result)
	}

	o.Port = "1"
	result = Run(context.Background(), []Check{clickhouseCheck("ClickHouse", o)}, password)
	if len(result) != 1 || strings.Contains(result[0].Err, password) || strings.Contains(result[0].Err, encoded) {
		t.Fatalf("closed-port result = %#v", result)
	}
}

func TestClickHouseTargetDefaults(t *testing.T) {
	if got := clickhouseTarget(clickhouseOpts{Host: "h"}); got != "http://h:8123" {
		t.Fatalf("HTTP target = %q", got)
	}
	if got := clickhouseTarget(clickhouseOpts{Scheme: "https", Host: "h"}); got != "https://h:8443" {
		t.Fatalf("HTTPS target = %q", got)
	}
}

func TestValidateClickHouseTarget(t *testing.T) {
	for _, tc := range []struct {
		name, host, scheme, port, wantHost, wantPort string
	}{
		{"localhost", "localhost", "", "", "localhost", "8123"},
		{"hostname", "clickhouse", "", "", "clickhouse", "8123"},
		{"domain", "ch.example.com", "", "", "ch.example.com", "8123"},
		{"trailing dot", "ch.example.com.", "", "", "ch.example.com.", "8123"},
		{"underscore", "my_host", "", "", "my_host", "8123"},
		{"IPv4", "127.0.0.1", "", "", "127.0.0.1", "8123"},
		{"IPv6", "::1", "", "", "::1", "8123"},
		{"bracketed IPv6", "[::1]", "", "", "::1", "8123"},
		{"IPv6 address", "2001:db8::1", "", "", "2001:db8::1", "8123"},
		{"bracketed IPv6 address", "[2001:db8::1]", "", "", "2001:db8::1", "8123"},
		{"HTTPS default", "ch", "https", "", "ch", "8443"},
		{"explicit port", "ch", "", "9000", "ch", "9000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, port, err := validateClickHouseTarget(clickhouseOpts{Host: tc.host, Scheme: tc.scheme, Port: tc.port})
			if err != nil || host != tc.wantHost || port != tc.wantPort {
				t.Fatalf("validateClickHouseTarget() = (%q, %q, %v), want (%q, %q, nil)", host, port, err, tc.wantHost, tc.wantPort)
			}
		})
	}

	badHosts := []string{"", "user@evil.com", "user:pass@evil.com", "evil.com/path", "evil.com?x=1", "evil.com#f", "evil.com:8123", "http://evil.com", "a b", "ev\til", "evil.com\x00", "fe80::1%eth0", "[1.2.3.4]", "[host]", "[::1", "::1]", "-bad", ".bad", "a..b", strings.Repeat("a", 64), "ex%61mple.com"}
	for _, input := range badHosts {
		t.Run("reject host", func(t *testing.T) {
			err := validateTargetError(clickhouseOpts{Host: input, Password: "secret"})
			if err == nil || (input != "" && strings.Contains(err.Error(), input)) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error = %v for host %q", err, input)
			}
		})
	}

	for _, input := range []string{"abc", "0", "65536", "-1", "80/x", "80 ", "8123@x"} {
		t.Run("reject port", func(t *testing.T) {
			err := validateTargetError(clickhouseOpts{Host: "localhost", Port: input, Password: "secret"})
			if err == nil || strings.Contains(err.Error(), input) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error = %v for port %q", err, input)
			}
		})
	}
}

func validateTargetError(o clickhouseOpts) error {
	_, _, err := validateClickHouseTarget(o)
	return err
}

func TestClickHousePingRejectsMalformedHostWithoutRequest(t *testing.T) {
	var hit atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"user@127.0.0.1", "127.0.0.1/x"} {
		t.Run(host, func(t *testing.T) {
			o := clickhouseOpts{Host: host, Port: port, Password: "secret"}
			if err := clickhousePing(context.Background(), o); err == nil {
				t.Fatal("malformed host ping succeeded")
			}
			check := clickhouseCheck("ClickHouse", o)
			if err := check.Run(context.Background()); err == nil || strings.Contains(err.Error(), o.Password) {
				t.Fatalf("check error = %v", err)
			}
			if check.Target != "http://invalid" {
				t.Fatalf("target = %q, want http://invalid", check.Target)
			}
		})
	}
	if hit.Load() {
		t.Fatal("server received a request")
	}
}

func TestClickHousePingIPv4IPv6Hostname(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Ok."))
		}))
		defer srv.Close()
		_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
		if err != nil {
			t.Fatal(err)
		}
		if err := clickhousePing(context.Background(), clickhouseOpts{Host: "127.0.0.1", Port: port}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("hostname", func(t *testing.T) {
		addresses, err := net.LookupHost("localhost")
		if err != nil {
			t.Skipf("localhost lookup failed: %v", err)
		}
		foundIPv4 := false
		for _, address := range addresses {
			if address == "127.0.0.1" {
				foundIPv4 = true
				break
			}
		}
		if !foundIPv4 {
			t.Skip("localhost does not resolve to 127.0.0.1")
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Ok."))
		}))
		defer srv.Close()
		_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
		if err != nil {
			t.Fatal(err)
		}
		if err := clickhousePing(context.Background(), clickhouseOpts{Host: "localhost", Port: port}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("IPv6", func(t *testing.T) {
		listener, err := net.Listen("tcp6", "[::1]:0")
		if err != nil {
			t.Skipf("IPv6 loopback unavailable: %v", err)
		}
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Ok."))
		}))
		srv.Listener = listener
		srv.Start()
		defer srv.Close()
		_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
		if err != nil {
			t.Fatal(err)
		}
		for _, host := range []string{"::1", "[::1]"} {
			if err := clickhousePing(context.Background(), clickhouseOpts{Host: host, Port: port}); err != nil {
				t.Fatalf("host %q: %v", host, err)
			}
		}
	})
}

func TestClickHouseTargetFormatting(t *testing.T) {
	for _, tc := range []struct {
		o    clickhouseOpts
		want string
	}{
		{clickhouseOpts{Host: "::1"}, "http://[::1]:8123"},
		{clickhouseOpts{Host: "[::1]"}, "http://[::1]:8123"},
		{clickhouseOpts{Scheme: "https", Host: "ch"}, "https://ch:8443"},
		{clickhouseOpts{Host: "evil.com/path"}, "http://invalid"},
	} {
		if got := clickhouseTarget(tc.o); got != tc.want {
			t.Fatalf("clickhouseTarget(%+v) = %q, want %q", tc.o, got, tc.want)
		}
	}
}

func TestClickHousePingDoesNotFollowRedirect(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusMovedPermanently, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var destinationHit atomic.Bool
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				destinationHit.Store(true)
				_, _ = w.Write([]byte("Ok."))
			}))
			defer destination.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", destination.URL)
				w.WriteHeader(status)
			}))
			defer origin.Close()
			err := clickhousePing(context.Background(), clickhouseServerOpts(t, origin))
			if err == nil || !strings.Contains(err.Error(), "HTTP "+strconv.Itoa(status)) {
				t.Fatalf("error = %v, want HTTP %d", err, status)
			}
			if destinationHit.Load() {
				t.Fatal("redirect destination received a request")
			}
		})
	}
}
