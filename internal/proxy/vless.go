package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
)

// vlessDialer implements the VLESS protocol for the tcp and ws transports.
type vlessDialer struct {
	address  string
	uuid     [16]byte
	security string
	sni      string
	insecure bool
	network  string
	wsPath   string
	wsHost   string
	flow     string
}

// NewVLESSDialer builds a dialer for `vless://` links.
func NewVLESSDialer(link string) (Dialer, error) {
	parsed, err := parseLink(link)
	if err != nil {
		return nil, err
	}
	uuid, err := normalizeUUID(parsed.Username)
	if err != nil {
		return nil, err
	}

	if err := validateVLESSParams(parsed); err != nil {
		return nil, err
	}

	d := &vlessDialer{
		address:  parsed.Address,
		uuid:     uuid,
		security: strings.ToLower(firstNonEmpty(parsed.param("security"), "none")),
		sni:      firstNonEmpty(parsed.param("sni"), parsed.param("peer"), parsed.Host),
		insecure: parsed.boolParam("allowInsecure", false),
		network:  strings.ToLower(firstNonEmpty(parsed.param("type"), "tcp")),
		wsPath:   parsed.param("path"),
		wsHost:   firstNonEmpty(parsed.param("host"), parsed.Host),
		flow:     strings.ToLower(parsed.param("flow")),
	}
	return d, nil
}

// validateVLESSParams rejects transports we cannot implement up front so
// callers can skip a config without opening a connection.
func validateVLESSParams(parsed *Link) error {
	security := strings.ToLower(firstNonEmpty(parsed.param("security"), "none"))
	switch security {
	case "reality":
		return &UnsupportedError{Protocol: "vless", Reason: "REALITY requires TLS fingerprinting which is not implemented"}
	case "", "none", "tls":
	default:
		return &UnsupportedError{Protocol: "vless", Reason: fmt.Sprintf("security %q is not implemented", security)}
	}
	network := strings.ToLower(firstNonEmpty(parsed.param("type"), "tcp"))
	if network != "tcp" && network != "raw" && network != "ws" {
		return &UnsupportedError{Protocol: "vless", Reason: fmt.Sprintf("transport %q is not implemented", network)}
	}
	if flow := strings.ToLower(parsed.param("flow")); flow != "" {
		return &UnsupportedError{Protocol: "vless", Reason: fmt.Sprintf("flow %q (XTLS) is not implemented", flow)}
	}
	return nil
}

func (d *vlessDialer) Protocol() string { return "vless" }

func validateSecurityAndNetwork(security, network, flow string) error {
	switch security {
	case "reality":
		return &UnsupportedError{Protocol: "vless", Reason: "REALITY requires TLS fingerprinting which is not implemented"}
	case "", "none", "tls":
	default:
		return &UnsupportedError{Protocol: "vless", Reason: fmt.Sprintf("security %q is not implemented", security)}
	}
	if network != "tcp" && network != "raw" && network != "ws" {
		return &UnsupportedError{Protocol: "vless", Reason: fmt.Sprintf("transport %q is not implemented", network)}
	}
	if flow != "" {
		return &UnsupportedError{Protocol: "vless", Reason: fmt.Sprintf("flow %q (XTLS) is not implemented", flow)}
	}
	return nil
}

func (d *vlessDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("vless proxy supports tcp only, got %q", network)
	}
	if err := validateSecurityAndNetwork(d.security, d.network, d.flow); err != nil {
		return nil, err
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

// writeRequest sends the VLESS request header:
// [version][uuid 16 bytes][addon length][command][port][address type][address].
func (d *vlessDialer) writeRequest(ctx context.Context, conn net.Conn, addr string) error {
	host, port, err := splitAddr(addr)
	if err != nil {
		return err
	}
	target, err := v2rayAddress(host, port)
	if err != nil {
		return err
	}

	request := make([]byte, 0, 1+16+1+1+len(target))
	request = append(request, 0x00) // protocol version
	request = append(request, d.uuid[:]...)
	request = append(request, 0x00) // no addons
	request = append(request, 0x01) // TCP
	request = append(request, target...)

	if err := writeWithContext(ctx, conn, request); err != nil {
		return fmt.Errorf("write vless request: %w", err)
	}

	// The server replies with [version][addon length][addons...].
	header := make([]byte, 2)
	if _, err := readFullWithContext(ctx, conn, header); err != nil {
		return fmt.Errorf("read vless response: %w", err)
	}
	if header[0] != 0x00 {
		return fmt.Errorf("unexpected vless version: 0x%02x", header[0])
	}
	if header[1] != 0x00 {
		addons := make([]byte, header[1])
		if _, err := readFullWithContext(ctx, conn, addons); err != nil {
			return fmt.Errorf("read vless addons: %w", err)
		}
	}
	return nil
}
