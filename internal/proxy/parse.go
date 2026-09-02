package proxy

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Link is the normalized form of a proxy URI.
type Link struct {
	Scheme   string
	Host     string
	Port     int
	Username string // uuid for vless, password for trojan, user for http/socks
	Password string
	Query    url.Values
	Fragment string
	Address  string // host:port
}

func (l *Link) param(key string) string {
	return strings.TrimSpace(l.Query.Get(key))
}

// boolParam reads a query flag, defaulting to def when absent or empty.
func (l *Link) boolParam(key string, def bool) bool {
	value := strings.ToLower(l.param(key))
	switch value {
	case "", "auto":
		return def
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func defaultPort(scheme string) int {
	switch strings.ToLower(scheme) {
	case "http":
		return 80
	case "https", "trojan", "vless":
		return 443
	case "socks", "socks5":
		return 1080
	}
	return 0
}

// parseLink parses a standard proxy URI (vless/trojan/http/https/socks).
func parseLink(raw string) (*Link, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse link: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("parse link: missing host in %q", raw)
	}

	link := &Link{
		Scheme:   strings.ToLower(u.Scheme),
		Host:     u.Hostname(),
		Query:    u.Query(),
		Fragment: u.Fragment,
	}
	if u.User != nil {
		link.Username = u.User.Username()
		link.Password, _ = u.User.Password()
	}

	if port := u.Port(); port != "" {
		link.Port, err = strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("parse link: invalid port %q: %w", port, err)
		}
	} else {
		link.Port = defaultPort(link.Scheme)
	}
	if link.Port == 0 {
		return nil, fmt.Errorf("parse link: missing port in %q", raw)
	}
	if link.Host == "" {
		return nil, fmt.Errorf("parse link: empty host in %q", raw)
	}
	link.Address = joinHostPort(link.Host, link.Port)
	return link, nil
}

// vmessLink is the JSON payload carried by `vmess://` links.
type vmessLink struct {
	Version  string `json:"v"`
	Name     string `json:"ps"`
	Address  string `json:"add"`
	Port     any    `json:"port"`
	ID       string `json:"id"`
	AlterID  any    `json:"aid"`
	Network  string `json:"net"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Path     string `json:"path"`
	TLS      string `json:"tls"`
	SNI      string `json:"sni"`
	Security string `json:"scy"`
	// AllowInsecure is produced by some generators as a boolean and by others
	// as a string, so it is decoded from the raw map instead.
	AllowInsecure bool
}

func (v *vmessLink) intPort() (int, error) {
	switch p := v.Port.(type) {
	case float64:
		return int(p), nil
	case string:
		port, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return 0, fmt.Errorf("invalid vmess port %q", p)
		}
		return port, nil
	case nil:
		return 0, fmt.Errorf("vmess link has no port")
	default:
		return 0, fmt.Errorf("unsupported vmess port type %T", p)
	}
}

func (v *vmessLink) alterID() int {
	switch a := v.AlterID.(type) {
	case float64:
		return int(a)
	case string:
		id, _ := strconv.Atoi(strings.TrimSpace(a))
		return id
	}
	return 0
}

// parseVMessLink decodes the base64 JSON carried by a `vmess://` link.
func parseVMessLink(raw string) (*vmessLink, error) {
	payload := raw
	if idx := strings.Index(raw, "://"); idx >= 0 {
		payload = raw[idx+3:]
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, fmt.Errorf("empty vmess link")
	}

	decoded, err := decodeBase64Flexible(payload)
	if err != nil {
		return nil, fmt.Errorf("decode vmess link: %w", err)
	}

	link := &vmessLink{}
	if err := json.Unmarshal(decoded, link); err != nil {
		return nil, fmt.Errorf("parse vmess payload: %w", err)
	}
	// Some exporters emit booleans/strings interchangeably.
	var rawFields map[string]any
	if err := json.Unmarshal(decoded, &rawFields); err == nil {
		link.AllowInsecure = truthy(rawFields["allowInsecure"], truthy(rawFields["skip-cert-verify"], false))
	}

	link.Address = strings.TrimSpace(link.Address)
	if link.Address == "" {
		return nil, fmt.Errorf("vmess link has no address")
	}
	if link.ID == "" {
		return nil, fmt.Errorf("vmess link has no id")
	}
	if _, err := link.intPort(); err != nil {
		return nil, err
	}
	return link, nil
}

func truthy(value any, def bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			return true
		case "0", "false", "no":
			return false
		}
	case float64:
		return v != 0
	}
	return def
}

// decodeBase64Flexible accepts standard, URL and unpadded base64.
func decodeBase64Flexible(payload string) ([]byte, error) {
	payload = strings.TrimSpace(payload)
	if idx := strings.Index(payload, "://"); idx >= 0 {
		payload = payload[idx+3:]
	}
	payload = strings.TrimRight(payload, "=")

	decoders := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}
	// Raw decoders reject padded input, so try them with padding restored too.
	candidates := []string{payload, payload + "=", payload + "==", payload + "==="}
	for _, enc := range decoders {
		for _, candidate := range candidates {
			if len(candidate)%4 == 0 || enc == base64.RawStdEncoding || enc == base64.RawURLEncoding {
				if decoded, err := enc.DecodeString(candidate); err == nil {
					return decoded, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("not valid base64")
}

// normalizeUUID parses a UUID text into its 16 raw bytes.
func normalizeUUID(value string) ([16]byte, error) {
	var out [16]byte
	cleaned := strings.TrimSpace(value)
	cleaned = strings.Trim(cleaned, "{}")
	if cleaned == "" {
		return out, fmt.Errorf("empty uuid")
	}
	hex := ""
	for _, part := range strings.Split(cleaned, "-") {
		if len(part)%2 != 0 {
			part = "0" + part
		}
		hex += part
	}
	raw, err := parseHex(hex)
	if err != nil {
		return out, fmt.Errorf("invalid uuid %q: %w", value, err)
	}
	if len(raw) != 16 {
		return out, fmt.Errorf("invalid uuid %q: expected 16 bytes, got %d", value, len(raw))
	}
	copy(out[:], raw)
	return out, nil
}

// firstNonEmpty returns the first non empty value.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// hostOnly strips a port from a host header value such as "example.com:443".
func hostOnly(value string) string {
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return value
}

// parseHex decodes a hex string.
func parseHex(value string) ([]byte, error) {
	return hex.DecodeString(value)
}
