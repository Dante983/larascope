package conns

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func startFakeRedis(t *testing.T, handler func(net.Conn)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				handler(conn)
			}()
		}
	}()
	return l.Addr().String()
}

func readCmd(t *testing.T, r *bufio.Reader) []string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "*") {
		t.Fatalf("RESP array header %q: %v", line, err)
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(line[1:]), "%d", &n); err != nil {
		t.Fatal(err)
	}
	args := make([]string, n)
	for i := range args {
		line, err = r.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "$") {
			t.Fatalf("RESP bulk header %q: %v", line, err)
		}
		var size int
		if _, err := fmt.Sscanf(strings.TrimSpace(line[1:]), "%d", &size); err != nil {
			t.Fatal(err)
		}
		data := make([]byte, size+2)
		if _, err := io.ReadFull(r, data); err != nil {
			t.Fatal(err)
		}
		args[i] = string(data[:size])
	}
	return args
}

func TestRedisPing(t *testing.T) {
	tests := []struct {
		name    string
		opts    redisOpts
		server  func(net.Conn)
		want    [][]string
		wantErr string
	}{
		{"ping only", redisOpts{}, func(c net.Conn) { r := bufio.NewReader(c); _ = readCmd(t, r); _, _ = c.Write([]byte("+PONG\r\n")) }, [][]string{{"PING"}}, ""},
		{"password", redisOpts{Password: "secret"}, func(c net.Conn) {
			r := bufio.NewReader(c)
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("+OK\r\n"))
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("+PONG\r\n"))
		}, [][]string{{"AUTH", "secret"}, {"PING"}}, ""},
		{"username password", redisOpts{Username: "user", Password: "secret"}, func(c net.Conn) {
			r := bufio.NewReader(c)
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("+OK\r\n"))
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("+PONG\r\n"))
		}, [][]string{{"AUTH", "user", "secret"}, {"PING"}}, ""},
		{"database", redisOpts{Password: "secret", DB: 2}, func(c net.Conn) {
			r := bufio.NewReader(c)
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("+OK\r\n"))
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("+OK\r\n"))
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("+PONG\r\n"))
		}, [][]string{{"AUTH", "secret"}, {"SELECT", "2"}, {"PING"}}, ""},
		{"wrong password", redisOpts{Password: "secret"}, func(c net.Conn) {
			r := bufio.NewReader(c)
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("-WRONGPASS invalid\r\n"))
		}, nil, "redis error"},
		{"HTTP", redisOpts{}, func(c net.Conn) {
			r := bufio.NewReader(c)
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("HTTP/1.1 400\r\n"))
		}, nil, "unexpected redis reply"},
		{"not PONG", redisOpts{}, func(c net.Conn) { r := bufio.NewReader(c); _ = readCmd(t, r); _, _ = c.Write([]byte("+OK\r\n")) }, nil, "unexpected PING reply"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr := startFakeRedis(t, func(c net.Conn) {
				tc.server(c)
			})
			tc.opts.Addr = addr
			err := redisPing(context.Background(), tc.opts)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error=%v, want %q", err, tc.wantErr)
			}
		})
	}
	addr := startFakeRedis(t, func(net.Conn) {})
	if err := redisPing(context.Background(), redisOpts{Addr: addr}); err == nil {
		t.Fatal("expected error from immediately closed server")
	}
}

func TestRedisTLS(t *testing.T) {
	cert, pool := testCertificate(t)
	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				if _, err := r.ReadString('\n'); err != nil {
					return
				}
				_, _ = conn.Write([]byte("+PONG\r\n"))
			}()
		}
	}()

	o := redisOpts{Addr: l.Addr().String(), TLS: true, ServerName: "127.0.0.1"}
	if err := redisPing(context.Background(), o); err == nil {
		t.Fatal("expected unknown-authority error")
	}
	o.rootCAs = pool
	if err := redisPing(context.Background(), o); err != nil {
		t.Fatalf("TLS PING: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := redisPing(ctx, redisOpts{Addr: l.Addr().String()}); err == nil {
		t.Fatal("expected plain TCP client error against TLS server")
	}
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	return cert, pool
}

func TestRedisErrorsAndTimeouts(t *testing.T) {
	t.Run("password is redacted by Run", func(t *testing.T) {
		addr := startFakeRedis(t, func(c net.Conn) {
			r := bufio.NewReader(c)
			_ = readCmd(t, r)
			_, _ = c.Write([]byte("-WRONGPASS secret\r\n"))
		})
		results := Run(context.Background(), []Check{redisCheck("Redis", redisOpts{Addr: addr, Password: "secret"})}, "secret")
		if results[0].OK || !strings.Contains(results[0].Err, "redis error") || strings.Contains(results[0].Err, "secret") {
			t.Fatalf("results=%#v", results)
		}
	})
	t.Run("hung server", func(t *testing.T) {
		addr := startFakeRedis(t, func(net.Conn) { time.Sleep(time.Second) })
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		started := time.Now()
		results := Run(ctx, []Check{redisCheck("Redis", redisOpts{Addr: addr})})
		if results[0].OK || time.Since(started) >= 2*time.Second {
			t.Fatalf("results=%#v elapsed=%s", results, time.Since(started))
		}
	})
	t.Run("connection refused", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		if err := redisPing(context.Background(), redisOpts{Addr: addr}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestWriteCmd(t *testing.T) {
	var b bytes.Buffer
	if err := writeCmd(&b, "AUTH", "a b"); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), "*2\r\n$4\r\nAUTH\r\n$3\r\na b\r\n"; got != want {
		t.Fatalf("writeCmd=%q, want %q", got, want)
	}
}
