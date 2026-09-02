package proxy

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // required by the VMess protocol
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"hash/fnv"
	"io"
	"net"
	"strings"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/proxy/chacha20poly1305"
)

// VMess constants.
const (
	vmessVersion byte = 1

	vmessCommandTCP byte = 0x01

	// Request options. We only ever send plain chunked streams, which every
	// server understands and which avoids the SHAKE based length masking.
	vmessOptionChunkStream byte = 0x01

	// Body encryption types as understood by VMess.
	vmessSecurityAES128GCM byte = 0x03
	vmessSecurityChaCha20  byte = 0x04
	vmessSecurityNone      byte = 0x05

	// Maximum body chunk size (v2ray uses the same value).
	vmessChunkSize = 8192
)

// vmessMagic is appended to the UUID before hashing to build the command key.
var vmessMagic = []byte{
	0xc4, 0x86, 0x19, 0xfe, 0x8f, 0x02, 0x49, 0xe0,
	0xb9, 0xe9, 0xed, 0xf7, 0x63, 0xe1, 0x7e, 0x21,
}

// KDF salts taken from the VMess AEAD specification.
const (
	kdfSaltAuthIDEncryptionKey = "AES Auth ID Encryption"
	kdfSaltVMessAEADKDF        = "VMess AEAD KDF"
	kdfSaltHeaderPayloadKey    = "VMess Header AEAD Key"
	kdfSaltHeaderPayloadIV     = "VMess Header AEAD Nonce"
	kdfSaltHeaderLengthKey     = "VMess Header AEAD Key_Length"
	kdfSaltHeaderLengthIV      = "VMess Header AEAD Nonce_Length"
	kdfSaltResponseHeaderKey   = "AEAD Resp Header Key"
	kdfSaltResponseHeaderIV    = "AEAD Resp Header IV"
	kdfSaltResponseLengthKey   = "AEAD Resp Header Len Key"
	kdfSaltResponseLengthIV    = "AEAD Resp Header Len IV"
)

// vmessDialer implements the VMess protocol (AEAD header variant).
type vmessDialer struct {
	address  string
	id       [16]byte
	cmdKey   [16]byte
	security byte
	network  string
	tls      *tlsOptions
	wsPath   string
	wsHost   string
	httpHost []string
	httpPath string
}

// NewVMessDialer builds a dialer for `vmess://` links.
func NewVMessDialer(link string) (Dialer, error) {
	parsed, err := parseVMessLink(link)
	if err != nil {
		return nil, err
	}
	id, err := normalizeUUID(parsed.ID)
	if err != nil {
		return nil, err
	}
	port, err := parsed.intPort()
	if err != nil {
		return nil, err
	}

	security := byte(vmessSecurityAES128GCM)
	switch strings.ToLower(strings.TrimSpace(parsed.Security)) {
	case "", "auto", "aes-128-gcm":
		security = vmessSecurityAES128GCM
	case "chacha20-poly1305":
		security = vmessSecurityChaCha20
	case "none", "zero":
		// Plain bodies use a masked length parser we do not implement.
		return nil, &UnsupportedError{Protocol: "vmess", Reason: "unencrypted body is not implemented"}
	default:
		return nil, &UnsupportedError{Protocol: "vmess", Reason: fmt.Sprintf("encryption %q is not implemented", parsed.Security)}
	}
	if parsed.alterID() != 0 {
		return nil, &UnsupportedError{Protocol: "vmess", Reason: "legacy alterId authentication is not implemented"}
	}

	network := strings.ToLower(strings.TrimSpace(parsed.Network))
	if network == "" {
		network = "tcp"
	}
	if network != "tcp" && network != "ws" {
		return nil, &UnsupportedError{Protocol: "vmess", Reason: fmt.Sprintf("transport %q is not implemented", network)}
	}

	d := &vmessDialer{
		address:  joinHostPort(parsed.Address, port),
		id:       id,
		security: security,
		network:  network,
		wsPath:   parsed.Path,
		wsHost:   firstNonEmpty(parsed.Host, parsed.Address),
	}
	d.cmdKey = md5.Sum(append(append([]byte(nil), id[:]...), vmessMagic...)) //nolint:gosec // protocol requirement

	if strings.EqualFold(strings.TrimSpace(parsed.TLS), "tls") {
		host := firstNonEmpty(parsed.SNI, hostOnly(parsed.Host), parsed.Address)
		d.tls = &tlsOptions{ServerName: host, Insecure: parsed.AllowInsecure}
	}

	return d, nil
}

