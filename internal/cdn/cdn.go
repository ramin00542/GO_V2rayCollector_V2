// Package cdn provides CDN upload and management functionality
package cdn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ramin00542/GO_V2rayCollector_V2/internal/config"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/fetch"
	"github.com/ramin00542/GO_V2rayCollector_V2/internal/logging"
)

// CDNProvider interface for different CDN providers
type CDNProvider interface {
	Upload(ctx context.Context, filePath string, content []byte) (string, error)
	Delete(ctx context.Context, fileID string) error
	List(ctx context.Context) ([]CDNFile, error)
	GetURL(fileID string) string
	GetName() string
}

// CDNFile represents a file on the CDN
type CDNFile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	UploadedAt time.Time `json:"uploaded_at"`
	Public    bool      `json:"public"`
}

// CDNManager manages uploads to various CDN providers
type CDNManager struct {
	providers map[string]CDNProvider
	config    CDNConfig
	logger    *logging.Logger
	client    *fetch.Client
	cache     map[string]CDNFile
}

// CDNConfig holds configuration for CDN uploads
type CDNConfig struct {
	Provider     string `json:"provider"`
	APIKey       string `json:"api_key"`
	APISecret    string `json:"api_secret"`
	BucketName   string `json:"bucket_name"`
	BaseURL      string `json:"base_url"`
	PublicFiles  bool   `json:"public_files"`
	ExpireAfter  string `json:"expire_after"` // Duration string
}

// NewCDNManager creates a new CDN manager
func NewCDNManager(cfg CDNConfig, logger *logging.Logger) (*CDNManager, error) {
	client, err := fetch.NewClient(fetch.DefaultConfig())
	if err != nil {
		return nil, err
	}
	
	m := &CDNManager{
		providers: make(map[string]CDNProvider),
		config:    cfg,
		logger:    logger,
		client:    client,
		cache:     make(map[string]CDNFile),
	}
	
	// Initialize provider based on configuration
	if err := m.initProvider(); err != nil {
		return nil, err
	}
	
	return m, nil
}

// initProvider initializes the CDN provider based on configuration
func (m *CDNManager) initProvider() error {
	switch strings.ToLower(m.config.Provider) {
	case "cloudflare":
		provider, err := NewCloudflareProvider(m.config, m.client, m.logger)
		if err != nil {
			return err
		}
		m.providers["cloudflare"] = provider
		
	case "aws", "s3":
		provider, err := NewS3Provider(m.config, m.client, m.logger)
		if err != nil {
			return err
		}
		m.providers["aws"] = provider
		
	case "github":
		provider, err := NewGitHubProvider(m.config, m.client, m.logger)
		if err != nil {
			return err
		}
		m.providers["github"] = provider
		
	case "local":
		provider := NewLocalProvider(m.config, m.logger)
		m.providers["local"] = provider
		
	default:
		return fmt.Errorf("unsupported CDN provider: %s", m.config.Provider)
	}
	
	return nil
}

// GetProvider returns the configured CDN provider
func (m *CDNManager) GetProvider() (CDNProvider, error) {
	if len(m.providers) == 0 {
		return nil, fmt.Errorf("no CDN provider configured")
	}
	
	// Return the first (and only) provider
	for _, provider := range m.providers {
		return provider, nil
	}
	
	return nil, fmt.Errorf("no CDN provider available")
}

// UploadFile uploads a file to the CDN
func (m *CDNManager) UploadFile(ctx context.Context, filePath string) (CDNFile, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return CDNFile{}, err
	}
	
	// Get provider
	provider, err := m.GetProvider()
	if err != nil {
		return CDNFile{}, err
	}
	
	// Upload to CDN
	fileID, err := provider.Upload(ctx, filePath, content)
	if err != nil {
		return CDNFile{}, err
	}
	
	// Create CDN file info
	file := CDNFile{
		ID:        fileID,
		Name:      filepath.Base(filePath),
		URL:       provider.GetURL(fileID),
		Size:      int64(len(content)),
		UploadedAt: time.Now().UTC(),
		Public:    m.config.PublicFiles,
	}
	
	// Cache the file
	m.cache[filePath] = file
	
	return file, nil
}

// UploadConfig uploads a config file to the CDN
func (m *CDNManager) UploadConfig(ctx context.Context, configValue string) (CDNFile, error) {
	// Generate a unique filename
	filename := generateConfigFilename(configValue)
	
	// Create temporary file
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, filename)
	defer os.Remove(tmpPath)
	
	if err := os.WriteFile(tmpPath, []byte(configValue), 0644); err != nil {
		return CDNFile{}, err
	}
	
	return m.UploadFile(ctx, tmpPath)
}

