package provider

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/fetch"
)

func TestSubscriptionUsesBase64OnlyAsFallback(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("vless://id@example.com:443?security=tls"))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(content))
	}))
	defer server.Close()
	client, err := fetch.NewClient(fetch.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	provider := NewSubscriptionProvider(client, nil, false)
	result := provider.Fetch(context.Background(), domain.Source{URL: server.URL, Enabled: true, Kind: domain.SourceSubscription})
	if result.Error != "" || result.Accepted != 1 || result.Configs[0].Protocol != domain.ProtocolVLESS {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSummary(t *testing.T) {
	summary := Summarize([]domain.ProviderResult{{Accepted: 2}, {Error: "network", Rejected: 1}})
	if summary.Requests != 2 || summary.Succeeded != 1 || summary.Failed != 1 || summary.Accepted != 2 || summary.Rejected != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

