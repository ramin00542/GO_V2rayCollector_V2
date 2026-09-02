package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// httpDialer tunnels connections through an HTTP proxy using CONNECT.
// When the link itself is https, the proxy connection is wrapped in TLS first.
type httpDialer struct {
	address  string
	username string
	password string
	tls      *tlsOptions
}

// NewHTTPDialer builds a dialer for `http://` and `https://` proxy links.
func NewHTTPDialer(link string) (Dialer, error) {
	parsed, err := parseLink(link)
	if err != nil {
		return nil, err
	}
	d := &httpDialer{
		address:  parsed.Address,
		username: parsed.Username,
		password: parsed.Password,
	}
	if parsed.Scheme == "https" {
		d.tls = &tlsOptions{
			ServerName: firstNonEmpty(parsed.param("sni"), parsed.Host),
			Insecure:   parsed.boolParam("allowInsecure", false),
		}
	}
	return d, nil
}

func (d *httpDialer) Protocol() string { return "http" }

func (d *httpDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if !strings.HasPrefix(network, "tcp") {
		return nil, fmt.Errorf("http proxy supports tcp only, got %q", network)
	}

	conn, err := dialTCP(ctx, d.address)
	if err != nil {
		return nil, err
	}
	if d.tls != nil {
		tlsConn := tls.Client(conn, d.tls.config())
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("tls handshake with proxy: %w", err)
		}
		conn = tlsConn
	}

	if err := d.connect(ctx, conn, addr); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (d *httpDialer) connect(ctx context.Context, conn net.Conn, addr string) error {
	var request strings.Builder
	request.WriteString("CONNECT " + addr + " HTTP/1.1\r\n")
	request.WriteString("Host: " + addr + "\r\n")
	if d.username != "" {
		credentials := d.username
		if d.password != "" {
			credentials += ":" + d.password
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
		request.WriteString("Proxy-Authorization: Basic " + encoded + "\r\n")
	}
	request.WriteString("Proxy-Connection: keep-alive\r\n\r\n")

	if err := writeWithContext(ctx, conn, []byte(request.String())); err != nil {
		return fmt.Errorf("write CONNECT request: %w", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := readLineWithContext(ctx, conn, reader)
	if err != nil {
		return fmt.Errorf("read CONNECT response: %w", err)
	}

	fields := strings.Fields(statusLine)
	if len(fields) < 2 {
		return fmt.Errorf("malformed CONNECT response: %q", statusLine)
	}
	statusCode, err := strconv.Atoi(fields[1])
	if err != nil {
		return fmt.Errorf("malformed CONNECT status: %q", statusLine)
	}

	// Consume response headers.
	for {
		line, err := readLineWithContext(ctx, conn, reader)
		if err != nil {
			return fmt.Errorf("read CONNECT headers: %w", err)
		}
		if line == "" {
			break
		}
	}

	if statusCode < 200 || statusCode > 299 {
		return fmt.Errorf("proxy refused CONNECT: %s", strings.TrimSpace(statusLine))
	}
	return nil
}
