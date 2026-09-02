package proxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// wsOptions describes a WebSocket transport hop.
type wsOptions struct {
	Host string
	Path string
	// Headers are extra request headers, used for the v2ray "host" override.
	Headers map[string]string
}

// wsHandshake upgrades an existing connection to WebSocket (RFC 6455).
func wsHandshake(ctx context.Context, conn net.Conn, opts wsOptions) (net.Conn, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	path := opts.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	host := firstNonEmpty(opts.Host, "")

	headers := make(http.Header)
	headers.Set("Host", host)
	headers.Set("Upgrade", "websocket")
	headers.Set("Connection", "Upgrade")
	headers.Set("Sec-WebSocket-Key", base64.StdEncoding.EncodeToString(key))
	headers.Set("Sec-WebSocket-Version", "13")
	for name, value := range opts.Headers {
		headers.Set(name, value)
	}

	var raw strings.Builder
	raw.WriteString("GET " + path + " HTTP/1.1\r\n")
	for _, name := range []string{"Host", "Upgrade", "Connection", "Sec-WebSocket-Key", "Sec-WebSocket-Version"} {
		raw.WriteString(name + ": " + headers.Get(name) + "\r\n")
	}
	for name, value := range opts.Headers {
		if headers.Get(name) == value {
			continue
		}
		raw.WriteString(name + ": " + value + "\r\n")
	}
	raw.WriteString("\r\n")

	if err := writeWithContext(ctx, conn, []byte(raw.String())); err != nil {
		return nil, fmt.Errorf("write websocket upgrade: %w", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := readLineWithContext(ctx, conn, reader)
	if err != nil {
		return nil, fmt.Errorf("read websocket upgrade response: %w", err)
	}
	fields := strings.Fields(statusLine)
	if len(fields) < 2 || fields[1] != "101" {
		return nil, fmt.Errorf("websocket upgrade rejected: %s", strings.TrimSpace(statusLine))
	}
	// Skip the remaining response headers.
	for {
		line, err := readLineWithContext(ctx, conn, reader)
		if err != nil {
			return nil, fmt.Errorf("read websocket headers: %w", err)
		}
		if line == "" {
			break
		}
	}

	return &wsConn{Conn: conn, reader: reader, mask: true}, nil
}

// wsConn implements net.Conn on top of WebSocket binary frames. Clients must
// mask their frames, servers must not; the mask flag selects the behaviour so
// the same type can be used on both ends.
type wsConn struct {
	net.Conn
	reader    *bufio.Reader
	pending   []byte
	mask      bool
	fragments []byte
}

const (
	wsHeaderMinSize = 2
	wsMaxFrameSize  = 1 << 20
)

func (c *wsConn) Read(out []byte) (int, error) {
	for len(c.pending) == 0 {
		if err := c.readFrame(); err != nil {
			return 0, err
		}
	}
	n := copy(out, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func (c *wsConn) Write(payload []byte) (int, error) {
	if len(payload) == 0 {
		return 0, nil
	}
	header := make([]byte, wsHeaderMinSize)
	header[0] = 0x82 // FIN + binary opcode

	length := len(payload)
	switch {
	case length < 126:
		header[1] = byte(length)
	case length < 65536:
		header[1] = 126
		header = append(header, byte(length>>8), byte(length))
	default:
		header[1] = 127
		for i := 7; i >= 0; i-- {
			header = append(header, byte(length>>(8*i)))
		}
	}
	if c.mask {
		header[1] |= 0x80
	}

	body := payload
	if c.mask {
		mask := make([]byte, 4)
		if _, err := rand.Read(mask); err != nil {
			return 0, err
		}
		header = append(header, mask...)
		body = make([]byte, len(payload))
		copy(body, payload)
		for i := range body {
			body[i] ^= mask[i%4]
		}
	}

	if _, err := c.Conn.Write(header); err != nil {
		return 0, err
	}
	if _, err := c.Conn.Write(body); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (c *wsConn) readFrame() error {
	header := make([]byte, wsHeaderMinSize)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		return err
	}

	fin := header[0]&0x80 != 0
	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0

	length := int64(header[1] & 0x7F)
	switch length {
	case 126:
		extended := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, extended); err != nil {
			return err
		}
		length = int64(extended[0])<<8 | int64(extended[1])
	case 127:
		extended := make([]byte, 8)
		if _, err := io.ReadFull(c.reader, extended); err != nil {
			return err
		}
		length = 0
		for _, b := range extended {
			length = length<<8 | int64(b)
		}
	}
	if length < 0 || length > wsMaxFrameSize {
		return fmt.Errorf("invalid websocket frame length: %d", length)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, maskKey[:]); err != nil {
			return err
		}
	}

	chunk := make([]byte, length)
	if _, err := io.ReadFull(c.reader, chunk); err != nil {
		return err
	}
	if masked {
		for i := range chunk {
			chunk[i] ^= maskKey[i%4]
		}
	}

	switch opcode {
	case 0x0: // continuation of a fragmented message
		c.fragments = append(c.fragments, chunk...)
		if !fin {
			c.pending = nil
			return nil
		}
		c.pending = c.fragments
		c.fragments = nil
		return nil
	case 0x2: // binary
		if !fin {
			// Start of a fragmented message: keep collecting.
			c.fragments = append(c.fragments, chunk...)
			c.pending = nil
			return nil
		}
		if len(c.fragments) > 0 {
			c.pending = append(c.fragments, chunk...)
			c.fragments = nil
		} else {
			c.pending = chunk
		}
		return nil
	case 0x9: // ping
		return c.writeControl(0xA, chunk)
	case 0x8: // close
		return io.EOF
	case 0xA: // pong
		return nil
	default:
		return fmt.Errorf("unsupported websocket opcode: 0x%02x", opcode)
	}
}

func (c *wsConn) writeControl(opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode, byte(len(payload))}
	body := payload
	if c.mask {
		mask := make([]byte, 4)
		if _, err := rand.Read(mask); err != nil {
			return err
		}
		header[1] |= 0x80
		header = append(header, mask...)
		body = make([]byte, len(payload))
		for i := range payload {
			body[i] = payload[i] ^ mask[i%4]
		}
	}
	if _, err := c.Conn.Write(header); err != nil {
		return err
	}
	_, err := c.Conn.Write(body)
	return err
}
