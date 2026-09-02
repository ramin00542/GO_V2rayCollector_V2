package proxy

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // required by the WebSocket handshake
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
)

// wsGUID is the magic value defined by RFC 6455 for the handshake response.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsServerHandshake completes the server side of a WebSocket upgrade and
// returns the framed connection.
func wsServerHandshake(conn net.Conn, expectedPath string) (net.Conn, error) {
	reader := bufio.NewReader(conn)

	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(requestLine)
	if len(fields) < 2 {
		return nil, io.ErrUnexpectedEOF
	}
	if fields[1] != expectedPath {
		return nil, errUnexpectedPath
	}

	var key string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, _ := strings.Cut(line, ":")
		if strings.EqualFold(strings.TrimSpace(name), "Sec-WebSocket-Key") {
			key = strings.TrimSpace(value)
		}
	}
	if key == "" {
		return nil, errMissingKey
	}

	digest := sha1.Sum([]byte(key + wsGUID)) //nolint:gosec // protocol requirement
	accept := base64.StdEncoding.EncodeToString(digest[:])

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := conn.Write([]byte(response)); err != nil {
		return nil, err
	}
	// Servers send unmasked frames.
	return &wsConn{Conn: conn, reader: reader, mask: false}, nil
}

var (
	errUnexpectedPath = &wsTestError{"unexpected websocket path"}
	errMissingKey     = &wsTestError{"missing websocket key"}
)

type wsTestError struct{ message string }

func (e *wsTestError) Error() string { return e.message }

func TestVLESSOverWebSocket(t *testing.T) {
	target := startEcho(t)
	const uuid = "b831381d-6324-4d53-ad4f-8cda48b30811"
	const path = "/vmess-ws"

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
			go func() {
				framed, err := wsServerHandshake(conn, path)
				if err != nil {
					conn.Close()
					return
				}
				// After the upgrade the payload is a plain VLESS request.
				expectVLESSDest(framed)
			}()
		}
	}()

	dialer, err := NewVLESSDialer("vless://" + uuid + "@" + listener.Addr().String() + "?type=ws&security=none&path=" + path)
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	conn, err := dialer.DialContext(contextForTest(), "tcp", target)
	if err != nil {
		t.Fatalf("dial through vless+ws: %v", err)
	}
	assertRoundTrip(t, conn, "vless-ws-works")
}

func TestVMessOverWebSocket(t *testing.T) {
	target := startEcho(t)
	const uuid = "b831381d-6324-4d53-ad4f-8cda48b30811"
	const path = "/vmess"

	id, err := normalizeUUID(uuid)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	cmdKey := md5Sum(append(append([]byte(nil), id[:]...), vmessMagic...))

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
			go func() {
				framed, err := wsServerHandshake(conn, path)
				if err != nil {
					conn.Close()
					return
				}
				handleVMessTestServer(framed, cmdKey)
			}()
		}
	}()

	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	payload, err := jsonMarshal(map[string]string{
		"v":    "2",
		"ps":   "ws",
		"add":  host,
		"port": port,
		"id":   uuid,
		"aid":  "0",
		"net":  "ws",
		"type": "none",
		"path": path,
		"host": host,
	})
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	dialer, err := NewVMessDialer("vmess://" + base64.StdEncoding.EncodeToString(payload))
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	conn, err := dialer.DialContext(contextForTest(), "tcp", target)
	if err != nil {
		t.Fatalf("dial through vmess+ws: %v", err)
	}
	assertRoundTrip(t, conn, "vmess-ws-works")
}

// expectVLESSDest reads a VLESS request from conn and relays it to the target.
func expectVLESSDest(conn net.Conn) {
	defer conn.Close()

	// [version][uuid 16][addon length]
	header := make([]byte, 18)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != 0x00 || header[17] != 0x00 {
		return
	}
	command := make([]byte, 1)
	if _, err := io.ReadFull(conn, command); err != nil || command[0] != 0x01 {
		return
	}

	host, port, err := readV2RayTestAddress(conn)
	if err != nil {
		return
	}

	upstream, err := net.Dial("tcp", net.JoinHostPort(host, itoa(port)))
	if err != nil {
		return
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{0x00, 0x00}); err != nil {
		return
	}
	relay(conn, upstream)
}

// readV2RayTestAddress reads "port(2) + address type + address" from conn.
func readV2RayTestAddress(conn net.Conn) (string, int, error) {
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", 0, err
	}
	port := int(binary.BigEndian.Uint16(portBytes))

	addressType := make([]byte, 1)
	if _, err := io.ReadFull(conn, addressType); err != nil {
		return "", 0, err
	}
	switch addressType[0] {
	case 0x01:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", 0, err
		}
		return net.IP(ip).String(), port, nil
	case 0x02:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", 0, err
		}
		domain := make([]byte, length[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", 0, err
		}
		return string(domain), port, nil
	case 0x03:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", 0, err
		}
		return net.IP(ip).String(), port, nil
	}
	return "", 0, errors.New("unknown address type")
}
