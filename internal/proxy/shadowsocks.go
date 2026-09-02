package proxy

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // required by the shadowsocks key derivation
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/proxy/chacha20"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/proxy/chacha20poly1305"
)

// The maximum plaintext size of a single shadowsocks chunk.
const ssMaxChunkSize = 0x3FFF

// ssDialer implements the shadowsocks protocol for AEAD and legacy stream ciphers.
type ssDialer struct {
	address  string
	key      []byte
	saltSize int
	cipher   string
	aead     bool
}

// NewShadowsocksDialer builds a dialer for `ss://` links, both SIP002 and the
// legacy base64 form.
func NewShadowsocksDialer(link string) (Dialer, error) {
	method, password, address, err := parseShadowsocksLink(link)
	if err != nil {
		return nil, err
	}

	keySize, saltSize, aead, err := cipherInfo(method)
	if err != nil {
		return nil, err
	}

	return &ssDialer{
		address:  address,
		key:      evpBytesToKey(password, keySize),
		saltSize: saltSize,
		cipher:   method,
		aead:     aead,
	}, nil
}

func (d *ssDialer) Protocol() string { return "shadowsocks" }

func (d *ssDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := dialTCP(ctx, d.address)
	if err != nil {
		return nil, err
	}

	target, err := socks5AddressHostPort(addr)
	if err != nil {
		conn.Close()
		return nil, err
	}

	wrapped, err := newShadowsocksConn(conn, d.key, d.saltSize, d.cipher, d.aead, target)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return wrapped, nil
}

func socks5AddressHostPort(addr string) ([]byte, error) {
	host, port, err := splitAddr(addr)
	if err != nil {
		return nil, err
	}
	return socks5Address(host, port)
}

// cipherInfo returns the key length, the salt/IV length used on the wire and
// whether the method is an AEAD cipher. For stream ciphers the IV length is
// the block size of the underlying cipher, which differs from the key length.
func cipherInfo(method string) (keySize, saltSize int, aead bool, err error) {
	switch strings.ToLower(method) {
	case "aes-128-gcm":
		return 16, 16, true, nil
	case "aes-192-gcm":
		return 24, 24, true, nil
	case "aes-256-gcm":
		return 32, 32, true, nil
	case "chacha20-ietf-poly1305":
		return 32, 32, true, nil
	case "xchacha20-ietf-poly1305":
		return 32, 32, true, nil
	case "aes-128-cfb", "aes-128-ctr":
		return 16, 16, false, nil
	case "aes-192-cfb", "aes-192-ctr":
		return 24, 16, false, nil
	case "aes-256-cfb", "aes-256-ctr":
		return 32, 16, false, nil
	case "chacha20-ietf":
		return 32, 12, false, nil
	case "rc4-md5":
		return 16, 16, false, nil
	}
	return 0, 0, false, &UnsupportedError{
		Protocol: "shadowsocks",
		Reason:   fmt.Sprintf("cipher %q is not implemented", method),
	}
}

// evpBytesToKey derives a key the OpenSSL way, which is what shadowsocks uses.
func evpBytesToKey(password string, keyLen int) []byte {
	key := make([]byte, 0, keyLen)
	previous := []byte(nil)
	for len(key) < keyLen {
		digest := md5.New() //nolint:gosec // protocol requirement
		digest.Write(previous)
		digest.Write([]byte(password))
		previous = digest.Sum(nil)
		key = append(key, previous...)
	}
	return key[:keyLen]
}

