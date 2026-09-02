package parser

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/domain"
)

var uriCandidate = regexp.MustCompile(`(?i)(?:vmess|vless|trojan|ssr|ss|hysteria2|hysteria|hy2|tuic|wireguard|warp|slipnet|brook|naive|ssh|socks5|socks|https?|tg|mtproto|openvpn|naiveproxy|argo)://[^\s<>'"]+`)
var argoBlock = regexp.MustCompile(`(?s)-----BEGIN ARGO VPN BRIDGE BLOCK-----.*?-----END ARGO VPN BRIDGE BLOCK-----`)

// Multiline protocol blocks
var openVPNBlock = regexp.MustCompile(`(?s)<ca>.*?</ca>|-----BEGIN.*?-----END|<tls-auth>.*?</tls-auth>`)
var wireguardBlock = regexp.MustCompile(`(?s)\[Interface\].*?\[Peer\]|PrivateKey.*?=.*?|PublicKey.*?=.*?|Endpoint.*?=.*?`)

// Single-line patterns for multiline protocols
var openVPNURL = regexp.MustCompile(`(?i)openvpn://[^\s<>'"]+`)
var wireguardURL = regexp.MustCompile(`(?i)wireguard://[^\s<>'"]+`)

type Rejection struct {
	Value  string
	Reason string
}

// Extract returns only syntactically valid, normalized protocol candidates.
// Unknown URL schemes are intentionally not emitted.
// For multiline protocols (OpenVPN, WireGuard), it attempts to extract complete blocks.
func Extract(text string, keepUnknown bool) ([]domain.ParsedConfig, []Rejection) {
	text = html.UnescapeString(text)
	seen := make(map[string]bool)
	configs := make([]domain.ParsedConfig, 0)
	rejected := make([]Rejection, 0)

	// Extract Argo blocks
	for _, block := range argoBlock.FindAllString(text, -1) {
		parsed, err := Parse(block, keepUnknown)
		if err != nil {
			rejected = append(rejected, Rejection{Value: block, Reason: err.Error()})
			continue
		}
		if !seen[parsed.Fingerprint] {
			seen[parsed.Fingerprint] = true
			configs = append(configs, parsed)
		}
	}
	text = argoBlock.ReplaceAllString(text, "")

	// Extract OpenVPN blocks (multiline)
	for _, block := range openVPNBlock.FindAllString(text, -1) {
		// For now, we'll skip detailed OpenVPN parsing as it's complex
		// Just mark it as a candidate if it looks like OpenVPN
		if strings.Contains(strings.ToLower(block), "openvpn") ||
			strings.Contains(strings.ToLower(block), "<ca>") ||
			strings.Contains(strings.ToLower(block), "tls-auth") {
			// Create a fingerprint based on the block content
			hash := sha256.Sum256([]byte(block))
			fingerprint := hex.EncodeToString(hash[:])
			if !seen[fingerprint] && keepUnknown {
				seen[fingerprint] = true
				configs = append(configs, domain.ParsedConfig{
					Value:       block,
					Protocol:    domain.ProtocolOpenVPN,
					Canonical:   block,
					Fingerprint: fingerprint,
				})
			}
		}
	}
	text = openVPNBlock.ReplaceAllString(text, "")

	// Extract WireGuard blocks (multiline)
	for _, block := range wireguardBlock.FindAllString(text, -1) {
		// For now, we'll skip detailed WireGuard parsing as it's complex
		// Just mark it as a candidate if it looks like WireGuard
		if strings.Contains(strings.ToLower(block), "interface") ||
			strings.Contains(strings.ToLower(block), "privatekey") ||
			strings.Contains(strings.ToLower(block), "publickey") ||
			strings.Contains(strings.ToLower(block), "endpoint") {
			// Create a fingerprint based on the block content
			hash := sha256.Sum256([]byte(block))
			fingerprint := hex.EncodeToString(hash[:])
			if !seen[fingerprint] && keepUnknown {
				seen[fingerprint] = true
				configs = append(configs, domain.ParsedConfig{
					Value:       block,
					Protocol:    domain.ProtocolWireGuard,
					Canonical:   block,
					Fingerprint: fingerprint,
				})
			}
		}
	}
	text = wireguardBlock.ReplaceAllString(text, "")

	// Extract single-line URI candidates
	for _, candidate := range uriCandidate.FindAllString(text, -1) {
		candidate = trimCandidate(candidate)
		parsed, err := Parse(candidate, keepUnknown)
		if err != nil {
			rejected = append(rejected, Rejection{Value: candidate, Reason: err.Error()})
			continue
		}
		if !seen[parsed.Fingerprint] {
			seen[parsed.Fingerprint] = true
			configs = append(configs, parsed)
		}
	}

	// Also check for OpenVPN and WireGuard URLs
	for _, candidate := range openVPNURL.FindAllString(text, -1) {
		candidate = trimCandidate(candidate)
		parsed, err := Parse(candidate, keepUnknown)
		if err != nil {
			rejected = append(rejected, Rejection{Value: candidate, Reason: err.Error()})
			continue
		}
		if !seen[parsed.Fingerprint] {
			seen[parsed.Fingerprint] = true
			configs = append(configs, parsed)
		}
	}

	for _, candidate := range wireguardURL.FindAllString(text, -1) {
		candidate = trimCandidate(candidate)
		parsed, err := Parse(candidate, keepUnknown)
		if err != nil {
			rejected = append(rejected, Rejection{Value: candidate, Reason: err.Error()})
			continue
		}
		if !seen[parsed.Fingerprint] {
			seen[parsed.Fingerprint] = true
			configs = append(configs, parsed)
		}
	}

	return configs, rejected
}

