package proxy

import (
	"bufio"
	"context"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// startEcho starts a TCP listener that echoes every byte it receives back to
// the sender. It stands in for the target website.
func startEcho(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener.Addr().String()
}

// assertRoundTrip writes payload through the proxy connection and checks that
// the echo target sent it back unchanged.
func assertRoundTrip(t *testing.T, conn net.Conn, payload string) {
	t.Helper()
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write through proxy: %v", err)
	}
	buffer := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buffer); err != nil {
		t.Fatalf("read through proxy: %v", err)
	}
	if string(buffer) != payload {
		t.Fatalf("round trip mismatch: got %q, want %q", buffer, payload)
	}
}

// relay copies data between two connections and closes both.
func relay(a, b net.Conn) {
	defer a.Close()
	defer b.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}

func readHTTPHeaders(reader *bufio.Reader) error {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimRight(line, "\r\n") == "" {
			return nil
		}
	}
}

func TestHTTPDialerRoundTrip(t *testing.T) {
	target := startEcho(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		reader := bufio.NewReader(conn)
		request, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return
		}
		if !strings.HasPrefix(request, "CONNECT "+target) {
			conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
			conn.Close()
			return
		}
		if err := readHTTPHeaders(reader); err != nil {
			conn.Close()
			return
		}
		upstream, err := net.Dial("tcp", target)
		if err != nil {
			conn.Close()
			return
		}
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
			conn.Close()
			return
		}
		relay(conn, upstream)
	}()

	dialer, err := NewHTTPDialer("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("dial through http proxy: %v", err)
	}
	assertRoundTrip(t, conn, "http-proxy-works")
}

func TestSOCKS5DialerRoundTrip(t *testing.T) {
	target := startEcho(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleSocks5TestServer(conn)
		}
	}()

	dialer, err := NewSOCKS5Dialer("socks5://alice:secret@" + listener.Addr().String())
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("dial through socks5 proxy: %v", err)
	}
	assertRoundTrip(t, conn, "socks5-works")

	// Wrong credentials must be rejected rather than silently tunneled.
	badDialer, err := NewSOCKS5Dialer("socks5://alice:wrong@" + listener.Addr().String())
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	if _, err := badDialer.DialContext(context.Background(), "tcp", target); err == nil {
		t.Fatal("expected authentication failure")
	}
}

// handleSocks5TestServer is a minimal socks5 server used by the tests. It
// requires the credentials alice/secret.
func handleSocks5TestServer(conn net.Conn) {
	defer conn.Close()

	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return
	}
	methods := make([]byte, greeting[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	// Ask for username/password and validate it.
	if _, err := conn.Write([]byte{0x05, 0x02}); err != nil {
		return
	}
	auth := make([]byte, 2)
	if _, err := io.ReadFull(conn, auth); err != nil {
		return
	}
	user := make([]byte, auth[1])
	if _, err := io.ReadFull(conn, user); err != nil {
		return
	}
	passLength := make([]byte, 1)
	if _, err := io.ReadFull(conn, passLength); err != nil {
		return
	}
	password := make([]byte, passLength[0])
	if _, err := io.ReadFull(conn, password); err != nil {
		return
	}
	if string(user) != "alice" || string(password) != "secret" {
		conn.Write([]byte{0x01, 0x01})
		return
	}
	if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
		return
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	host, err := readSocks5TestAddress(conn, header[3])
	if err != nil {
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBytes)

	upstream, err := net.Dial("tcp", net.JoinHostPort(host, itoa(int(port))))
	if err != nil {
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	relay(conn, upstream)
}

// readSocks5TestAddress reads the address part of a socks5 request.
func readSocks5TestAddress(conn net.Conn, addressType byte) (string, error) {
	switch addressType {
	case 0x01:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		return net.IP(ip).String(), nil
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", err
		}
		host := make([]byte, length[0])
		if _, err := io.ReadFull(conn, host); err != nil {
			return "", err
		}
		return string(host), nil
	case 0x04:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		return net.IP(ip).String(), nil
	}
	return "", errors.New("unknown address type")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func TestShadowsocksDialerRoundTrip(t *testing.T) {
	target := startEcho(t)
	password := "s3cret-pass"

	cases := []struct {
		method string
		aead   bool
	}{
		{"aes-256-gcm", true},
		{"aes-128-gcm", true},
		{"chacha20-ietf-poly1305", true},
		{"aes-256-cfb", false},
		{"rc4-md5", false},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			keyLength, saltLength, aead, err := cipherInfo(tc.method)
			if err != nil || aead != tc.aead {
				t.Fatalf("unexpected cipher info: %v", err)
			}
			key := evpBytesToKey(password, keyLength)
			if saltLength == 0 {
				t.Fatalf("missing salt size for %s", tc.method)
			}

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen proxy: %v", err)
			}
			defer listener.Close()

			go serveShadowsocks(listener, key, saltLength, tc.method, tc.aead)

			link := "ss://" + base64RawURL(tc.method+":"+password) + "@" + listener.Addr().String() + "#test"
			dialer, err := NewShadowsocksDialer(link)
			if err != nil {
				t.Fatalf("build dialer: %v", err)
			}
			conn, err := dialer.DialContext(context.Background(), "tcp", target)
			if err != nil {
				t.Fatalf("dial through shadowsocks: %v", err)
			}
			assertRoundTrip(t, conn, "shadowsocks-works")
		})
	}
}

