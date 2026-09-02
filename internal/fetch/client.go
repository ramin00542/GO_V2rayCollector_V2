package fetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	Timeout        time.Duration
	HeaderTimeout  time.Duration
	MaxBodyBytes   int64
	MaxRedirects   int
	UserAgent      string
	ProxyURL       string
	Retries        int
	RetryBaseDelay time.Duration
}

type Response struct {
	URL        string
	StatusCode int
	Header     http.Header
	Body       []byte
}

type HTTPError struct {
	StatusCode int
	URL        string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.StatusCode) }

type Client struct {
	httpClient *http.Client
	maxBody    int64
	userAgent  string
	retries    int
	retryDelay time.Duration
}

func DefaultConfig() Config {
	return Config{
		Timeout:        25 * time.Second,
		HeaderTimeout:  10 * time.Second,
		MaxBodyBytes:   8 << 20,
		MaxRedirects:   3,
		UserAgent:      "V2rayCollector/2.0 (public-source collector)",
		Retries:        2,
		RetryBaseDelay: time.Second,
	}
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 || cfg.HeaderTimeout <= 0 || cfg.MaxBodyBytes < 1 || cfg.MaxRedirects < 0 || cfg.Retries < 0 || cfg.RetryBaseDelay <= 0 {
		return nil, fmt.Errorf("invalid fetch configuration")
	}
	if cfg.UserAgent == "" {
		return nil, fmt.Errorf("user agent is required")
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: cfg.HeaderTimeout,
	}
	if cfg.ProxyURL != "" {
		proxy, err := url.Parse(cfg.ProxyURL)
		if err != nil || proxy.Scheme == "" || proxy.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > cfg.MaxRedirects {
				return http.ErrUseLastResponse
			}
			// Allow HTTP redirects only if the original URL was HTTP
			// or if it's a same-origin redirect (e.g., http://example.com -> http://example.com/path)
			if request.URL.Scheme != "https" {
				// Check if this is a same-origin redirect
				if len(via) > 0 {
					originalURL := via[0].URL
					if originalURL.Scheme == "http" && originalURL.Host == request.URL.Host {
						return nil // Allow same-origin HTTP redirect
					}
				}
				return fmt.Errorf("redirect to non-HTTPS URL rejected: %s", request.URL.String())
			}
			return nil
		},
	}
	return &Client{httpClient: client, maxBody: cfg.MaxBodyBytes, userAgent: cfg.UserAgent, retries: cfg.Retries, retryDelay: cfg.RetryBaseDelay}, nil
}

func (c *Client) Get(ctx context.Context, rawURL string, limiter *Limiter) (Response, error) {
	if c == nil {
		return Response{}, fmt.Errorf("nil fetch client")
	}
	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return Response{}, err
		}
	}
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		response, retry, err := c.getOnce(ctx, rawURL)
		if err == nil {
			return response, nil
		}
		lastErr = err
		if !retry || attempt == c.retries {
			break
		}
		delay := c.retryDelay * time.Duration(1<<attempt)
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-time.After(delay):
		}
	}
	return Response{}, lastErr
}

// Do executes an arbitrary HTTP request with the configured client.
// It exists for API calls that need methods other than GET (for example the
// PUT/POST requests used by CDN providers). Unlike Get, the raw
// *http.Response is returned, so the caller is responsible for closing the
// response body.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c == nil {
		return nil, fmt.Errorf("nil fetch client")
	}
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if req.URL != nil && strings.HasPrefix(req.URL.String(), "https://api.github.com/") && req.Header.Get("Authorization") == "" {
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		}
	}
	return c.httpClient.Do(req)
}

func (c *Client) getOnce(ctx context.Context, rawURL string) (Response, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Response{}, false, err
	}
	request.Header.Set("User-Agent", c.userAgent)
	request.Header.Set("Accept", "text/plain,text/html,application/json;q=0.9,*/*;q=0.1")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" && strings.HasPrefix(rawURL, "https://api.github.com/") {
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		// Check if it's a redirect error
		if urlErr, ok := err.(*url.Error); ok {
			if opErr, ok := urlErr.Err.(*net.OpError); ok {
				// This might be a redirect limit error
				if opErr.Op == "http" && strings.Contains(opErr.Net, "redirect") {
					return Response{}, false, &HTTPError{StatusCode: http.StatusTooManyRequests, URL: rawURL}
				}
			}
		}
		return Response{}, true, err
	}
	defer response.Body.Close()

	// Follow redirects automatically for 3xx status codes
	// The http.Client with CheckRedirect already handles this, so we shouldn't get 3xx here
	// But if we do, treat it as retryable
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return Response{}, true, &HTTPError{StatusCode: response.StatusCode, URL: rawURL}
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return Response{}, response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500, &HTTPError{StatusCode: response.StatusCode, URL: rawURL}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxBody+1))
	if err != nil {
		return Response{}, true, err
	}
	if int64(len(body)) > c.maxBody {
		return Response{}, false, fmt.Errorf("GET %s: response exceeds %d bytes", rawURL, c.maxBody)
	}
	return Response{URL: response.Request.URL.String(), StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: body}, false, nil
}