// parseShadowsocksLink extracts method, password and server address from a link.
func parseShadowsocksLink(link string) (method, password, address string, err error) {
	raw := strings.TrimSpace(link)
	raw = strings.TrimPrefix(raw, "ss://")
	if raw == "" {
		return "", "", "", errors.New("empty shadowsocks link")
	}

	// Drop the fragment (the config name) before parsing.
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = raw[:idx]
	}

	if idx := strings.IndexByte(raw, '@'); idx >= 0 {
		// SIP002: userinfo holds the base64 encoded "method:password".
		userinfo := raw[:idx]
		hostPart := raw[idx+1:]
		decoded, err := decodeBase64Flexible(userinfo)
		if err != nil {
			return "", "", "", fmt.Errorf("decode shadowsocks userinfo: %w", err)
		}
		credentials := string(decoded)
		sep := strings.IndexByte(credentials, ':')
		if sep < 0 {
			return "", "", "", errors.New("malformed shadowsocks credentials")
		}
		method, password = credentials[:sep], credentials[sep+1:]
		address, err = normalizeHostPort(hostPart)
		if err != nil {
			return "", "", "", err
		}
		return method, password, address, nil
	}

	// Legacy: the whole payload is base64 encoded "method:password@host:port"
	// (optionally URL safe and without padding).
	decoded, err := decodeBase64Flexible(raw)
	if err != nil {
		return "", "", "", fmt.Errorf("decode legacy shadowsocks link: %w", err)
	}
	payload := string(decoded)
	at := strings.LastIndexByte(payload, '@')
	if at < 0 {
		return "", "", "", errors.New("malformed legacy shadowsocks link")
	}
	credentials := payload[:at]
	hostPart := payload[at+1:]
	sep := strings.IndexByte(credentials, ':')
	if sep < 0 {
		return "", "", "", errors.New("malformed shadowsocks credentials")
	}
	method, password = credentials[:sep], credentials[sep+1:]
	address, err = normalizeHostPort(hostPart)
	if err != nil {
		return "", "", "", err
	}
	return method, password, address, nil
}

// normalizeHostPort makes sure the address carries an explicit port.
func normalizeHostPort(value string) (string, error) {
	host, port, err := splitAddr(strings.Trim(strings.TrimSpace(value), "/[]"))
	if err != nil {
		return "", err
	}
	if port == 0 {
		return "", fmt.Errorf("shadowsocks server has no port: %q", value)
	}
	return joinHostPort(host, port), nil
}

// ssConn is a shadowsocks connection. It writes the target address once and
// then transparently en/decrypts the payload. Each direction uses its own
// salt (AEAD) or IV (stream ciphers), as required by SIP004.
type ssConn struct {
	net.Conn
	cipher    string
	aead      bool
	key       []byte
	saltSize  int
	request   []byte
	sent      bool
	encrypter io.Writer
	decrypter *ssLazyReader
}

func newShadowsocksConn(conn net.Conn, key []byte, saltSize int, method string, aead bool, request []byte) (net.Conn, error) {
	wrapped := &ssConn{
		Conn:      conn,
		cipher:    method,
		aead:      aead,
		key:       key,
		saltSize:  saltSize,
		request:   request,
		decrypter: &ssLazyReader{conn: conn, key: key, saltSize: saltSize, method: method, aead: aead},
	}
	if err := wrapped.initWriter(); err != nil {
		return nil, err
	}
	return wrapped, nil
}

