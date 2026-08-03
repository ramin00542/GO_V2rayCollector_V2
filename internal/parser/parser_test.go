package parser

import (
	"encoding/base64"
	"testing"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
)

func TestVLESSFragmentDoesNotChangeFingerprint(t *testing.T) {
	first, err := Parse("vless://id@example.com:443?security=tls&type=ws#one", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Parse("vless://id@example.com:443?type=ws&security=tls#two", false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("display fragment/query ordering changed fingerprint")
	}
}

func TestVLESSEncryptionNoneIsNotRejected(t *testing.T) {
	parsed, err := Parse("vless://id@example.com:443?security=reality&encryption=none", false)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Protocol != domain.ProtocolVLESS {
		t.Fatal("wrong protocol")
	}
}

func TestInsecureTransportIsRejected(t *testing.T) {
	if _, err := Parse("trojan://secret@example.com:443?allowInsecure=true", false); err == nil {
		t.Fatal("insecure transport was accepted")
	}
}

func TestVMessURLSafeBase64(t *testing.T) {
	payload := `{"add":"example.com","port":"443","id":"abc","net":"ws","tls":"tls"}`
	value := "vmess://" + base64.RawURLEncoding.EncodeToString([]byte(payload))
	parsed, err := Parse(value, false)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Protocol != domain.ProtocolVMess {
		t.Fatal("wrong protocol")
	}
}

func TestArgoRemainsOneConfig(t *testing.T) {
	text := "-----BEGIN ARGO VPN BRIDGE BLOCK-----\nabc\n-----END ARGO VPN BRIDGE BLOCK-----"
	configs, rejected := Extract(text, false)
	if len(rejected) != 0 || len(configs) != 1 || configs[0].Protocol != domain.ProtocolArgo {
		t.Fatalf("unexpected extraction: configs=%d rejected=%d", len(configs), len(rejected))
	}
}
