package proxy

import (
	"context"
	"crypto/aes"
	"crypto/md5" //nolint:gosec // protocol requirement
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestVMessKDFVector checks the key derivation against the vector published by
// v2ray-core (proxy/vmess/aead/kdf_test.go).
func TestVMessKDFVector(t *testing.T) {
	got := vmessKDF(
		[]byte("Demo Key for KDF Value Test"),
		"Demo Path for KDF Value Test",
		"Demo Path for KDF Value Test2",
		"Demo Path for KDF Value Test3",
	)
	const want = "53e9d7e1bd7bd25022b71ead07d8a596efc8a845c7888652fd684b4903dc8892"
	if hex.EncodeToString(got) != want {
		t.Fatalf("vmessKDF mismatch:\n got %s\nwant %s", hex.EncodeToString(got), want)
	}
}

func TestVMessDialerRoundTrip(t *testing.T) {
	target := startEcho(t)
	const uuid = "b831381d-6324-4d53-ad4f-8cda48b30811"

	id, err := normalizeUUID(uuid)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	cmdKey := md5.Sum(append(append([]byte(nil), id[:]...), vmessMagic...)) //nolint:gosec // protocol requirement

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
			go handleVMessTestServer(conn, cmdKey)
		}
	}()

	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	payload := map[string]string{
		"v":    "2",
		"ps":   "test",
		"add":  host,
		"port": port,
		"id":   uuid,
		"aid":  "0",
		"net":  "tcp",
		"type": "none",
	}
	encoded, err := jsonMarshal(payload)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}

	dialer, err := NewVMessDialer("vmess://" + base64.StdEncoding.EncodeToString(encoded))
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	conn, err := dialer.DialContext(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("dial through vmess: %v", err)
	}
	assertRoundTrip(t, conn, "vmess-works")
}

func TestVMessLinkParsing(t *testing.T) {
	raw := `{"v":"2","ps":"node","add":"example.net","port":"443","id":"b831381d-6324-4d53-ad4f-8cda48b30811","aid":"0","net":"ws","type":"none","host":"example.net","path":"/ws","tls":"tls","sni":"sni.example.net","scy":"auto"}`
	link := "vmess://" + base64.StdEncoding.EncodeToString([]byte(raw))

	parsed, err := parseVMessLink(link)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Address != "example.net" || parsed.Network != "ws" || parsed.Path != "/ws" {
		t.Fatalf("unexpected parse result: %+v", parsed)
	}
	if port, err := parsed.intPort(); err != nil || port != 443 {
		t.Fatalf("unexpected port: %d (%v)", port, err)
	}

	dialer, err := NewVMessDialer(link)
	if err != nil {
		t.Fatalf("build dialer: %v", err)
	}
	vmess, ok := dialer.(*vmessDialer)
	if !ok {
		t.Fatalf("unexpected dialer type: %T", dialer)
	}
	if vmess.network != "ws" || vmess.tls == nil || vmess.tls.ServerName != "sni.example.net" {
		t.Fatalf("unexpected vmess dialer: %+v", vmess)
	}
	if vmess.address != "example.net:443" {
		t.Fatalf("unexpected address: %s", vmess.address)
	}
}

// handleVMessTestServer is a minimal VMess server implemented from the
// protocol description. It validates the client request and then relays the
// decrypted stream to the requested target.
func handleVMessTestServer(conn net.Conn, cmdKey [16]byte) {
	defer conn.Close()

	authID := make([]byte, 16)
	if _, err := io.ReadFull(conn, authID); err != nil {
		return
	}
	if !vmessVerifyAuthID(cmdKey, authID) {
		return
	}

	encryptedLength := make([]byte, 18)
	if _, err := io.ReadFull(conn, encryptedLength); err != nil {
		return
	}
	nonce := make([]byte, 8)
	if _, err := io.ReadFull(conn, nonce); err != nil {
		return
	}

	lengthAEAD, err := aesGCM(vmessKDF16(cmdKey[:], kdfSaltHeaderLengthKey, string(authID), string(nonce)))
	if err != nil {
		return
	}
	rawLength, err := lengthAEAD.Open(nil, vmessKDF(cmdKey[:], kdfSaltHeaderLengthIV, string(authID), string(nonce))[:12], encryptedLength, authID)
	if err != nil || len(rawLength) != 2 {
		return
	}
	headerSize := int(binary.BigEndian.Uint16(rawLength))

	payloadAEAD, err := aesGCM(vmessKDF16(cmdKey[:], kdfSaltHeaderPayloadKey, string(authID), string(nonce)))
	if err != nil {
		return
	}
	encryptedHeader := make([]byte, headerSize+16)
	if _, err := io.ReadFull(conn, encryptedHeader); err != nil {
		return
	}
	header, err := payloadAEAD.Open(nil, vmessKDF(cmdKey[:], kdfSaltHeaderPayloadIV, string(authID), string(nonce))[:12], encryptedHeader, authID)
	if err != nil {
		return
	}

	requestKey, requestIV, responseHeaderByte, security, host, port, err := parseVMessTestHeader(header)
	if err != nil {
		return
	}

	keyDigest := sha256.Sum256(requestKey[:])
	ivDigest := sha256.Sum256(requestIV[:])
	var responseKey, responseIV [16]byte
	copy(responseKey[:], keyDigest[:16])
	copy(responseIV[:], ivDigest[:16])

	requestAEAD, err := vmessBodyAEAD(security, requestKey[:])
	if err != nil {
		return
	}
	responseAEAD, err := vmessBodyAEAD(security, responseKey[:])
	if err != nil {
		return
	}

	// The address travels inside the request header, so the body carries the
	// raw payload straight away.
	reader := newChunkReader(conn, requestAEAD, requestIV)
	upstream, err := net.Dial("tcp", net.JoinHostPort(host, itoa(port)))
	if err != nil {
		return
	}
	defer upstream.Close()

	// Reply with the encrypted response header.
	responseLengthAEAD, err := aesGCM(vmessKDF16(responseKey[:], kdfSaltResponseLengthKey))
	if err != nil {
		return
	}
	responsePayloadAEAD, err := aesGCM(vmessKDF16(responseKey[:], kdfSaltResponseHeaderKey))
	if err != nil {
		return
	}
	responseHeader := []byte{responseHeaderByte, 0x00, 0x00, 0x00}
	sealedLength := responseLengthAEAD.Seal(nil, vmessKDF(responseIV[:], kdfSaltResponseLengthIV)[:12], []byte{0x00, 0x04}, nil)
	sealedHeader := responsePayloadAEAD.Seal(nil, vmessKDF(responseIV[:], kdfSaltResponseHeaderIV)[:12], responseHeader, nil)
	if _, err := conn.Write(sealedLength); err != nil {
		return
	}
	if _, err := conn.Write(sealedHeader); err != nil {
		return
	}

	writer := newChunkWriter(conn, responseAEAD, responseIV)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := io.Copy(writer, upstream); err != nil {
			return
		}
	}()

	_, _ = io.Copy(upstream, reader)
	<-done
}

