// Package proxy turns proxy links (vmess/vless/trojan/shadowsocks/http/socks)
// into real network dialers so collected configs can actually be exercised
// end to end instead of being tested with a plain direct connection.
//
// Everything here is implemented on top of the standard library: the repository
// is intentionally dependency free.
package proxy

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Dialer opens connections to a target address through a proxy.
type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)

	// Protocol reports the proxy protocol implementation in use.
	Protocol() string
}

// UnsupportedError is returned for links we cannot currently exercise, for
// example QUIC based transports or protocols that require TLS fingerprinting.
type UnsupportedError struct {
	Protocol string
	Reason   string
}

func (e *UnsupportedError) Error() string {
	if e.Protocol == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s is not testable: %s", e.Protocol, e.Reason)
}

// ErrUnsupported reports whether err describes a protocol we simply cannot test
// (as opposed to a network or configuration failure).
func ErrUnsupported(err error) bool {
	var target *UnsupportedError
	return errors.As(err, &target)
}

// NewDialer builds a dialer for a proxy link.
func NewDialer(link string) (Dialer, error) {
	scheme, _, found := strings.Cut(strings.TrimSpace(link), "://")
	if !found {
		return nil, &UnsupportedError{Reason: "not a proxy URI"}
	}
	switch strings.ToLower(scheme) {
	case "http", "https":
		return NewHTTPDialer(link)
	case "socks", "socks5":
		return NewSOCKS5Dialer(link)
	case "ss":
		return NewShadowsocksDialer(link)
	case "trojan":
		return NewTrojanDialer(link)
	case "vless":
		return NewVLESSDialer(link)
	case "vmess":
		return NewVMessDialer(link)
	case "hysteria", "hysteria2", "hy2", "tuic":
		return nil, &UnsupportedError{Protocol: scheme, Reason: "QUIC transport is not implemented"}
	case "mtproto":
		return nil, &UnsupportedError{Protocol: scheme, Reason: "Telegram MTProto proxy is not implemented"}
	default:
		return nil, &UnsupportedError{Protocol: scheme, Reason: "no client implementation"}
	}
}

// Supported reports whether the link can be turned into a dialer.
func Supported(link string) bool {
	_, err := NewDialer(link)
	return err == nil
}

// tlsOptions carries the TLS settings shared by every TLS based transport.
type tlsOptions struct {
	ServerName string
	Insecure   bool
	ALPN       []string
}

func (o tlsOptions) config() *tls.Config {
	cfg := &tls.Config{
		ServerName:         o.ServerName,
		InsecureSkipVerify: o.Insecure,
	}
	if len(o.ALPN) > 0 {
		cfg.NextProtos = o.ALPN
	}
	return cfg
}

// dialTCP opens a TCP connection honouring the context deadline.
func dialTCP(ctx context.Context, address string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	if deadline, ok := ctx.Deadline(); ok {
		d.Deadline = deadline
	}
	return d.DialContext(ctx, "tcp", address)
}

// joinHostPort formats a host/port pair.
func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// splitAddr splits a "host:port" target, defaulting to port 443 when missing.
func splitAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port") {
			return "", 0, err
		}
		return addr, 443, nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return host, port, nil
}

// trojanAddress encodes host/port the way trojan expects:
// 0x01 IPv4, 0x03 domain (length prefixed), 0x04 IPv6.
func trojanAddress(host string, port int) ([]byte, error) {
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

// v2rayAddress encodes host/port for VMess and VLESS: the port comes first,
// then the address, using 0x01 IPv4, 0x02 domain, 0x03 IPv6.
func v2rayAddress(host string, port int) ([]byte, error) {
	out := make([]byte, 0, 2+1+len(host))
	out = append(out, 0, 0)
	binary.BigEndian.PutUint16(out, uint16(port))
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			out = append(out, 0x01)
			out = append(out, v4...)
		} else {
			out = append(out, 0x03)
			out = append(out, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return nil, fmt.Errorf("domain too long: %d bytes", len(host))
		}
		out = append(out, 0x02, byte(len(host)))
		out = append(out, host...)
	}
	return out, nil
}