func (d *vmessDialer) Protocol() string { return "vmess" }

func (d *vmessDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("vmess proxy supports tcp only, got %q", network)
	}

	host, port, err := splitAddr(addr)
	if err != nil {
		return nil, err
	}
	target, err := v2rayAddress(host, port)
	if err != nil {
		return nil, err
	}

	conn, err := dialTCP(ctx, d.address)
	if err != nil {
		return nil, err
	}

	if d.tls != nil {
		tlsConn := tls.Client(conn, d.tls.config())
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

	wrapped, err := newVMessConn(conn, d.cmdKey, d.security, target)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return wrapped, nil
}

// vmessConn is a VMess connection: the encrypted request header is sent with
// the first write, and the encrypted response header is consumed before the
// first read of the body.
type vmessConn struct {
	net.Conn

	cmdKey   [16]byte
	security byte

	requestKey  [16]byte
	requestIV   [16]byte
	responseKey [16]byte
	responseIV  [16]byte
	respHeader  byte

	target []byte
	sent   bool
	writer *chunkWriter
	reader *chunkReader
}

func newVMessConn(conn net.Conn, cmdKey [16]byte, security byte, target []byte) (*vmessConn, error) {
	random := make([]byte, 33) // 16 + 16 + 1
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}

	c := &vmessConn{Conn: conn, cmdKey: cmdKey, security: security, target: target}
	copy(c.requestKey[:], random[:16])
	copy(c.requestIV[:], random[16:32])
	c.respHeader = random[32]

	// Response key/IV are derived with SHA256 in the AEAD variant.
	keyDigest := sha256.Sum256(c.requestKey[:])
	ivDigest := sha256.Sum256(c.requestIV[:])
	copy(c.responseKey[:], keyDigest[:16])
	copy(c.responseIV[:], ivDigest[:16])

	requestAEAD, err := vmessBodyAEAD(security, c.requestKey[:])
	if err != nil {
		return nil, err
	}
	responseAEAD, err := vmessBodyAEAD(security, c.responseKey[:])
	if err != nil {
		return nil, err
	}
	c.writer = newChunkWriter(conn, requestAEAD, c.requestIV)
	c.reader = newChunkReader(conn, responseAEAD, c.responseIV)
	c.reader.responseKey = c.responseKey
	c.reader.responseIV = c.responseIV
	c.reader.expected = c.respHeader
	return c, nil
}

func vmessBodyAEAD(security byte, key []byte) (cipher.AEAD, error) {
	switch security {
	case vmessSecurityAES128GCM, vmessSecurityNone:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case vmessSecurityChaCha20:
		return chacha20poly1305.New(vmessChacha20Key(key))
	}
	return nil, fmt.Errorf("unsupported vmess security: 0x%02x", security)
}

// vmessChacha20Key expands a 16 byte VMess key into a 32 byte ChaCha20 key the
// same way v2ray does.
func vmessChacha20Key(key []byte) []byte {
	out := make([]byte, 32)
	first := md5.Sum(key) //nolint:gosec // protocol requirement
	copy(out, first[:])
	second := md5.Sum(out[:16]) //nolint:gosec // protocol requirement
	copy(out[16:], second[:])
	return out
}

func (c *vmessConn) Write(payload []byte) (int, error) {
	if !c.sent {
		c.sent = true
		header, err := c.buildRequestHeader()
		if err != nil {
			return 0, err
		}
		if _, err := c.Conn.Write(header); err != nil {
			return 0, fmt.Errorf("write vmess header: %w", err)
		}
	}
	if len(payload) == 0 {
		return 0, nil
	}
	return c.writer.Write(payload)
}

func (c *vmessConn) Read(buffer []byte) (int, error) {
	if err := c.reader.ensureResponseHeader(); err != nil {
		return 0, err
	}
	return c.reader.Read(buffer)
}

