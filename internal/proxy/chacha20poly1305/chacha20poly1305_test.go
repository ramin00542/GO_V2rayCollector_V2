package chacha20poly1305

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestVectorsRFC8439 runs the AEAD test vector from RFC 8439 section 2.8.2.
func TestVectorsRFC8439(t *testing.T) {
	key := mustHex(t, "808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9f")
	nonce := mustHex(t, "070000004041424344454647")
	aad := mustHex(t, "50515253c0c1c2c3c4c5c6c7")
	plaintext := []byte("Ladies and Gentlemen of the class of '99: If I could offer you only one tip for the future, sunscreen would be it.")
	wantCiphertext := "d31a8d34648e60db7b86afbc53ef7ec2a4aded51296e08fea9e2b5a736ee62d63dbea45e8ca9671282fafb69da92728b1a71de0a9e060b2905d6a5b67ecd3b3692ddbd7f2d778b8c9803aee328091b58fab324e4fad675945585808b4831d7bc3ff4def08e4b7a9de576d26586cec64b6116" +
		"1ae10b594f09e26a7e902ecbd0600691"

	aead, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sealed := aead.Seal(nil, nonce, plaintext, aad)
	if hex.EncodeToString(sealed) != wantCiphertext {
		t.Fatalf("Seal mismatch:\n got %s\nwant %s", hex.EncodeToString(sealed), wantCiphertext)
	}

	opened, err := aead.Open(nil, nonce, sealed, aad)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("Open mismatch: got %q", opened)
	}

	// A modified tag must be rejected.
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := aead.Open(nil, nonce, tampered, aad); err == nil {
		t.Fatal("expected authentication failure for a tampered tag")
	}
}

func mustHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode %q: %v", value, err)
	}
	return decoded
}