// initWriter emits this direction's salt/IV and prepares the encrypter.
func (c *ssConn) initWriter() error {
	method := strings.ToLower(c.cipher)

	if !c.aead {
		iv := make([]byte, c.saltSize)
		if _, err := rand.Read(iv); err != nil {
			return err
		}
		stream, err := newSSStream(method, c.key, iv, true)
		if err != nil {
			return err
		}
		if _, err := c.Conn.Write(iv); err != nil {
			return fmt.Errorf("write shadowsocks iv: %w", err)
		}
		c.encrypter = &cipher.StreamWriter{S: stream, W: c.Conn}
		return nil
	}

	salt := make([]byte, c.saltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	sealer, _, err := ssAEADPair(method, c.key, salt)
	if err != nil {
		return err
	}
	if _, err := c.Conn.Write(salt); err != nil {
		return fmt.Errorf("write shadowsocks salt: %w", err)
	}
	c.encrypter = &ssAEADWriter{w: c.Conn, sealer: sealer}
	return nil
}

// newSSStream builds the stream cipher for one direction. CFB (unlike CTR,
// ChaCha20 and RC4) is not symmetric, so the direction has to be explicit.
func newSSStream(method string, key, iv []byte, encrypt bool) (cipher.Stream, error) {
	switch method {
	case "aes-128-cfb", "aes-192-cfb", "aes-256-cfb":
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		if encrypt {
			return cipher.NewCFBEncrypter(block, iv), nil
		}
		return cipher.NewCFBDecrypter(block, iv), nil
	case "aes-128-ctr", "aes-192-ctr", "aes-256-ctr":
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewCTR(block, iv), nil
	case "chacha20-ietf":
		stream, err := chacha20.NewUnauthenticatedCipher(key, iv)
		if err != nil {
			return nil, err
		}
		return stream, nil
	case "rc4-md5":
		digest := md5.New() //nolint:gosec // protocol requirement
		digest.Write(key)
		digest.Write(iv)
		return rc4.NewCipher(digest.Sum(nil)) //nolint:gosec // protocol requirement
	}
	return nil, fmt.Errorf("unsupported stream cipher %q", method)
}

// ssAEADPair creates the AEAD used for a connection along with the initial
// nonce derived from the salt.
func ssAEADPair(method string, key, salt []byte) (cipher.AEAD, cipher.AEAD, error) {
	subKey := make([]byte, len(key))
	hkdfSHA1(key, salt, []byte("ss-subkey"), subKey)

	switch method {
	case "aes-128-gcm", "aes-192-gcm", "aes-256-gcm":
		block, err := aes.NewCipher(subKey)
		if err != nil {
			return nil, nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, nil, err
		}
		return aead, aead, nil
	case "chacha20-ietf-poly1305":
		aead, err := chacha20poly1305.New(subKey)
		if err != nil {
			return nil, nil, err
		}
		return aead, aead, nil
	case "xchacha20-ietf-poly1305":
		aead, err := chacha20poly1305.NewX(subKey)
		if err != nil {
			return nil, nil, err
		}
		return aead, aead, nil
	}
	return nil, nil, fmt.Errorf("unsupported aead cipher %q", method)
}

// ssAEADWriter writes length prefixed, AEAD sealed chunks.
type ssAEADWriter struct {
	w      io.Writer
	sealer cipher.AEAD
	nonce  [12]byte
}

func (w *ssAEADWriter) Write(payload []byte) (int, error) {
	if len(payload) == 0 {
		return 0, nil
	}
	written := 0
	for len(payload) > 0 {
		size := len(payload)
		if size > ssMaxChunkSize {
			size = ssMaxChunkSize
		}
		chunk := payload[:size]
		payload = payload[size:]

		if err := w.writeChunk(chunk); err != nil {
			return written, err
		}
		written += size
	}
	return written, nil
}

func (w *ssAEADWriter) writeChunk(chunk []byte) error {
	length := make([]byte, 2+w.sealer.Overhead())
	length[0], length[1] = byte(len(chunk)>>8), byte(len(chunk))
	w.incrementNonce()
	w.sealer.Seal(length[:0], w.nonce[:w.sealer.NonceSize()], length[:2], nil)
	if _, err := w.w.Write(length); err != nil {
		return err
	}

	w.incrementNonce()
	sealed := w.sealer.Seal(nil, w.nonce[:w.sealer.NonceSize()], chunk, nil)
	if _, err := w.w.Write(sealed); err != nil {
		return err
	}
	return nil
}

func (w *ssAEADWriter) incrementNonce() {
	incrementNonce(w.nonce[:])
}

// ssAEADReader reads length prefixed AEAD chunks.
type ssAEADReader struct {
	r       io.Reader
	opener  cipher.AEAD
	nonce   [12]byte
	buffer  []byte
	pending []byte
}

func (r *ssAEADReader) Read(out []byte) (int, error) {
	if len(r.pending) == 0 {
		if err := r.readChunk(); err != nil {
			return 0, err
		}
	}
	n := copy(out, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *ssAEADReader) readChunk() error {
	sizeBuffer := make([]byte, 2+r.opener.Overhead())
	if _, err := io.ReadFull(r.r, sizeBuffer); err != nil {
		return err
	}
	r.incrementNonce()
	length, err := r.opener.Open(sizeBuffer[:0], r.nonce[:r.opener.NonceSize()], sizeBuffer, nil)
	if err != nil {
		return fmt.Errorf("decrypt chunk length: %w", err)
	}
	if len(length) != 2 {
		return errors.New("unexpected chunk length size")
	}
	size := int(length[0])<<8 | int(length[1])
	if size == 0 {
		return io.EOF
	}
	if size > ssMaxChunkSize+1 {
		return fmt.Errorf("oversized shadowsocks chunk: %d", size)
	}

	sealed := make([]byte, size+r.opener.Overhead())
	if _, err := io.ReadFull(r.r, sealed); err != nil {
		return err
	}
	r.incrementNonce()
	plain, err := r.opener.Open(nil, r.nonce[:r.opener.NonceSize()], sealed, nil)
	if err != nil {
		return fmt.Errorf("decrypt chunk: %w", err)
	}
	r.pending = plain
	return nil
}

func (r *ssAEADReader) incrementNonce() {
	incrementNonce(r.nonce[:])
}

// incrementNonce advances a big-endian counter, as required by SIP004.
func incrementNonce(nonce []byte) {
	for i := range nonce {
		nonce[i]++
		if nonce[i] != 0 {
			return
		}
	}
}

func (c *ssConn) Write(payload []byte) (int, error) {
	if !c.sent {
		c.sent = true
		if _, err := c.encrypter.Write(c.request); err != nil {
			return 0, err
		}
	}
	return c.encrypter.Write(payload)
}

func (c *ssConn) Read(buffer []byte) (int, error) {
	return c.decrypter.Read(buffer)
}

// ssLazyReader reads the peer salt/IV on the first read and then decrypts.
type ssLazyReader struct {
	conn     net.Conn
	key      []byte
	saltSize int
	method   string
	aead     bool
	once     sync.Once
	reader   io.Reader
	initErr  error
}

func (r *ssLazyReader) Read(buffer []byte) (int, error) {
	r.once.Do(func() { r.reader, r.initErr = r.init() })
	if r.initErr != nil {
		return 0, r.initErr
	}
	return r.reader.Read(buffer)
}

func (r *ssLazyReader) init() (io.Reader, error) {
	method := strings.ToLower(r.method)
	if !r.aead {
		iv := make([]byte, r.saltSize)
		if _, err := io.ReadFull(r.conn, iv); err != nil {
			return nil, fmt.Errorf("read shadowsocks iv: %w", err)
		}
		stream, err := newSSStream(method, r.key, iv, false)
		if err != nil {
			return nil, err
		}
		return &cipher.StreamReader{S: stream, R: r.conn}, nil
	}

	salt := make([]byte, r.saltSize)
	if _, err := io.ReadFull(r.conn, salt); err != nil {
		return nil, fmt.Errorf("read shadowsocks salt: %w", err)
	}
	_, opener, err := ssAEADPair(method, r.key, salt)
	if err != nil {
		return nil, err
	}
	return &ssAEADReader{r: r.conn, opener: opener}, nil
}

// hkdfSHA1 is the HKDF-SHA1 expansion used by shadowsocks to derive the
// per session sub key (SIP004). Keys longer than a SHA1 digest are produced by
// iterating the expansion, as RFC 5869 requires.
func hkdfSHA1(secret, salt, info []byte, out []byte) {
	if len(info) == 0 {
		info = []byte("ss-subkey")
	}

	// Extract.
	extractor := hmac.New(sha1.New, salt)
	extractor.Write(secret)
	prk := extractor.Sum(nil)

	// Expand.
	var block []byte
	counter := byte(1)
	for filled := 0; filled < len(out); {
		expander := hmac.New(sha1.New, prk)
		expander.Write(block)
		expander.Write(info)
		expander.Write([]byte{counter})
		block = expander.Sum(nil)
		counter++
		filled += copy(out[filled:], block)
	}
}
