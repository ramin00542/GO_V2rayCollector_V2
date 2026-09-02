package proxy

import (
	"encoding/hex"
	"testing"
)

// TestHKDFSHA1Vector pins the HKDF-SHA1 expansion used for the shadowsocks
// session sub key. The expected value was produced by independent RFC 5869
// implementations (Go and Python) so a regression here is caught immediately.
func TestHKDFSHA1Vector(t *testing.T) {
	secret := []byte("demo-password-key-material")
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i)
	}

	out := make([]byte, 32)
	hkdfSHA1(secret, salt, []byte("ss-subkey"), out)
	const want32 = "07d136d1135849f00215f6b01c542b46817b1ced608622b9b37a7d8e1f5a9f38"
	if hex.EncodeToString(out) != want32 {
		t.Fatalf("32 byte expansion mismatch:\n got %s\nwant %s", hex.EncodeToString(out), want32)
	}

	short := make([]byte, 16)
	hkdfSHA1(secret, salt, []byte("ss-subkey"), short)
	const want16 = "07d136d1135849f00215f6b01c542b46"
	if hex.EncodeToString(short) != want16 {
		t.Fatalf("16 byte expansion mismatch:\n got %s\nwant %s", hex.EncodeToString(short), want16)
	}
}

func TestParseShadowsocksLink(t *testing.T) {
	cases := []struct {
		name     string
		link     string
		method   string
		password string
		address  string
	}{
		{
			name:     "SIP002 with tag",
			link:     "ss://" + base64RawURL("aes-256-gcm:password") + "@example.com:8388#My Server",
			method:   "aes-256-gcm",
			password: "password",
			address:  "example.com:8388",
		},
		{
			name:     "SIP002 without tag",
			link:     "ss://" + base64RawURL("chacha20-ietf-poly1305:pa55w0rd") + "@1.2.3.4:443",
			method:   "chacha20-ietf-poly1305",
			password: "pa55w0rd",
			address:  "1.2.3.4:443",
		},
		{
			name:     "legacy base64 payload",
			link:     "ss://" + base64RawURL("aes-256-cfb:legacy@example.com:8080"),
			method:   "aes-256-cfb",
			password: "legacy",
			address:  "example.com:8080",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method, password, address, err := parseShadowsocksLink(tc.link)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if method != tc.method || password != tc.password || address != tc.address {
				t.Fatalf("got %q / %q / %q, want %q / %q / %q",
					method, password, address, tc.method, tc.password, tc.address)
			}
		})
	}
}

func TestCipherInfoSizes(t *testing.T) {
	// Stream ciphers use the block size as IV length, not the key length.
	cases := map[string][2]int{
		"aes-256-gcm":            {32, 32},
		"aes-128-gcm":            {16, 16},
		"chacha20-ietf-poly1305": {32, 32},
		"aes-256-cfb":            {32, 16},
		"aes-192-cfb":            {24, 16},
		"chacha20-ietf":          {32, 12},
		"rc4-md5":                {16, 16},
	}
	for method, want := range cases {
		keySize, saltSize, aead, err := cipherInfo(method)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		if keySize != want[0] || saltSize != want[1] {
			t.Fatalf("%s: got key=%d salt=%d, want key=%d salt=%d", method, keySize, saltSize, want[0], want[1])
		}
		_ = aead
	}
}
