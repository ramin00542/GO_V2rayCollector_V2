package provider

import (
	"regexp"
	"strings"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/repository"
)

var telegramLink = regexp.MustCompile(`(?i)https?://t\.me/(?:s/)?[a-zA-Z0-9_]+`)
var httpsLink = regexp.MustCompile(`https://[^\s<>'"]+`)

// DiscoverPublicLinks extracts public references only; validation happens before promotion.
func DiscoverPublicLinks(text string) []domain.DiscoveredLink {
	seen := map[string]bool{}
	out := []domain.DiscoveredLink{}
	for _, raw := range telegramLink.FindAllString(text, -1) {
		name := repository.NormalizeTelegramChannel(raw)
		if name != "" && !seen["c:"+name] {
			seen["c:"+name] = true
			out = append(out, domain.DiscoveredLink{Kind: domain.DiscoveryChannel, Value: name})
		}
	}
	for _, raw := range httpsLink.FindAllString(text, -1) {
		raw = strings.TrimRight(raw, ".,;:)]}>\"")
		if strings.Contains(strings.ToLower(raw), "t.me/") {
			continue
		}
		if normalized, ok := repository.NormalizeSourceURL(raw); ok && !seen["s:"+normalized] {
			seen["s:"+normalized] = true
			out = append(out, domain.DiscoveredLink{Kind: domain.DiscoverySource, Value: normalized})
		}
	}
	return out
}