// parseVMessTestHeader validates a decrypted request header and extracts the
// session keys together with the expected response header byte.
func parseVMessTestHeader(header []byte) (requestKey, requestIV [16]byte, responseHeader, security byte, host string, port int, err error) {
	const fixedSize = 1 + 16 + 16 + 1 + 1 + 3 // version, IV, key, resp header, option, security/zero/command
	if len(header) < fixedSize+4 {
		return requestKey, requestIV, 0, 0, "", 0, errors.New("header too short")
	}
	if header[0] != vmessVersion {
		return requestKey, requestIV, 0, 0, "", 0, fmt.Errorf("unexpected version: 0x%02x", header[0])
	}

	checksum := fnv.New32a()
	if _, err := checksum.Write(header[:len(header)-4]); err != nil {
		return requestKey, requestIV, 0, 0, "", 0, err
	}
	if string(checksum.Sum(nil)) != string(header[len(header)-4:]) {
		return requestKey, requestIV, 0, 0, "", 0, errors.New("header checksum mismatch")
	}

	copy(requestIV[:], header[1:17])
	copy(requestKey[:], header[17:33])
	responseHeader = header[33]
	security = header[35]
	if header[37] != vmessCommandTCP {
		return requestKey, requestIV, 0, 0, "", 0, errors.New("only TCP is supported")
	}

	// The address block sits between the fixed part and the checksum:
	// port (2 bytes) followed by the address itself.
	addressBlock := header[fixedSize : len(header)-4]
	host, port, _, addressErr := decodeV2RayTestAddress(addressBlock)
	if addressErr != nil {
		return requestKey, requestIV, 0, 0, "", 0, addressErr
	}
	return requestKey, requestIV, responseHeader, security, host, port, nil
}

// decodeV2RayTestAddress reads the VMess/VLESS address layout: port first,
// then 0x01 IPv4, 0x02 domain, 0x03 IPv6.
func decodeV2RayTestAddress(block []byte) (string, int, int, error) {
	if len(block) < 5 {
		return "", 0, 0, errors.New("truncated address")
	}
	port := int(binary.BigEndian.Uint16(block[:2]))
	switch block[2] {
	case 0x01:
		if len(block) < 7 {
			return "", 0, 0, errors.New("truncated IPv4 address")
		}
		return net.IP(block[3:7]).String(), port, 7, nil
	case 0x02:
		length := int(block[3])
		if len(block) < 4+length+2 {
			return "", 0, 0, errors.New("truncated domain address")
		}
		return string(block[4 : 4+length]), port, 4 + length, nil
	case 0x03:
		if len(block) < 19 {
			return "", 0, 0, errors.New("truncated IPv6 address")
		}
		return net.IP(block[3:19]).String(), port, 19, nil
	}
	return "", 0, 0, errors.New("unknown address type")
}

// vmessVerifyAuthID decrypts the authentication identifier and checks its
// checksum and timestamp window, exactly like a server does.
func vmessVerifyAuthID(cmdKey [16]byte, authID []byte) bool {
	block, err := aes.NewCipher(vmessKDF16(cmdKey[:], kdfSaltAuthIDEncryptionKey))
	if err != nil {
		return false
	}
	decrypted := make([]byte, 16)
	block.Decrypt(decrypted, authID)

	if crc32.ChecksumIEEE(decrypted[:12]) != binary.BigEndian.Uint32(decrypted[12:]) {
		return false
	}
	moment := int64(binary.BigEndian.Uint64(decrypted[:8]))
	return time.Now().Unix()-moment < 120 && moment-time.Now().Unix() < 120
}

func jsonMarshal(payload map[string]string) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("{")
	first := true
	for key, value := range payload {
		if !first {
			builder.WriteString(",")
		}
		first = false
		builder.WriteString(`"` + key + `":"` + value + `"`)
	}
	builder.WriteString("}")
	return []byte(builder.String()), nil
}