// UploadSubscription uploads all configs from a subscription to the CDN
func (m *CDNManager) UploadSubscription(ctx context.Context, subURL string, configs []string) ([]CDNFile, error) {
	var files []CDNFile
	
	for i, config := range configs {
		// Create a unique filename for each config
		filename := fmt.Sprintf("sub_%s_config_%d.txt", sanitizeFilename(subURL), i+1)
		tmpDir := os.TempDir()
		tmpPath := filepath.Join(tmpDir, filename)
		defer os.Remove(tmpPath)
		
		if err := os.WriteFile(tmpPath, []byte(config), 0644); err != nil {
			return nil, err
		}
		
		file, err := m.UploadFile(ctx, tmpPath)
		if err != nil {
			return files, err
		}
		
		files = append(files, file)
	}
	
	return files, nil
}

// UploadAllConfigs uploads all configs to the CDN
func (m *CDNManager) UploadAllConfigs(ctx context.Context, configs []string) ([]CDNFile, error) {
	var files []CDNFile
	
	for i, config := range configs {
		file, err := m.UploadConfig(ctx, config)
		if err != nil {
			return files, err
		}
		files = append(files, file)
		
		// Log progress
		m.logger.Info("uploaded config to CDN", "index", i+1, "total", len(configs))
	}
	
	return files, nil
}

// GenerateSubscriptionLink generates a subscription link for all uploaded configs
func (m *CDNManager) GenerateSubscriptionLink(ctx context.Context, configs []string) (string, error) {
	// Upload all configs
	files, err := m.UploadAllConfigs(ctx, configs)
	if err != nil {
		return "", err
	}
	
	// Generate subscription content
	var sb strings.Builder
	for _, file := range files {
		sb.WriteString(file.URL)
		sb.WriteString("\n")
	}
	
	// Upload subscription file
	subscriptionContent := sb.String()
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, "subscription.txt")
	defer os.Remove(tmpPath)
	
	if err := os.WriteFile(tmpPath, []byte(subscriptionContent), 0644); err != nil {
		return "", err
	}
	
	// Upload to CDN
	file, err := m.UploadFile(ctx, tmpPath)
	if err != nil {
		return "", err
	}
	
	return file.URL, nil
}

// DeleteFile deletes a file from the CDN
func (m *CDNManager) DeleteFile(ctx context.Context, fileID string) error {
	provider, err := m.GetProvider()
	if err != nil {
		return err
	}
	
	return provider.Delete(ctx, fileID)
}

// ListFiles lists all files on the CDN
func (m *CDNManager) ListFiles(ctx context.Context) ([]CDNFile, error) {
	provider, err := m.GetProvider()
	if err != nil {
		return nil, err
	}
	
	return provider.List(ctx)
}

// GetFileURL returns the URL of a file on the CDN
func (m *CDNManager) GetFileURL(fileID string) string {
	provider, err := m.GetProvider()
	if err != nil {
		return ""
	}
	
	return provider.GetURL(fileID)
}

// generateConfigFilename generates a unique filename for a config
func generateConfigFilename(config string) string {
	// Create hash of config
	hash := sha256.Sum256([]byte(config))
	hashStr := hex.EncodeToString(hash[:])[:16]
	
	// Get protocol from config
	protocol := extractProtocol(config)
	
	return fmt.Sprintf("%s_%s.txt", protocol, hashStr)
}

// extractProtocol extracts the protocol from a config string
func extractProtocol(config string) string {
	config = strings.ToLower(strings.TrimSpace(config))
	
	if strings.HasPrefix(config, "vmess://") {
		return "vmess"
	}
	if strings.HasPrefix(config, "vless://") {
		return "vless"
	}
	if strings.HasPrefix(config, "trojan://") {
		return "trojan"
	}
	if strings.HasPrefix(config, "ssr://") {
		return "ssr"
	}
	if strings.HasPrefix(config, "ss://") {
		return "ss"
	}
	
	return "config"
}

// sanitizeFilename creates a safe filename from a URL
func sanitizeFilename(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	
	// Remove special characters
	url = strings.ReplaceAll(url, "/", "_")
	url = strings.ReplaceAll(url, "?", "_")
	url = strings.ReplaceAll(url, "=", "_")
	url = strings.ReplaceAll(url, "&", "_")
	url = strings.ReplaceAll(url, "#", "_")
	
	// Trim to reasonable length
	if len(url) > 50 {
		url = url[:50]
	}
	
	return url
}