// buildRequestHeader constructs and seals the VMess request header.
func (c *vmessConn) buildRequestHeader() ([]byte, error) {
	buffer := make([]byte, 0, 64+len(c.target))
	buffer = append(buffer, vmessVersion)
	buffer = append(buffer, c.requestIV[:]...)
	buffer = append(buffer, c.requestKey[:]...)
	buffer = append(buffer, c.respHeader)
	buffer = append(buffer, vmessOptionChunkStream)
	buffer = append(buffer, byte(c.security), 0x00, vmessCommandTCP)
	buffer = append(buffer, c.target...)

	// FNV-1a checksum of everything written so far.
	checksum := fnv.New32a()
	if _, err := checksum.Write(buffer); err != nil {
		return nil, err
	}
	buffer = checksum.Sum(buffer)

	return vmessSealHeader(c.cmdKey, buffer), nil
}

// vmessSealHeader encrypts the request header with AES-128-GCM keys derived
// from the command key, prefixed by an encrypted auth id.
func vmessSealHeader(key [16]byte, data []byte) []byte {
	authID := vmessCreateAuthID(key[:], time.Now().Unix())

	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return nil
	}

	lengthBuffer := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBuffer, uint16(len(data)))

	lengthKey := vmessKDF16(key[:], kdfSaltHeaderLengthKey, string(authID[:]), string(nonce))
	lengthIV := vmessKDF(key[:], kdfSaltHeaderLengthIV, string(authID[:]), string(nonce))[:12]
	lengthAEAD := newGCMOrNil(lengthKey)
	encryptedLength := lengthAEAD.Seal(nil, lengthIV, lengthBuffer, authID[:])

	payloadKey := vmessKDF16(key[:], kdfSaltHeaderPayloadKey, string(authID[:]), string(nonce))
	payloadIV := vmessKDF(key[:], kdfSaltHeaderPayloadIV, string(authID[:]), string(nonce))[:12]
	payloadAEAD := newGCMOrNil(payloadKey)
	encryptedPayload := payloadAEAD.Seal(nil, payloadIV, data, authID[:])

	out := make([]byte, 0, 16+len(encryptedLength)+8+len(encryptedPayload))
	out = append(out, authID[:]...)
	out = append(out, encryptedLength...)
	out = append(out, nonce...)
	out = append(out, encryptedPayload...)
	return out
}

func newGCMOrNil(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	return aead
}

// vmessCreateAuthID builds the 16 byte authentication identifier.
func vmessCreateAuthID(cmdKey []byte, moment int64) [16]byte {
	buffer := bytes.NewBuffer(nil)
	_ = binary.Write(buffer, binary.BigEndian, moment)
	random := make([]byte, 4)
	_, _ = io.ReadFull(rand.Reader, random)
	buffer.Write(random)
	_ = binary.Write(buffer, binary.BigEndian, crc32.ChecksumIEEE(buffer.Bytes()))

	var result [16]byte
	aesBlock, err := aes.NewCipher(vmessKDF16(cmdKey, kdfSaltAuthIDEncryptionKey))
	if err != nil {
		return result
	}
	aesBlock.Encrypt(result[:], buffer.Bytes())
	return result
}

// vmessKDF is the chained HMAC-SHA256 key derivation used by VMess.
func vmessKDF(key []byte, path ...string) []byte {
	creator := &vmessHMacCreator{value: []byte(kdfSaltVMessAEADKDF)}
	for _, element := range path {
		creator = &vmessHMacCreator{value: []byte(element), parent: creator}
	}
	mac := creator.create()
	mac.Write(key)
	return mac.Sum(nil)
}

func vmessKDF16(key []byte, path ...string) []byte {
	return vmessKDF(key, path...)[:16]
}

type vmessHMacCreator struct {
	parent *vmessHMacCreator
	value  []byte
}

func (h *vmessHMacCreator) create() hash.Hash {
	if h.parent == nil {
		return hmac.New(sha256.New, h.value)
	}
	return hmac.New(h.parent.create, h.value)
}

// chunkWriter writes the VMess body: [2 byte length][AEAD(payload)].
type chunkWriter struct {
	w     io.Writer
	aead  cipher.AEAD
	nonce [16]byte
	count uint16
}

func newChunkWriter(w io.Writer, aead cipher.AEAD, iv [16]byte) *chunkWriter {
	return &chunkWriter{w: w, aead: aead, nonce: iv}
}

