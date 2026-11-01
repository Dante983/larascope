package conns

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type redisOpts struct {
	Addr, Username, Password string
	DB                       int
	TLS                      bool
	ServerName               string
	rootCAs                  *x509.CertPool
}

func redisCheck(name string, o redisOpts) Check {
	return Check{Name: name, Target: o.Addr, Run: func(ctx context.Context) error {
		err := redisPing(ctx, o)
		if err != nil && o.Password != "" {
			return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), o.Password, "***"))
		}
		return err
	}}
}

func redisPing(ctx context.Context, o redisOpts) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", o.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	if o.TLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: o.ServerName, MinVersion: tls.VersionTLS12, RootCAs: o.rootCAs})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = tlsConn
	}

	deadline := time.Now().Add(3 * time.Second)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	r := bufio.NewReaderSize(conn, 4096)
	if o.Password != "" {
		args := []string{"AUTH", o.Password}
		if o.Username != "" {
			args = []string{"AUTH", o.Username, o.Password}
		}
		if err := writeCmd(conn, args...); err != nil {
			return err
		}
		if reply, err := readReply(r); err != nil {
			return err
		} else if reply != "OK" {
			return fmt.Errorf("unexpected AUTH reply")
		}
	}
	if o.DB > 0 {
		if err := writeCmd(conn, "SELECT", strconv.Itoa(o.DB)); err != nil {
			return err
		}
		if reply, err := readReply(r); err != nil {
			return err
		} else if reply != "OK" {
			return fmt.Errorf("unexpected SELECT reply")
		}
	}
	if err := writeCmd(conn, "PING"); err != nil {
		return err
	}
	reply, err := readReply(r)
	if err != nil {
		return err
	}
	if reply != "PONG" {
		return fmt.Errorf("unexpected PING reply")
	}
	return nil
}

func writeCmd(w io.Writer, args ...string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

func readReply(r *bufio.Reader) (string, error) {
	line, err := r.ReadSlice('\n')
	if err != nil {
		return "", err
	}
	line = bytesTrimSuffix(line, '\n')
	line = bytesTrimSuffix(line, '\r')
	if len(line) == 0 {
		return "", fmt.Errorf("unexpected redis reply %q", "")
	}
	switch line[0] {
	case '-':
		return "", fmt.Errorf("redis error: %s", line[1:])
	case '+':
		return string(line[1:]), nil
	default:
		return "", fmt.Errorf("unexpected redis reply %q", truncate(string(line), 64))
	}
}

func bytesTrimSuffix(b []byte, c byte) []byte {
	if len(b) > 0 && b[len(b)-1] == c {
		return b[:len(b)-1]
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