func Parse(raw string, keepUnknown bool) (domain.ParsedConfig, error) {
	value := strings.TrimSpace(html.UnescapeString(raw))
	if value == "" {
		return domain.ParsedConfig{}, fmt.Errorf("empty candidate")
	}
	// Increase size limit for multiline configs (OpenVPN, WireGuard can be large)
	if len(value) > 65536 { // 64KB limit for multiline configs
		return domain.ParsedConfig{}, fmt.Errorf("oversized candidate")
	}
	protocol := detect(value)
	if protocol == domain.ProtocolUnknown && !keepUnknown {
		return domain.ParsedConfig{}, fmt.Errorf("unknown protocol")
	}
	canonical, err := canonicalize(value, protocol)
	if err != nil {
		return domain.ParsedConfig{}, err
	}
	sum := sha256.Sum256([]byte(canonical))
	return domain.ParsedConfig{Value: value, Protocol: protocol, Canonical: canonical, Fingerprint: hex.EncodeToString(sum[:])}, nil
}

func detect(value string) domain.Protocol {
	lower := strings.ToLower(value)

	// Check for multiline protocols first
	if strings.HasPrefix(value, "-----BEGIN ARGO VPN BRIDGE BLOCK-----") {
		return domain.ProtocolArgo
	}

	// Check for OpenVPN multiline config
	if strings.Contains(lower, "<ca>") ||
		strings.Contains(lower, "</ca>") ||
		strings.Contains(lower, "tls-auth") ||
		strings.Contains(lower, "tls-crypt") ||
		strings.Contains(lower, "client") && strings.Contains(lower, "dev tun") {
		return domain.ProtocolOpenVPN
	}

	// Check for WireGuard multiline config
	if strings.Contains(lower, "[interface]") ||
		strings.Contains(lower, "[peer]") ||
		(strings.Contains(lower, "privatekey") && strings.Contains(lower, "publickey")) {
		return domain.ProtocolWireGuard
	}

	// Check for single-line URL protocols
	switch {
	case strings.HasPrefix(lower, "vmess://"):
		return domain.ProtocolVMess
	case strings.HasPrefix(lower, "vless://"):
		return domain.ProtocolVLESS
	case strings.HasPrefix(lower, "trojan://"):
		return domain.ProtocolTrojan
	case strings.HasPrefix(lower, "ssr://"):
		return domain.ProtocolShadowsocksR
	case strings.HasPrefix(lower, "ss://"):
		return domain.ProtocolShadowsocks
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return domain.ProtocolHysteria2
	case strings.HasPrefix(lower, "hysteria://"):
		return domain.ProtocolHysteria
	case strings.HasPrefix(lower, "tuic://"):
		return domain.ProtocolTUIC
	case strings.HasPrefix(lower, "wireguard://"):
		return domain.ProtocolWireGuard
	case strings.HasPrefix(lower, "warp://"):
		return domain.ProtocolWARP
	case strings.HasPrefix(lower, "slipnet://"):
		return domain.ProtocolSlipnet
	case strings.HasPrefix(lower, "brook://"):
		return domain.ProtocolBrook
	case strings.HasPrefix(lower, "naive://"):
		return domain.ProtocolNaiveProxy
	case strings.HasPrefix(lower, "ssh://"):
		return domain.ProtocolSSH
	case strings.HasPrefix(lower, "socks5://"):
		return domain.ProtocolSOCKS5
	case strings.HasPrefix(lower, "socks://"):
		return domain.ProtocolSOCKS
	case strings.HasPrefix(lower, "tg://proxy?") || strings.HasPrefix(lower, "https://t.me/proxy?"):
		return domain.ProtocolMTProto
	case strings.HasPrefix(lower, "tg://socks?") || strings.HasPrefix(lower, "https://t.me/socks?"):
		return domain.ProtocolTelegramSOCKS
	case strings.HasPrefix(lower, "https://"):
		return domain.ProtocolHTTPS
	case strings.HasPrefix(lower, "http://"):
		return domain.ProtocolHTTP
	case strings.HasPrefix(lower, "openvpn://"):
		return domain.ProtocolOpenVPN
	default:
		return domain.ProtocolUnknown
	}
}

