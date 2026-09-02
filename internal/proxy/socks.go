package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
)

// socks5Dialer implements the client side of SOCKS5 (RFC 1928).
type socks5Dialer struct {
	address  string
	username string
	password string
}

// NewSOCKS5Dialer builds a dialer for `socks://` and `socks5://` links.
func NewSOCKS5Dialer(link string) (Dialer, error) {
	parsed, err := parseLink(link)
	if err != nil {
		return nil, err
	}
	return &socks5Dialer{
		address:  parsed.Address,
		username: parsed.Username,
		password: parsed.Password,
	}, nil
}

func (d *socks5Dialer) Protocol() string { return "socks5" }

func (d *socks5Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if !strings.HasPrefix(network, "tcp") {
		return nil, fmt.Errorf("socks5 proxy supports tcp only, got %q", network)
	}

	conn, err := dialTCP(ctx, d.address)
	if err != nil {
		return nil, err
	}
	if err := d.negotiate(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := d.request(ctx, conn, addr); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// negotiate performs the method selection handshake.
func (d *socks5Dialer) negotiate(ctx context.Context, conn net.Conn) error {
	methods := []byte{0x00} // no authentication
	if d.username != "" {
		methods = append(methods, 0x02) // username/password
	}

	greeting := append([]byte{0x05, byte(len(methods))}, methods...)
	if err := writeWithContext(ctx, conn, greeting); err != nil {
		return fmt.Errorf("write greeting: %w", err)
	}

	response := make([]byte, 2)
	if _, err := readFullWithContext(ctx, conn, response); err != nil {
		return fmt.Errorf("read method selection: %w", err)
	}
	if response[0] != 0x05 {
		return fmt.Errorf("unsupported socks version: 0x%02x", response[0])
	}

	switch response[1] {
	case 0x00:
		return nil
	case 0x02:
		return d.authenticate(ctx, conn)
	case 0xFF:
		return fmt.Errorf("socks5 server rejected all authentication methods")
	default:
		return fmt.Errorf("unsupported socks5 authentication method: 0x%02x", response[1])
	}
}

// authenticate performs username/password authentication (RFC 1929).
func (d *socks5Dialer) authenticate(ctx context.Context, conn net.Conn) error {
	request := []byte{0x01, byte(len(d.username))}
	request = append(request, d.username...)
	request = append(request, byte(len(d.password)))
	request = append(request, d.password...)
	if err := writeWithContext(ctx, conn, request); err != nil {
		return fmt.Errorf("write auth request: %w", err)
	}

	response := make([]byte, 2)
	if _, err := readFullWithContext(ctx, conn, response); err != nil {
		return fmt.Errorf("read auth response: %w", err)
	}
	if response[1] != 0x00 {
		return fmt.Errorf("socks5 authentication failed: 0x%02x", response[1])
	}
	return nil
}

// request asks the proxy to connect to addr.
func (d *socks5Dialer) request(ctx context.Context, conn net.Conn, addr string) error {
	host, port, err := splitAddr(addr)
	if err != nil {
		return err
	}
	target, err := socks5Address(host, port)
	if err != nil {
		return err
	}

	request := []byte{0x05, 0x01, 0x00} // version, CONNECT, reserved
	request = append(request, target...)
	if err := writeWithContext(ctx, conn, request); err != nil {
		return fmt.Errorf("write connect request: %w", err)
	}

	header := make([]byte, 4)
	if _, err := readFullWithContext(ctx, conn, header); err != nil {
		return fmt.Errorf("read connect reply: %w", err)
	}
	if header[0] != 0x05 {
		return fmt.Errorf("unsupported socks version: 0x%02x", header[0])
	}
	if header[1] != 0x00 {
		return fmt.Errorf("socks5 connect rejected: 0x%02x (%s)", header[1], socks5Error(header[1]))
	}

	// Skip the bound address, whose shape depends on the address type.
	var skip int
	switch header[3] {
	case 0x01: // IPv4
		skip = 4
	case 0x03: // domain
		length := make([]byte, 1)
		if _, err := readFullWithContext(ctx, conn, length); err != nil {
			return fmt.Errorf("read bound address: %w", err)
		}
		skip = int(length[0])
	case 0x04: // IPv6
		skip = 16
	default:
		return fmt.Errorf("unknown socks5 address type: 0x%02x", header[3])
	}
	skip += 2 // port

	if _, err := io.CopyN(io.Discard, conn, int64(skip)); err != nil {
		return fmt.Errorf("skip bound address: %w", err)
	}
	return nil
}

func socks5Error(code byte) string {
	switch code {
	case 0x01:
		return "general failure"
	case 0x02:
		return "connection not allowed"
	case 0x03:
		return "network unreachable"
	case 0x04:
		return "host unreachable"
	case 0x05:
		return "connection refused"
	case 0x06:
		return "TTL expired"
	case 0x07:
		return "command not supported"
	case 0x08:
		return "address type not supported"
	}
	return "unknown"
}

// socks5Address encodes host/port using the classic socks5 layout:
// 0x01 IPv4, 0x03 domain, 0x04 IPv6.
func socks5Address(host string, port int) ([]byte, error) {
	out := make([]byte, 0, 1+len(host)+2)
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			out = append(out, 0x01)
			out = append(out, v4...)
		} else {
			out = append(out, 0x04)
			out = append(out, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return nil, fmt.Errorf("domain too long: %d bytes", len(host))
		}
		out = append(out, 0x03, byte(len(host)))
		out = append(out, host...)
	}
	out = append(out, 0, 0)
	binary.BigEndian.PutUint16(out[len(out)-2:], uint16(port))
	return out, nil
}
