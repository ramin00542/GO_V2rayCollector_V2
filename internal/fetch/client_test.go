package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("12345"))
	}))
	defer server.Close()
	config := DefaultConfig()
	config.MaxBodyBytes = 4
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), server.URL, nil); err == nil {
		t.Fatal("oversized body was accepted")
	}
}

func TestLimiterHonorsCancellation(t *testing.T) {
	limiter, err := NewLimiter(1)
	if err != nil {
		t.Fatal(err)
	}
	defer limiter.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := limiter.Wait(ctx); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("unexpected cancellation error: %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("cancelled limiter waited")
	}
}