func (w *chunkWriter) Write(payload []byte) (int, error) {
	written := 0
	for len(payload) > 0 {
		size := len(payload)
		if size > vmessChunkSize-w.aead.Overhead()-2 {
			size = vmessChunkSize - w.aead.Overhead() - 2
		}
		chunk := payload[:size]
		payload = payload[size:]

		nonce := w.nextNonce()
		length := make([]byte, 2)
		binary.BigEndian.PutUint16(length, uint16(len(chunk)+w.aead.Overhead()))
		if _, err := w.w.Write(length); err != nil {
			return written, err
		}
		sealed := w.aead.Seal(nil, nonce, chunk, nil)
		if _, err := w.w.Write(sealed); err != nil {
			return written, err
		}
		written += size
	}
	return written, nil
}

func (w *chunkWriter) nextNonce() []byte {
	nonce := append([]byte(nil), w.nonce[:]...)
	binary.BigEndian.PutUint16(nonce, w.count)
	w.count++
	return nonce[:w.aead.NonceSize()]
}

// chunkReader reads the VMess body, transparently consuming the encrypted
// response header on first use.
type chunkReader struct {
	r     io.Reader
	aead  cipher.AEAD
	nonce [16]byte
	count uint16

	responseKey [16]byte
	responseIV  [16]byte
	expected    byte
	headerRead  bool
	pending     []byte
}

func newChunkReader(r io.Reader, aead cipher.AEAD, iv [16]byte) *chunkReader {
	return &chunkReader{r: r, aead: aead, nonce: iv}
}

// ensureResponseHeader reads and validates the encrypted response header.
func (r *chunkReader) ensureResponseHeader() error {
	if r.headerRead {
		return nil
	}
	r.headerRead = true

	lengthKey := vmessKDF16(r.responseKey[:], kdfSaltResponseLengthKey)
	lengthIV := vmessKDF(r.responseIV[:], kdfSaltResponseLengthIV)[:12]
	lengthAEAD, err := aesGCM(lengthKey)
	if err != nil {
		return err
	}
	encryptedLength := make([]byte, 18)
	if _, err := io.ReadFull(r.r, encryptedLength); err != nil {
		return fmt.Errorf("read vmess response length: %w", err)
	}
	decryptedLength, err := lengthAEAD.Open(nil, lengthIV, encryptedLength, nil)
	if err != nil {
		return fmt.Errorf("decrypt vmess response length: %w", err)
	}
	if len(decryptedLength) != 2 {
		return errors.New("unexpected vmess response length size")
	}
	size := int(binary.BigEndian.Uint16(decryptedLength))

	payloadKey := vmessKDF16(r.responseKey[:], kdfSaltResponseHeaderKey)
	payloadIV := vmessKDF(r.responseIV[:], kdfSaltResponseHeaderIV)[:12]
	payloadAEAD, err := aesGCM(payloadKey)
	if err != nil {
		return err
	}
	encryptedPayload := make([]byte, size+16)
	if _, err := io.ReadFull(r.r, encryptedPayload); err != nil {
		return fmt.Errorf("read vmess response header: %w", err)
	}
	header, err := payloadAEAD.Open(nil, payloadIV, encryptedPayload, nil)
	if err != nil {
		return fmt.Errorf("decrypt vmess response header: %w", err)
	}
	if len(header) < 4 {
		return fmt.Errorf("truncated vmess response header: %d bytes", len(header))
	}
	if header[0] != r.expected {
		return fmt.Errorf("unexpected vmess response header: got 0x%02x, want 0x%02x", header[0], r.expected)
	}
	return nil
}

func (r *chunkReader) Read(out []byte) (int, error) {
	for len(r.pending) == 0 {
		if err := r.readChunk(); err != nil {
			return 0, err
		}
	}
	n := copy(out, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *chunkReader) readChunk() error {
	length := make([]byte, 2)
	if _, err := io.ReadFull(r.r, length); err != nil {
		return err
	}
	size := int(binary.BigEndian.Uint16(length))
	if size > vmessChunkSize {
		return fmt.Errorf("oversized vmess chunk: %d", size)
	}

	sealed := make([]byte, size)
	if _, err := io.ReadFull(r.r, sealed); err != nil {
		return err
	}
	nonce := append([]byte(nil), r.nonce[:]...)
	binary.BigEndian.PutUint16(nonce, r.count)
	r.count++

	plain, err := r.aead.Open(nil, nonce[:r.aead.NonceSize()], sealed, nil)
	if err != nil {
		return fmt.Errorf("decrypt vmess chunk: %w", err)
	}
	if len(plain) == 0 {
		return io.EOF
	}
	r.pending = plain
	return nil
}

func aesGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