// serveShadowsocks runs a minimal shadowsocks server implemented directly from
// the protocol description, so the client is checked against an independent
// implementation of the same spec.
func serveShadowsocks(listener net.Listener, key []byte, saltSize int, method string, aead bool) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	var client io.Reader
	var server io.Writer

	if aead {
		// The client's salt arrives first and keys the client -> server direction.
		salt := make([]byte, saltSize)
		if _, err := io.ReadFull(conn, salt); err != nil {
			return
		}
		_, opener, err := ssAEADPair(method, key, salt)
		if err != nil {
			return
		}
		client = &ssAEADReader{r: conn, opener: opener}

		// The server then emits its own salt for the return direction.
		outSalt := make([]byte, saltSize)
		if _, err := io.ReadFull(rand.Reader, outSalt); err != nil {
			return
		}
		sealer, _, err := ssAEADPair(method, key, outSalt)
		if err != nil {
			return
		}
		if _, err := conn.Write(outSalt); err != nil {
			return
		}
		server = &ssAEADWriter{w: conn, sealer: sealer}
	} else {
		iv := make([]byte, saltSize)
		if _, err := io.ReadFull(conn, iv); err != nil {
			return
		}
		inStream, err := newSSStream(method, key, iv, false)
		if err != nil {
			return
		}
		client = &cipherStreamReader{stream: inStream, r: conn}

		outIV := make([]byte, saltSize)
		if _, err := io.ReadFull(rand.Reader, outIV); err != nil {
			return
		}
		outStream, err := newSSStream(method, key, outIV, true)
		if err != nil {
			return
		}
		if _, err := conn.Write(outIV); err != nil {
			return
		}
		server = &cipherStreamWriter{stream: outStream, w: conn}
	}

	// First chunk carries the socks5 style target address.
	address := make([]byte, 1+1+255+2)
	n, err := client.Read(address)
	if err != nil || n < 4 {
		return
	}
	host, port, consumed, err := decodeSocks5Address(address[:n])
	if err != nil {
		return
	}

	upstream, err := net.Dial("tcp", net.JoinHostPort(host, itoa(port)))
	if err != nil {
		return
	}
	defer upstream.Close()

	// Stream ciphers have no framing, so the first read may already carry
	// payload bytes that follow the address.
	if leftover := address[consumed:n]; len(leftover) > 0 {
		if _, err := upstream.Write(leftover); err != nil {
			return
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(server, upstream)
	}()
	_, _ = io.Copy(upstream, client)
	<-done
}

