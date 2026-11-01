package conns

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type clickhouseOpts struct {
	Scheme, Host, Port, Username, Password string
	rootCAs                                *x509.CertPool
	client                                 *http.Client
}

func clickhouseCheck(name string, o clickhouseOpts) Check {
	return Check{Name: name, Target: clickhouseTarget(o), Run: func(ctx context.Context) error {
		err := clickhousePing(ctx, o)
		if err == nil {
			return nil
		}
		return fmt.Errorf("%s", redactClickHouseError(err.Error(), o))
	}}
}

// clickhousePing sends credentials when configured, although /ping itself may
// not enforce auth on the server. A proxy that returns 401 or 403 is surfaced;
// no SQL is run.
func clickhousePing(ctx context.Context, o clickhouseOpts) error {
	host, port, err := validateClickHouseTarget(o)
	if err != nil {
		return err
	}
	scheme, _ := clickhouseSchemeAndPort(o)
	u := url.URL{Scheme: scheme, Host: net.JoinHostPort(host, port), Path: "/ping"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	if req.URL.Hostname() != host || req.URL.User != nil || req.URL.Path != "/ping" || req.URL.RawQuery != "" || req.URL.Fragment != "" {
		return errors.New("invalid CLICKHOUSE_HOST")
	}
	if o.Username != "" || o.Password != "" {
		req.SetBasicAuth(o.Username, o.Password)
	}

	client := o.client
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{Timeout: 2 * time.Second}).DialContext
		transport.TLSHandshakeTimeout = 2 * time.Second
		transport.ResponseHeaderTimeout = 3 * time.Second
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: o.rootCAs}
		transport.Proxy = nil
		transport.DisableKeepAlives = true
		client = &http.Client{
			Timeout: 3 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: transport,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "Ok." {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("clickhouse authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clickhouse /ping returned HTTP %d", resp.StatusCode)
	}
	return errors.New("unexpected clickhouse /ping response")
}

func clickhouseTarget(o clickhouseOpts) string {
	host, port, err := validateClickHouseTarget(o)
	scheme, _ := clickhouseSchemeAndPort(o)
	if err != nil {
		return scheme + "://invalid"
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

// validateClickHouseTarget returns the normalized host (no brackets) and port.
func validateClickHouseTarget(o clickhouseOpts) (host, port string, err error) {
	h := o.Host
	if h == "" {
		return "", "", errors.New("invalid CLICKHOUSE_HOST")
	}

	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		inner := h[1 : len(h)-1]
		addr, parseErr := netip.ParseAddr(inner)
		if parseErr != nil || !addr.Is6() || addr.Zone() != "" {
			return "", "", errors.New("invalid CLICKHOUSE_HOST")
		}
		host = inner
	} else if addr, parseErr := netip.ParseAddr(h); parseErr == nil {
		if addr.Zone() != "" {
			return "", "", errors.New("invalid CLICKHOUSE_HOST")
		}
		host = h
	} else {
		if len(h) > 253 || strings.HasPrefix(h, "-") || strings.HasPrefix(h, ".") {
			return "", "", errors.New("invalid CLICKHOUSE_HOST")
		}
		for _, c := range h {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
				return "", "", errors.New("invalid CLICKHOUSE_HOST")
			}
		}
		labels := strings.Split(h, ".")
		for i, label := range labels {
			if label == "" && i == len(labels)-1 {
				continue
			}
			if label == "" || len(label) > 63 {
				return "", "", errors.New("invalid CLICKHOUSE_HOST")
			}
		}
		host = h
	}

	_, port = clickhouseSchemeAndPort(o)
	if port == "" {
		return "", "", errors.New("invalid CLICKHOUSE_PORT")
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return "", "", errors.New("invalid CLICKHOUSE_PORT")
		}
	}
	portNumber, parseErr := strconv.Atoi(port)
	if parseErr != nil || portNumber < 1 || portNumber > 65535 {
		return "", "", errors.New("invalid CLICKHOUSE_PORT")
	}
	return host, port, nil
}

func clickhouseSchemeAndPort(o clickhouseOpts) (string, string) {
	scheme := "http"
	if o.Scheme == "https" {
		scheme = "https"
	}
	port := o.Port
	if port == "" {
		if scheme == "https" {
			port = "8443"
		} else {
			port = "8123"
		}
	}
	return scheme, port
}

func redactClickHouseError(message string, o clickhouseOpts) string {
	if o.Password == "" {
		return message
	}
	message = strings.ReplaceAll(message, o.Password, "***")
	credentials := base64.StdEncoding.EncodeToString([]byte(o.Username + ":" + o.Password))
	return strings.ReplaceAll(message, credentials, "***")
}
