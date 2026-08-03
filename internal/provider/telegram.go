package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/RaminTabriz/V2rayCollector/internal/domain"
	"github.com/RaminTabriz/V2rayCollector/internal/fetch"
	"github.com/RaminTabriz/V2rayCollector/internal/parser"
)

type TelegramProvider struct {
	client        *fetch.Client
	limiter       *fetch.Limiter
	publicBaseURL string
	keepUnknown   bool
}

func NewTelegramProvider(client *fetch.Client, limiter *fetch.Limiter, keepUnknown bool) *TelegramProvider {
	return &TelegramProvider{client: client, limiter: limiter, publicBaseURL: "https://t.me/s", keepUnknown: keepUnknown}
}

// SetPublicBaseURL is intended for controlled tests or an explicitly configured public mirror.
func (p *TelegramProvider) SetPublicBaseURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("Telegram base URL must be HTTPS")
	}
	p.publicBaseURL = strings.TrimRight(rawURL, "/")
	return nil
}

func (p *TelegramProvider) Fetch(ctx context.Context, channel domain.Channel) domain.ProviderResult {
	result := domain.ProviderResult{SourceURL: channel.URL, SourceKind: domain.SourceTelegram}
	if p == nil || p.client == nil {
		result.Error = "Telegram provider is not initialized"
		return result
	}
	if !channel.Enabled {
		return result
	}
	name := strings.TrimSpace(channel.Name)
	if name == "" {
		result.Error = "channel name is empty"
		return result
	}
	response, err := p.client.Get(ctx, p.publicBaseURL+"/"+url.PathEscape(name), p.limiter)
	if err != nil {
		recordError(&result, err)
		return result
	}
	result.HTTPStatus = response.StatusCode
	result.BytesRead = len(response.Body)
	result.Discovered = DiscoverPublicLinks(string(response.Body))
	configs, rejected := parser.Extract(string(response.Body), p.keepUnknown)
	result.Configs = configs
	result.Accepted = len(configs)
	result.Extracted = len(configs) + len(rejected)
	result.Rejected = len(rejected)
	return result
}
