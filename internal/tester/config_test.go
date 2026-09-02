package tester

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestTestConfigUsesTheProxy proves that a config test tunnels through the
// proxy instead of connecting directly: the target server only accepts requests
// that arrive through the local CONNECT proxy.
func TestTestConfigUsesTheProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	// A CONNECT proxy that records whether it was used at all.
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen proxy: %v", err)
	}
	defer proxyListener.Close()

	tunneled := make(chan struct{}, 1)
	go func() {
		conn, err := proxyListener.Accept()
		if err != nil {
			return
		}
		reader := bufio.NewReader(conn)
		request, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return
		}
		if !strings.HasPrefix(request, "CONNECT ") {
			conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n\r\n"))
			conn.Close()
			return
		}
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				conn.Close()
				return
			}
			if strings.TrimRight(line, "\r\n") == "" {
				break
			}
		}
		upstream, err := net.Dial("tcp", strings.TrimPrefix(target.URL, "http://"))
		if err != nil {
			conn.Close()
			return
		}
		tunneled <- struct{}{}
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
			conn.Close()
			return
		}
		go func() { _, _ = io.Copy(upstream, reader); upstream.Close() }()
		_, _ = io.Copy(conn, upstream)
		conn.Close()
	}()

	site := TargetSite{Name: "Local", URL: target.URL, Category: "test", ExpectedStatus: 200, TimeoutSeconds: 5}
	settings := TestSettings{MaxConcurrentTests: 1, RequestTimeout: 5, RetryCount: 0, UserAgent: "tester/1.0"}

	result := TestConfig(context.Background(), "http://"+proxyListener.Addr().String(), []TargetSite{site}, settings)
	if !result.IsValid {
		t.Fatalf("config should be valid: %s", result.ValidationErr)
	}
	if result.TotalTested != 1 || result.TotalSuccess != 1 {
		t.Fatalf("expected the site to be reachable through the proxy, got tested=%d success=%d (%v)",
			result.TotalTested, result.TotalSuccess, result.SiteResults)
	}
	select {
	case <-tunneled:
	case <-time.After(5 * time.Second):
		t.Fatal("the proxy was never used: the test connected directly")
	}
}

// TestUnsupportedConfigIsSkipped checks that a config we cannot exercise is
// reported as skipped instead of being counted as a failure or a success.
func TestUnsupportedConfigIsSkipped(t *testing.T) {
	settings := TestSettings{MaxConcurrentTests: 1, RequestTimeout: 1, RetryCount: 0, UserAgent: "tester/1.0"}
	sites := []TargetSite{{Name: "Local", URL: "https://example.com", Category: "test", ExpectedStatus: 200, TimeoutSeconds: 1}}

	result := TestConfig(context.Background(), "hysteria2://password@example.com:443", sites, settings)
	if result.SkipReason == "" {
		t.Fatal("expected a skip reason for a QUIC based config")
	}
	if result.TotalTested != 0 || result.TotalSuccess != 0 {
		t.Fatalf("skipped configs must not be tested: tested=%d success=%d", result.TotalTested, result.TotalSuccess)
	}
	if !result.IsValid {
		t.Fatal("skipped configs are still valid links")
	}
}

// TestInvalidConfigIsRejected keeps the parser behaviour intact.
func TestInvalidConfigIsRejected(t *testing.T) {
	settings := TestSettings{MaxConcurrentTests: 1, RequestTimeout: 1, RetryCount: 0, UserAgent: "tester/1.0"}
	result := TestConfig(context.Background(), "not-a-proxy-link", nil, settings)
	if result.IsValid {
		t.Fatal("expected an invalid config")
	}
	if result.ValidationErr == "" {
		t.Fatal("expected a validation error")
	}
}

// TestReportShape keeps the JSON contract the dashboard relies on.
func TestReportShape(t *testing.T) {
	report := GenerateReport([]ConfigTestResult{
		{
			ConfigValue:  "http://proxy.example:8080",
			ConfigType:   "http",
			IsValid:      true,
			SiteResults:  map[string]SiteResult{"Local": {Success: true, StatusCode: 200, Latency: time.Millisecond}},
			TotalTested:  1,
			TotalSuccess: 1,
		},
	}, []TargetSite{{Name: "Local", URL: "http://local.test", Category: "test", ExpectedStatus: 200, TimeoutSeconds: 1}})

	if report.TotalConfigs != 1 || report.ValidConfigs != 1 || report.WorkingConfigs != 1 {
		t.Fatalf("unexpected report counters: %+v", report)
	}
	stats, ok := report.SiteStatistics["Local"]
	if !ok || stats.TotalSuccess != 1 || stats.SuccessRate != 100 {
		t.Fatalf("unexpected site statistics: %+v", report.SiteStatistics)
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	for _, key := range []string{"total_configs", "valid_configs", "working_configs", "config_results", "site_statistics"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("report is missing %q", key)
		}
	}
}