type cipherStreamReader struct {
	stream cipher.Stream
	r      io.Reader
}

func (c *cipherStreamReader) Read(out []byte) (int, error) {
	n, err := c.r.Read(out)
	if n > 0 {
		c.stream.XORKeyStream(out[:n], out[:n])
	}
	return n, err
}

type cipherStreamWriter struct {
	stream cipher.Stream
	w      io.Writer
}

func (c *cipherStreamWriter) Write(payload []byte) (int, error) {
	buffer := make([]byte, len(payload))
	c.stream.XORKeyStream(buffer, payload)
	return c.w.Write(buffer)
}

// decodeSocks5Address parses a socks5 address block and reports how many bytes
// it consumed. With stream ciphers the payload follows immediately, so the
// remaining bytes of the buffer still belong to the client.
func decodeSocks5Address(block []byte) (string, int, int, error) {
	if len(block) < 2 {
		return "", 0, 0, errors.New("truncated address")
	}
	switch block[0] {
	case 0x01: // IPv4
		if len(block) < 1+4+2 {
			return "", 0, 0, errors.New("truncated IPv4 address")
		}
		host := net.IP(block[1:5]).String()
		return host, int(binary.BigEndian.Uint16(block[5:7])), 7, nil
	case 0x03: // domain
		length := int(block[1])
		if len(block) < 2+length+2 {
			return "", 0, 0, errors.New("truncated domain address")
		}
		return string(block[2 : 2+length]), int(binary.BigEndian.Uint16(block[2+length:])), 2 + length + 2, nil
	case 0x04: // IPv6
		if len(block) < 1+16+2 {
			return "", 0, 0, errors.New("truncated IPv6 address")
		}
		host := net.IP(block[1:17]).String()
		return host, int(binary.BigEndian.Uint16(block[17:19])), 19, nil
	}
	return "", 0, 0, errors.New("unknown address type")
}

func TestTrojanDialerRoundTrip(t *testing.T) {
	target := startEcho(t)
	password := "trojan-password"
	expected := hex.EncodeToString(sha256Sum224(password))

	cert := selfSignedCert(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("listen proxy: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.(*tls.Conn).Handshake(); err != nil {
			return
		}
		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if strings.TrimRight(line, "\r\n") != expected {
			return
		}
		command := make([]byte, 1)
		if _, err := io.ReadFull(reader, command); err != nil {
			return
		}
		if command[0] != 0x01 {
			return
		}
		addressType := make([]byte, 1)
		if _, err := io.ReadFull(reader, addressType); err != nil {
			return
		}
		host, port, err := trojanReadAddress(reader, addressType[0])
		if err != nil {
			return
		}
		upstream, err := net.Dial("tcp", net.JoinHostPort(host, itoa(port)))
		if err != nil {
			return
		}
		relay(conn, upstream)
	}()

	link := "trojan://" + password + "@" + listener.Addr().String() + "?security=tls&allowInsecure=1&type=tcp"
	dialer, err := NewTrojanDialer(link)
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("dial through trojan: %v", err)
	}
	assertRoundTrip(t, conn, "trojan-works")
}

func trojanReadAddress(reader io.Reader, addressType byte) (string, int, error) {
	var host string
	switch addressType {
	case 0x01:
		buffer := make([]byte, 4)
		if _, err := io.ReadFull(reader, buffer); err != nil {
			return "", 0, err
		}
		host = net.IP(buffer).String()
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(reader, length); err != nil {
			return "", 0, err
		}
		domain := make([]byte, length[0])
		if _, err := io.ReadFull(reader, domain); err != nil {
			return "", 0, err
		}
		host = string(domain)
	case 0x04:
		buffer := make([]byte, 16)
		if _, err := io.ReadFull(reader, buffer); err != nil {
			return "", 0, err
		}
		host = net.IP(buffer).String()
	default:
		return "", 0, errors.New("unknown trojan address type")
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return "", 0, err
	}
	return host, int(binary.BigEndian.Uint16(portBytes)), nil
}

