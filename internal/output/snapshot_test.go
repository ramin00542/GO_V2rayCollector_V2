package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/state"
)

func TestPublishSeparatesHTTPSAndTelegramProxy(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	entries := []state.Entry{
		{Value: "https://proxy.example:443", Protocol: domain.ProtocolHTTPS, Fingerprint: "https", Observations: map[string]state.Observation{"telegram:x": {Kind: domain.SourceTelegram, Channel: "x", LastSeenAt: now}}},
		{Value: "tg://proxy?server=x&port=443&secret=a", Protocol: domain.ProtocolMTProto, Fingerprint: "mt", Observations: map[string]state.Observation{"telegram:x": {Kind: domain.SourceTelegram, Channel: "x", LastSeenAt: now}}},
	}
	root := filepath.Join(t.TempDir(), "temporary")
	start, end := DayBounds(now)
	if err := Publish(root, entries, start, end, SnapshotOptions{WritePerChannel: true}); err != nil {
		t.Fatal(err)
	}
	https, err := os.ReadFile(filepath.Join(root, "telegram", "protocols", "https.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(https), "https://proxy.example") {
		t.Fatal("HTTPS config not in dedicated file")
	}
	mtproto, err := os.ReadFile(filepath.Join(root, "telegram", "telegram-proxies", "mtproto.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mtproto), "tg://proxy") {
		t.Fatal("MTProto config not in proxy file")
	}
}