// LoadCDNConfig loads CDN configuration from file
func LoadCDNConfig(path string) (CDNConfig, error) {
	var cfg CDNConfig
	
	// Use default config if file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return CDNConfig{
			Provider:    "local",
			PublicFiles: true,
		}, nil
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return CDNConfig{}, err
	}
	
	if err := json.Unmarshal(data, &cfg); err != nil {
		return CDNConfig{}, err
	}
	
	return cfg, nil
}

// SaveCDNConfig saves CDN configuration to file
func SaveCDNConfig(cfg CDNConfig, path string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tmpPath, path)
}

// ===== CDN Provider Implementations =====

// LocalProvider is a local file system "CDN" for testing
type LocalProvider struct {
	config CDNConfig
	logger *logging.Logger
	baseDir string
}

// NewLocalProvider creates a new local provider
func NewLocalProvider(cfg CDNConfig, logger *logging.Logger) *LocalProvider {
	baseDir := filepath.Join("data", "cdn")
	if cfg.BucketName != "" {
		baseDir = cfg.BucketName
	}
	
	return &LocalProvider{
		config:  cfg,
		logger: logger,
		baseDir: baseDir,
	}
}

// Upload uploads a file to the local file system
func (p *LocalProvider) Upload(ctx context.Context, filePath string, content []byte) (string, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(p.baseDir, 0755); err != nil {
		return "", err
	}
	
	// Generate file ID
	fileID := generateFileID(filePath, content)
	
	// Save file
	path := filepath.Join(p.baseDir, fileID)
	tmpPath := path + ".tmp"
	
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return "", err
	}
	
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}
	
	return fileID, nil
}

// Delete deletes a file from the local file system
func (p *LocalProvider) Delete(ctx context.Context, fileID string) error {
	path := filepath.Join(p.baseDir, fileID)
	return os.Remove(path)
}

// List lists all files in the local directory
func (p *LocalProvider) List(ctx context.Context) ([]CDNFile, error) {
	var files []CDNFile
	
	// List all files in the directory
	entries, err := os.ReadDir(p.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return files, nil
		}
		return nil, err
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		files = append(files, CDNFile{
			ID:        entry.Name(),
			Name:      entry.Name(),
			URL:       p.GetURL(entry.Name()),
			Size:      info.Size(),
			UploadedAt: info.ModTime(),
			Public:    true,
		})
	}
	
	return files, nil
}

// GetURL returns the URL for a file
func (p *LocalProvider) GetURL(fileID string) string {
	if p.config.BaseURL != "" {
		return fmt.Sprintf("%s/%s", p.config.BaseURL, fileID)
	}
	return fmt.Sprintf("/cdn/%s", fileID)
}

// GetName returns the provider name
func (p *LocalProvider) GetName() string {
	return "local"
}

// generateFileID generates a unique file ID
func generateFileID(filePath string, content []byte) string {
	// Create hash of content
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])
	
	// Add extension based on original file
	ext := filepath.Ext(filePath)
	if ext == "" {
		ext = ".txt"
	}
	
	return hashStr + ext
}

// CloudflareProvider implements CDNProvider for Cloudflare R2
// Note: This is a placeholder implementation. In a real implementation,
// you would use the Cloudflare R2 API.
type CloudflareProvider struct {
	config CDNConfig
	client *fetch.Client
	logger *logging.Logger
}

// NewCloudflareProvider creates a new Cloudflare provider
func NewCloudflareProvider(cfg CDNConfig, client *fetch.Client, logger *logging.Logger) (*CloudflareProvider, error) {
	// Validate configuration
	if cfg.APIKey == "" || cfg.BucketName == "" {
		return nil, fmt.Errorf("Cloudflare requires API key and bucket name")
	}
	
	return &CloudflareProvider{
		config: cfg,
		client: client,
		logger: logger,
	}, nil
}

// Upload uploads a file to Cloudflare R2
func (p *CloudflareProvider) Upload(ctx context.Context, filePath string, content []byte) (string, error) {
	// In a real implementation, this would use the Cloudflare R2 API
	// For now, we'll simulate the upload
	
	fileID := generateFileID(filePath, content)
	
	// Log the upload
	p.logger.Info("uploading to Cloudflare R2", "file", filePath, "size", len(content))
	
	// In a real implementation:
	// 1. Create a multipart form
	// 2. Add the file content
	// 3. Send to Cloudflare R2 API
	// 4. Return the file ID or URL
	
	return fileID, nil
}