func TestVLESSDialerRoundTrip(t *testing.T) {
	target := startEcho(t)
	uuid := "b831381d-6324-4d53-ad4f-8cda48b30811"

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		expectedUUID, err := normalizeUUID(uuid)
		if err != nil {
			return
		}
		header := make([]byte, 18)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		if header[0] != 0x00 || string(header[1:17]) != string(expectedUUID[:]) || header[17] != 0x00 {
			return
		}
		command := make([]byte, 1)
		if _, err := io.ReadFull(conn, command); err != nil || command[0] != 0x01 {
			return
		}
		portBytes := make([]byte, 2)
		if _, err := io.ReadFull(conn, portBytes); err != nil {
			return
		}
		port := binary.BigEndian.Uint16(portBytes)
		addressType := make([]byte, 1)
		if _, err := io.ReadFull(conn, addressType); err != nil {
			return
		}
		var host string
		switch addressType[0] {
		case 0x01:
			ip := make([]byte, 4)
			if _, err := io.ReadFull(conn, ip); err != nil {
				return
			}
			host = net.IP(ip).String()
		case 0x02:
			length := make([]byte, 1)
			if _, err := io.ReadFull(conn, length); err != nil {
				return
			}
			domain := make([]byte, length[0])
			if _, err := io.ReadFull(conn, domain); err != nil {
				return
			}
			host = string(domain)
		case 0x03:
			ip := make([]byte, 16)
			if _, err := io.ReadFull(conn, ip); err != nil {
				return
			}
			host = net.IP(ip).String()
		default:
			return
		}
		upstream, err := net.Dial("tcp", net.JoinHostPort(host, itoa(int(port))))
		if err != nil {
			return
		}
		if _, err := conn.Write([]byte{0x00, 0x00}); err != nil {
			return
		}
		relay(conn, upstream)
	}()

	dialer, err := NewVLESSDialer("vless://" + uuid + "@" + listener.Addr().String() + "?type=tcp&security=none")
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("dial through vless: %v", err)
	}
	assertRoundTrip(t, conn, "vless-works")
}

func TestUnsupportedProtocols(t *testing.T) {
	links := []string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@example.com:443?security=reality&type=tcp",
		"hysteria2://password@example.com:443",
		"mtproto://secret@example.com:443",
	}
	for _, link := range links {
		dialer, err := NewDialer(link)
		if err == nil {
			t.Fatalf("expected %s to be unsupported", link)
		}
		if !ErrUnsupported(err) {
			t.Fatalf("expected unsupported error for %s, got %T: %v", link, err, err)
		}
		if dialer != nil {
			t.Fatal("expected nil dialer")
		}
	}
}

// helpers used by the tests

func sha256Sum224(value string) []byte {
	digest := sha256.Sum224([]byte(value))
	return digest[:]
}

func base64RawURL(value string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var out strings.Builder
	for i := 0; i < len(value); i += 3 {
		var chunk [3]byte
		n := copy(chunk[:], value[i:])
		out.WriteByte(alphabet[chunk[0]>>2])
		switch n {
		case 1:
			out.WriteByte(alphabet[(chunk[0]&0x03)<<4])
		case 2:
			out.WriteByte(alphabet[(chunk[0]&0x03)<<4|(chunk[1]>>4)])
			out.WriteByte(alphabet[(chunk[1]&0x0f)<<2])
		default:
			out.WriteByte(alphabet[(chunk[0]&0x03)<<4|(chunk[1]>>4)])
			out.WriteByte(alphabet[(chunk[1]&0x0f)<<2|(chunk[2]>>6)])
			out.WriteByte(alphabet[chunk[2]&0x3f])
		}
	}
	return out.String()
}
