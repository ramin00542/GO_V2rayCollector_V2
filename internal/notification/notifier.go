// Package notification provides notification utilities
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Notifier interface for sending notifications
type Notifier interface {
	Send(ctx context.Context, message string, details map[string]interface{}) error
	Close() error
}

// NotificationConfig holds configuration for a notifier
type NotificationConfig struct {
	Type    string            `json:"type"`
	Enabled bool              `json:"enabled"`
	Options map[string]string `json:"options"`
}

// NotifierFactory creates notifiers based on configuration
type NotifierFactory struct{}

// Create creates a notifier based on configuration
func (f *NotifierFactory) Create(config NotificationConfig) (Notifier, error) {
	if !config.Enabled {
		return &NopNotifier{}, nil
	}

	switch strings.ToLower(config.Type) {
	case "telegram":
		return NewTelegramNotifier(config.Options)
	case "webhook":
		return NewWebhookNotifier(config.Options)
	case "file":
		return NewFileNotifier(config.Options)
	case "stdout":
		return &StdoutNotifier{}, nil
	default:
		return nil, fmt.Errorf("unknown notifier type: %s", config.Type)
	}
}

// NopNotifier is a no-op notifier
type NopNotifier struct{}

func (n *NopNotifier) Send(ctx context.Context, message string, details map[string]interface{}) error {
	return nil
}

func (n *NopNotifier) Close() error {
	return nil
}

// StdoutNotifier sends notifications to stdout
type StdoutNotifier struct{}

func (n *StdoutNotifier) Send(ctx context.Context, message string, details map[string]interface{}) error {
	fmt.Printf("[NOTIFICATION] %s\n", message)
	if len(details) > 0 {
		data, _ := json.MarshalIndent(details, "  ", "  ")
		fmt.Printf("[DETAILS] %s\n", string(data))
	}
	return nil
}

func (n *StdoutNotifier) Close() error {
	return nil
}

// FileNotifier sends notifications to a file
type FileNotifier struct {
	file *os.File
}

func NewFileNotifier(options map[string]string) (*FileNotifier, error) {
	path := options["path"]
	if path == "" {
		path = "notifications.log"
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &FileNotifier{file: file}, nil
}

func (n *FileNotifier) Send(ctx context.Context, message string, details map[string]interface{}) error {
	if n.file == nil {
		return fmt.Errorf("file notifier not initialized")
	}

	line := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), message)
	if _, err := n.file.WriteString(line); err != nil {
		return err
	}

	if len(details) > 0 {
		data, _ := json.MarshalIndent(details, "  ", "  ")
		if _, err := n.file.WriteString(fmt.Sprintf("  Details: %s\n", string(data))); err != nil {
			return err
		}
	}

	return n.file.Sync()
}

func (n *FileNotifier) Close() error {
	if n.file != nil {
		return n.file.Close()
	}
	return nil
}

// WebhookNotifier sends notifications to a webhook URL
type WebhookNotifier struct {
	url     string
	client  *http.Client
	headers map[string]string
}

func NewWebhookNotifier(options map[string]string) (*WebhookNotifier, error) {
	webhookURL := options["url"]
	if webhookURL == "" {
		return nil, fmt.Errorf("webhook URL is required")
	}

	// Parse URL to validate
	_, err := url.Parse(webhookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Build headers
	headers := make(map[string]string)
	if contentType, ok := options["content_type"]; ok {
		headers["Content-Type"] = contentType
	} else {
		headers["Content-Type"] = "application/json"
	}

	// Add custom headers
	for k, v := range options {
		if strings.HasPrefix(k, "header_") {
			headerName := strings.TrimPrefix(k, "header_")
			headers[headerName] = v
		}
	}

	return &WebhookNotifier{
		url:     webhookURL,
		client:  &http.Client{Timeout: 30 * time.Second},
		headers: headers,
	}, nil
}

func (n *WebhookNotifier) Send(ctx context.Context, message string, details map[string]interface{}) error {
	// Build payload
	payload := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   message,
		"details":   details,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	// Set headers
	for k, v := range n.headers {
		req.Header.Set(k, v)
	}

	// Send request
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (n *WebhookNotifier) Close() error {
	return nil
}

// TelegramNotifier sends notifications via Telegram Bot API
type TelegramNotifier struct {
	botToken  string
	chatID    string
	client    *http.Client
	parseMode string
}

func NewTelegramNotifier(options map[string]string) (*TelegramNotifier, error) {
	botToken := options["bot_token"]
	chatID := options["chat_id"]

	if botToken == "" {
		return nil, fmt.Errorf("Telegram bot token is required")
	}
	if chatID == "" {
		return nil, fmt.Errorf("Telegram chat ID is required")
	}

	parseMode := options["parse_mode"]
	if parseMode == "" {
		parseMode = "HTML"
	}

	return &TelegramNotifier{
		botToken:  botToken,
		chatID:    chatID,
		client:    &http.Client{Timeout: 30 * time.Second},
		parseMode: parseMode,
	}, nil
}

func (n *TelegramNotifier) Send(ctx context.Context, message string, details map[string]interface{}) error {
	// Build Telegram message
	text := message

	// Add details if present
	if len(details) > 0 {
		text += "\n\n<b>Details:</b>\n"
		for k, v := range details {
			text += fmt.Sprintf("• <b>%s:</b> <code>%v</code>\n", k, v)
		}
	}

	// Escape special characters for HTML
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")

	// Build payload
	payload := map[string]interface{}{
		"chat_id":    n.chatID,
		"text":       text,
		"parse_mode": n.parseMode,
	}

	// Add disable_web_page_preview to avoid link previews
	payload["disable_web_page_preview"] = true

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Build URL
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Telegram API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (n *TelegramNotifier) Close() error {
	return nil
}

// MultiNotifier sends notifications to multiple notifiers
type MultiNotifier struct {
	notifiers []Notifier
}

func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

func (n *MultiNotifier) Send(ctx context.Context, message string, details map[string]interface{}) error {
	var errs []string

	for _, notifier := range n.notifiers {
		if err := notifier.Send(ctx, message, details); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to send notifications: %s", strings.Join(errs, "; "))
	}

	return nil
}

func (n *MultiNotifier) Close() error {
	var errs []string

	for _, notifier := range n.notifiers {
		if err := notifier.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close notifiers: %s", strings.Join(errs, "; "))
	}

	return nil
}

// LoadNotifierConfigFromFile loads notifier configuration from a file
func LoadNotifierConfigFromFile(path string) ([]NotificationConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist, return empty config
		}
		return nil, err
	}

	var configs []NotificationConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}

	return configs, nil
}