func canonicalize(value string, protocol domain.Protocol) (string, error) {
	if protocol == domain.ProtocolArgo {
		return value, nil
	}
	if protocol == domain.ProtocolVMess {
		return canonicalVMess(value)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if protocol == domain.ProtocolMTProto || protocol == domain.ProtocolTelegramSOCKS {
		return canonicalTelegramProxy(parsed, protocol)
	}
	if protocol == domain.ProtocolUnknown {
		return "", fmt.Errorf("unsupported protocol")
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("missing host")
	}
	if err := validatePort(parsed.Port()); err != nil {
		return "", err
	}
	query := parsed.Query()
	for _, key := range []string{"remarks", "remark", "ps", "name"} {
		query.Del(key)
	}
	if isTruthy(query.Get("allowInsecure")) || isTruthy(query.Get("insecure")) || isTruthy(query.Get("allow_insecure")) {
		return "", fmt.Errorf("insecure transport is not accepted")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.RawQuery = sortedQuery(query)
	return parsed.String(), nil
}

func canonicalTelegramProxy(parsed *url.URL, protocol domain.Protocol) (string, error) {
	query := parsed.Query()
	server := strings.ToLower(strings.TrimSpace(query.Get("server")))
	if server == "" {
		return "", fmt.Errorf("Telegram proxy missing server")
	}
	query.Set("server", server)
	port := query.Get("port")
	if err := validatePort(port); err != nil {
		return "", err
	}
	if protocol == domain.ProtocolMTProto && strings.TrimSpace(query.Get("secret")) == "" {
		return "", fmt.Errorf("MTProto proxy missing secret")
	}
	query.Del("user")
	query.Del("remarks")
	query.Del("name")
	scheme := "tg://proxy"
	if protocol == domain.ProtocolTelegramSOCKS {
		scheme = "tg://socks"
	}
	return scheme + "?" + sortedQuery(query), nil
}

func canonicalVMess(value string) (string, error) {
	payload := strings.TrimPrefix(value, "vmess://")
	decoded, err := decodeBase64(payload)
	if err != nil {
		return "", fmt.Errorf("invalid VMess base64")
	}
	var fields map[string]any
	if err := json.Unmarshal(decoded, &fields); err != nil {
		return "", fmt.Errorf("invalid VMess JSON")
	}
	for _, key := range []string{"add", "port", "id"} {
		if fieldString(fields, key) == "" {
			return "", fmt.Errorf("VMess missing %s", key)
		}
	}
	port := fieldString(fields, "port")
	if err := validatePort(port); err != nil {
		return "", err
	}
	if isTruthy(fieldString(fields, "allowInsecure")) || isTruthy(fieldString(fields, "insecure")) {
		return "", fmt.Errorf("insecure transport is not accepted")
	}
	keys := []string{"add", "port", "id", "aid", "net", "type", "host", "path", "tls", "sni", "alpn", "fp"}
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		val := strings.ToLower(fieldString(fields, key))
		// Only include non-empty values
		if val != "" {
			values = append(values, key+"="+val)
		}
	}
	return strings.Join(values, "&"), nil
}

func fieldString(fields map[string]any, key string) string {
	value, ok := fields[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	for _, decoder := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := decoder.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("base64 decode failed")
}

func sortedQuery(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0)
	for _, key := range keys {
		items := append([]string(nil), values[key]...)
		sort.Strings(items)
		for _, item := range items {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(item))
		}
	}
	return strings.Join(parts, "&")
}

func validatePort(raw string) error {
	if raw == "" {
		return fmt.Errorf("missing port")
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port")
	}
	return nil
}

func isTruthy(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

func trimCandidate(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), ".,;:)]}>\"")
}

func IsIPAddressOrHostname(value string) bool {
	return net.ParseIP(value) != nil || strings.Contains(value, ".")
}

// DecodeBase64Text supports the standard base64 variants used by subscription files.
func DecodeBase64Text(value string) (string, bool) {
	decoded, err := decodeBase64(value)
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	return string(decoded), true
}
