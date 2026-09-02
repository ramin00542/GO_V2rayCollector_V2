package proxy

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

// trojanDialer implements the trojan protocol: TLS (usually) followed by a
// password authenticated, socks5 style request header.
type trojanDialer struct {
	address  string
	password string
	security string
	sni      string
	insecure bool
	network  string
	wsPath   string
	wsHost   string
}

// NewTrojanDialer builds a dialer for `trojan://` links.
func NewTrojanDialer(link string) (Dialer, error) {
	parsed, err := parseLink(link)
	if err != nil {
		return nil, err
	}
	if parsed.Username == "" {
		return nil, fmt.Errorf("trojan link has no password")
	}

	security := strings.ToLower(firstNonEmpty(parsed.param("security"), "tls"))
	if security == "reality" {
		return nil, &UnsupportedError{Protocol: "trojan", Reason: "REALITY requires TLS fingerprinting which is not implemented"}
	}
	network := strings.ToLower(firstNonEmpty(parsed.param("type"), "tcp"))
	if network != "tcp" && network != "ws" {
		return nil, &UnsupportedError{Protocol: "trojan", Reason: fmt.Sprintf("transport %q is not implemented", network)}
	}

	d := &trojanDialer{
		address:  parsed.Address,
		password: parsed.Username,
		security: security,
		sni:      firstNonEmpty(parsed.param("sni"), parsed.param("peer"), parsed.Host),
		insecure: parsed.boolParam("allowInsecure", false),
		network:  network,
		wsPath:   parsed.param("path"),
		wsHost:   firstNonEmpty(parsed.param("host"), parsed.Host),
	}
	return d, nil
}

func (d *trojanDialer) Protocol() string { return "trojan" }

func (d *trojanDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if !strings.HasPrefix(network, "tcp") || network != "tcp" {
		return nil, fmt.Errorf("trojan proxy supports tcp only, got %q", network)
	}
	conn, err := dialTCP(ctx, d.address)
	if err != nil {
		return nil, err
	}

	if d.security == "tls" {
		options := tlsOptions{ServerName: d.sni, Insecure: d.insecure}
		tlsConn := tls.Client(conn, options.config())
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("tls handshake: %w", err)
		}
		conn = tlsConn
	}

	if d.network == "ws" {
		conn, err = wsHandshake(ctx, conn, wsOptions{Host: d.wsHost, Path: d.wsPath})
		if err != nil {
			conn.Close()
			return nil, err
		}
	}

	if err := d.writeRequest(ctx, conn, addr); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (d *trojanDialer) writeRequest(ctx context.Context, conn net.Conn, addr string) error {
	host, port, err := splitAddr(addr)
	if err != nil {
		return err
	}
	target, err := trojanAddress(host, port)
	if err != nil {
		return err
	}

	// The password is sent as hex(sha224(password)).
	digest := sha256.Sum224([]byte(d.password))
	request := make([]byte, 0, hex.EncodedLen(len(digest))+len(target)+8)
	request = append(request, hex.EncodeToString(digest[:])...)
	request = append(request, '\r', '\n')
	request = append(request, 0x01) // CONNECT
	request = append(request, target...)
	request = append(request, '\r', '\n')

	return writeWithContext(ctx, conn, request)
}