// CreateNotifiersFromConfig creates notifiers from configuration file
func CreateNotifiersFromConfig(path string) ([]Notifier, error) {
	configs, err := LoadNotifierConfigFromFile(path)
	if err != nil {
		return nil, err
	}

	var notifiers []Notifier
	factory := &NotifierFactory{}

	for _, config := range configs {
		notifier, err := factory.Create(config)
		if err != nil {
			return nil, err
		}
		notifiers = append(notifiers, notifier)
	}

	if len(notifiers) == 0 {
		// Return a no-op notifier if no notifiers configured
		return []Notifier{&NopNotifier{}}, nil
	}

	// If we have multiple notifiers, wrap them in a MultiNotifier
	if len(notifiers) > 1 {
		return []Notifier{NewMultiNotifier(notifiers...)}, nil
	}

	return notifiers, nil
}

// SendNotification is a convenience function to send a notification using the default notifier
func SendNotification(ctx context.Context, message string, details map[string]interface{}) error {
	// For now, just use stdout notifier
	// In a real implementation, you would load configuration and create proper notifiers
	notifier := &StdoutNotifier{}
	return notifier.Send(ctx, message, details)
}

// NotificationMessageBuilder helps build notification messages
type NotificationMessageBuilder struct {
	parts []string
}

func NewNotificationMessageBuilder() *NotificationMessageBuilder {
	return &NotificationMessageBuilder{parts: []string{}}
}

func (b *NotificationMessageBuilder) Add(line string) *NotificationMessageBuilder {
	b.parts = append(b.parts, line)
	return b
}

func (b *NotificationMessageBuilder) Addf(format string, args ...interface{}) *NotificationMessageBuilder {
	b.parts = append(b.parts, fmt.Sprintf(format, args...))
	return b
}

func (b *NotificationMessageBuilder) AddHeader(header string) *NotificationMessageBuilder {
	b.parts = append(b.parts, fmt.Sprintf("=== %s ===", header))
	return b
}

func (b *NotificationMessageBuilder) AddSection(section string) *NotificationMessageBuilder {
	b.parts = append(b.parts, fmt.Sprintf("\n--- %s ---", section))
	return b
}

func (b *NotificationMessageBuilder) AddList(items []string) *NotificationMessageBuilder {
	if len(items) == 0 {
		return b
	}

	b.parts = append(b.parts, "")
	for _, item := range items {
		b.parts = append(b.parts, fmt.Sprintf("• %s", item))
	}
	return b
}

func (b *NotificationMessageBuilder) AddKeyValue(key, value string) *NotificationMessageBuilder {
	b.parts = append(b.parts, fmt.Sprintf("%s: %s", key, value))
	return b
}

func (b *NotificationMessageBuilder) Build() string {
	return strings.Join(b.parts, "\n")
}

func (b *NotificationMessageBuilder) BuildWithDetails() (string, map[string]interface{}) {
	message := b.Build()

	// For now, return empty details
	// In a more complete implementation, you could extract structured data
	return message, map[string]interface{}{}
}