// Delete deletes a file from Cloudflare R2
func (p *CloudflareProvider) Delete(ctx context.Context, fileID string) error {
	// In a real implementation, this would call the Cloudflare R2 API
	p.logger.Info("deleting from Cloudflare R2", "file_id", fileID)
	return nil
}

// List lists all files in Cloudflare R2
func (p *CloudflareProvider) List(ctx context.Context) ([]CDNFile, error) {
	// In a real implementation, this would call the Cloudflare R2 API
	p.logger.Info("listing files from Cloudflare R2")
	return []CDNFile{}, nil
}

// GetURL returns the URL for a file
func (p *CloudflareProvider) GetURL(fileID string) string {
	if p.config.BaseURL != "" {
		return fmt.Sprintf("%s/%s", p.config.BaseURL, fileID)
	}
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com/%s", p.config.BucketName, fileID)
}

// GetName returns the provider name
func (p *CloudflareProvider) GetName() string {
	return "cloudflare"
}

// S3Provider implements CDNProvider for AWS S3
// Note: This is a placeholder implementation. In a real implementation,
// you would use the AWS SDK.
type S3Provider struct {
	config CDNConfig
	client *fetch.Client
	logger *logging.Logger
}

// NewS3Provider creates a new S3 provider
func NewS3Provider(cfg CDNConfig, client *fetch.Client, logger *logging.Logger) (*S3Provider, error) {
	// Validate configuration
	if cfg.APIKey == "" || cfg.APISecret == "" || cfg.BucketName == "" {
		return nil, fmt.Errorf("S3 requires API key, API secret, and bucket name")
	}
	
	return &S3Provider{
		config: cfg,
		client: client,
		logger: logger,
	}, nil
}

// Upload uploads a file to S3
func (p *S3Provider) Upload(ctx context.Context, filePath string, content []byte) (string, error) {
	// In a real implementation, this would use the AWS SDK
	fileID := generateFileID(filePath, content)
	
	p.logger.Info("uploading to S3", "file", filePath, "size", len(content))
	
	// In a real implementation:
	// 1. Create a new S3 client
	// 2. Upload the file to the specified bucket
	// 3. Return the object key
	
	return fileID, nil
}

// Delete deletes a file from S3
func (p *S3Provider) Delete(ctx context.Context, fileID string) error {
	p.logger.Info("deleting from S3", "file_id", fileID)
	return nil
}

// List lists all files in S3
func (p *S3Provider) List(ctx context.Context) ([]CDNFile, error) {
	p.logger.Info("listing files from S3")
	return []CDNFile{}, nil
}

