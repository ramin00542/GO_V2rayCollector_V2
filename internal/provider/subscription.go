package provider

import (
	"context"
	"strings"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/fetch"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/parser"
)

type SubscriptionProvider struct {
	client      *fetch.Client
	limiter     *fetch.Limiter
	keepUnknown bool
}

func NewSubscriptionProvider(client *fetch.Client, limiter *fetch.Limiter, keepUnknown bool) *SubscriptionProvider {
	return &SubscriptionProvider{client: client, limiter: limiter, keepUnknown: keepUnknown}
}

func (p *SubscriptionProvider) Fetch(ctx context.Context, source domain.Source) domain.ProviderResult {
	result := domain.ProviderResult{SourceURL: source.URL, SourceKind: source.Kind}
	if p == nil || p.client == nil {
		result.Error = "subscription provider is not initialized"
		return result
	}
	if !source.Enabled {
		return result
	}
	response, err := p.client.Get(ctx, source.URL, p.limiter)
	if err != nil {
		recordError(&result, err)
		return result
	}
	result.HTTPStatus = response.StatusCode
	result.BytesRead = len(response.Body)
	result.Discovered = DiscoverPublicLinks(string(response.Body))
	configs, rejected := parser.Extract(string(response.Body), p.keepUnknown)
	// Base64 is a fallback only. A normal source is never replaced merely because
	// its raw text happens to also be valid base64.
	if len(configs) == 0 {
		if decoded, ok := parser.DecodeBase64Text(strings.TrimSpace(string(response.Body))); ok {
			configs, rejected = parser.Extract(decoded, p.keepUnknown)
		}
	}
	result.Configs = configs
	result.Accepted = len(configs)
	result.Extracted = len(configs) + len(rejected)
	result.Rejected = len(rejected)
	return result
}