// GetURL returns the URL for a file
func (p *S3Provider) GetURL(fileID string) string {
	if p.config.BaseURL != "" {
		return fmt.Sprintf("%s/%s", p.config.BaseURL, fileID)
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", p.config.BucketName, fileID)
}

// GetName returns the provider name
func (p *S3Provider) GetName() string {
	return "aws-s3"
}

// GitHubProvider implements CDNProvider for GitHub
// This uses GitHub as a simple CDN by uploading to a repository
type GitHubProvider struct {
	config CDNConfig
	client *fetch.Client
	logger *logging.Logger
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider(cfg CDNConfig, client *fetch.Client, logger *logging.Logger) (*GitHubProvider, error) {
	// Validate configuration
	if cfg.APIKey == "" || cfg.BucketName == "" {
		return nil, fmt.Errorf("GitHub requires API token and repository")
	}
	
	return &GitHubProvider{
		config: cfg,
		client: client,
		logger: logger,
	}, nil
}

// Upload uploads a file to GitHub
func (p *GitHubProvider) Upload(ctx context.Context, filePath string, content []byte) (string, error) {
	// In a real implementation, this would use the GitHub API
	fileID := generateFileID(filePath, content)
	
	p.logger.Info("uploading to GitHub", "file", filePath, "size", len(content))
	
	// In a real implementation:
	// 1. Create a new GitHub client
	// 2. Get the repository info from config.BucketName (format: owner/repo)
	// 3. Upload the file to the repository (e.g., to a specific branch/path)
	// 4. Return the file path
	
	return fileID, nil
}

// Delete deletes a file from GitHub
func (p *GitHubProvider) Delete(ctx context.Context, fileID string) error {
	p.logger.Info("deleting from GitHub", "file_id", fileID)
	return nil
}

// List lists all files in GitHub
func (p *GitHubProvider) List(ctx context.Context) ([]CDNFile, error) {
	p.logger.Info("listing files from GitHub")
	return []CDNFile{}, nil
}

// GetURL returns the URL for a file
func (p *GitHubProvider) GetURL(fileID string) string {
	if p.config.BaseURL != "" {
		return fmt.Sprintf("%s/%s", p.config.BaseURL, fileID)
	}
	// Format: https://raw.githubusercontent.com/owner/repo/branch/path/file
	parts := strings.Split(p.config.BucketName, "/")
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/cdn/%s", parts[0], parts[1], fileID)
}

// GetName returns the provider name
func (p *GitHubProvider) GetName() string {
	return "github"
}

// UploadToGitHubWithAPI uploads a file to GitHub using the API
func (p *GitHubProvider) UploadToGitHubWithAPI(ctx context.Context, repo, branch, path string, content []byte) (string, error) {
	// This is a more complete implementation using GitHub API
	
	// Get GitHub token from config
	token := p.config.APIKey
	if token == "" {
		return "", fmt.Errorf("GitHub token is required")
	}
	
	// Create URL for GitHub API
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repo, path)
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return "", err
	}
	
	// Set headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	
	// Get existing file SHA if it exists
	var sha string
	resp, err := p.client.Get(ctx, url, nil)
	if err == nil && resp.StatusCode == http.StatusOK {
		var fileInfo struct {
			SHA string `json:"sha"`
		}
		if err := json.Unmarshal(resp.Body, &fileInfo); err == nil {
			sha = fileInfo.SHA
		}
	}
	
	// Prepare the request body
	body := map[string]interface{}{
		"message": "Update config file",
		"content": string(content),
		"branch":  branch,
	}
	
	// If file exists, include SHA for update
	if sha != "" {
		body["sha"] = sha
	}
	
	// Marshal body
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	
	// Create new request with body
	req, err = http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}
	
	// Set headers again
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	
	// Send request
	resp, err = p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}
	
	// Parse response
	var response struct {
		Content struct {
			Path string `json:"path"`
		} `json:"content"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	
	return response.Content.Path, nil
}

// UploadLargeFileToGitHub uploads a large file to GitHub using multipart upload
func (p *GitHubProvider) UploadLargeFileToGitHub(ctx context.Context, repo, branch, path string, content []byte) (string, error) {
	// For large files, we need to use a different approach
	// This implementation uses the GitHub API for large files
	
	token := p.config.APIKey
	if token == "" {
		return "", fmt.Errorf("GitHub token is required")
	}
	
	// Step 1: Create an upload URL
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/blobs", repo)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(content)))
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/octet-stream")
	
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}
	
	// Parse response to get SHA
	var blob struct {
		SHA string `json:"sha"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&blob); err != nil {
		return "", err
	}
	
	// Step 2: Create a tree
	// This is simplified - in a real implementation, you would need to:
	// 1. Get the current tree
	// 2. Add the new blob to the tree
	// 3. Create a new tree
	// 4. Create a new commit
	// 5. Update the reference
	
	// For simplicity, we'll just return the blob SHA
	// In a real implementation, you would complete the process
	return blob.SHA, nil
}

// UploadFileWithMultipart uploads a file using multipart form data
func (p *GitHubProvider) UploadFileWithMultipart(ctx context.Context, repo, branch, path string, content []byte) (string, error) {
	// Create a buffer for the multipart form
	var bodyBuf bytes.Buffer
	writer := multipart.NewWriter(&bodyBuf)
	
	// Add the file
	fileWriter, err := writer.CreateFormFile("file", path)
	if err != nil {
		return "", err
	}
	
	if _, err := fileWriter.Write(content); err != nil {
		return "", err
	}
	
	// Add other fields
	if err := writer.WriteField("message", "Update config file"); err != nil {
		return "", err
	}
	if err := writer.WriteField("branch", branch); err != nil {
		return "", err
	}
	
	// Close the multipart writer
	if err := writer.Close(); err != nil {
		return "", err
	}
	
	// Create request
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", repo, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, &bodyBuf)
	if err != nil {
		return "", err
	}
	
	// Set headers
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	
	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error: %s - %s", resp.Status, string(body))
	}
	
	// Parse response
	var response struct {
		Content struct {
			Path string `json:"path"`
		} `json:"content"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	
	return response.Content.Path, nil
}
