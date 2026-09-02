package tester

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultTargetSites returns the default list of target sites
func DefaultTargetSites() []TargetSite {
	return []TargetSite{
		// Search Engines
		{Name: "Google", URL: "https://www.google.com", Category: "search", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Google Search", URL: "https://www.google.com/search?q=test", Category: "search", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Bing", URL: "https://www.bing.com", Category: "search", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "DuckDuckGo", URL: "https://duckduckgo.com", Category: "search", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Yahoo", URL: "https://search.yahoo.com", Category: "search", ExpectedStatus: 200, TimeoutSeconds: 10},

		// Video & Streaming
		{Name: "YouTube", URL: "https://www.youtube.com", Category: "video", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Netflix", URL: "https://www.netflix.com", Category: "streaming", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Twitch", URL: "https://www.twitch.tv", Category: "streaming", ExpectedStatus: 200, TimeoutSeconds: 10},

		// Code & Development
		{Name: "GitHub", URL: "https://github.com", Category: "code", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "GitLab", URL: "https://gitlab.com", Category: "code", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Stack Overflow", URL: "https://stackoverflow.com", Category: "code", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Bitbucket", URL: "https://bitbucket.org", Category: "code", ExpectedStatus: 200, TimeoutSeconds: 10},

		// Social Media
		{Name: "Twitter/X", URL: "https://x.com", Category: "social", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Instagram", URL: "https://www.instagram.com", Category: "social", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Facebook", URL: "https://www.facebook.com", Category: "social", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Reddit", URL: "https://www.reddit.com", Category: "social", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "LinkedIn", URL: "https://www.linkedin.com", Category: "social", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "TikTok", URL: "https://www.tiktok.com", Category: "social", ExpectedStatus: 200, TimeoutSeconds: 10},

		// News & Information
		{Name: "BBC News", URL: "https://www.bbc.com", Category: "news", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "CNN", URL: "https://edition.cnn.com", Category: "news", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Reuters", URL: "https://www.reuters.com", Category: "news", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Wikipedia", URL: "https://en.wikipedia.org", Category: "encyclopedia", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Quora", URL: "https://www.quora.com", Category: "qa", ExpectedStatus: 200, TimeoutSeconds: 10},

		// AI & Machine Learning
		{Name: "Gemini AI", URL: "https://gemini.google.com", Category: "ai", ExpectedStatus: 200, TimeoutSeconds: 15},
		{Name: "Hugging Face", URL: "https://huggingface.co", Category: "ai", ExpectedStatus: 200, TimeoutSeconds: 15},
		{Name: "Kaggle", URL: "https://www.kaggle.com", Category: "ai", ExpectedStatus: 200, TimeoutSeconds: 15},
		{Name: "Stable Diffusion", URL: "https://stability.ai", Category: "ai", ExpectedStatus: 200, TimeoutSeconds: 15},

		// Cloud & Networking
		{Name: "Cloudflare", URL: "https://www.cloudflare.com", Category: "network", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "AWS", URL: "https://aws.amazon.com", Category: "cloud", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Google Cloud", URL: "https://cloud.google.com", Category: "cloud", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Microsoft Azure", URL: "https://azure.microsoft.com", Category: "cloud", ExpectedStatus: 200, TimeoutSeconds: 10},

		// Shopping
		{Name: "Amazon", URL: "https://www.amazon.com", Category: "shopping", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "eBay", URL: "https://www.ebay.com", Category: "shopping", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "AliExpress", URL: "https://www.aliexpress.com", Category: "shopping", ExpectedStatus: 200, TimeoutSeconds: 10},

		// Corporate
		{Name: "Microsoft", URL: "https://www.microsoft.com", Category: "corporate", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Apple", URL: "https://www.apple.com", Category: "corporate", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Google", URL: "https://about.google.com", Category: "corporate", ExpectedStatus: 200, TimeoutSeconds: 10},

		// Speed Tests
		{Name: "Fast.com", URL: "https://fast.com", Category: "speedtest", ExpectedStatus: 200, TimeoutSeconds: 15},
		{Name: "Speedtest.net", URL: "https://www.speedtest.net", Category: "speedtest", ExpectedStatus: 200, TimeoutSeconds: 15},

		// Other
		{Name: "PayPal", URL: "https://www.paypal.com", Category: "finance", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Zoom", URL: "https://zoom.us", Category: "communication", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "WhatsApp Web", URL: "https://web.whatsapp.com", Category: "communication", ExpectedStatus: 200, TimeoutSeconds: 10},
		{Name: "Telegram Web", URL: "https://web.telegram.org", Category: "communication", ExpectedStatus: 200, TimeoutSeconds: 10},
	}
}

// LoadTargetSitesFromFile loads target sites from a JSON file
func LoadTargetSitesFromFile(path string) ([]TargetSite, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultTargetSites(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config TargetSitesConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config.Sites, nil
}

// SaveTargetSitesToFile saves target sites to a JSON file
func SaveTargetSitesToFile(sites []TargetSite, path string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	config := TargetSitesConfig{
		Version: 1,
		Sites:   sites,
		TestSettings: TestSettings{
			MaxConcurrentTests: 10,
			RequestTimeout:     15,
			RetryCount:         2,
			UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) ConfigTester/1.0",
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	// Rename to final path (atomic operation)
	return os.Rename(tmpPath, path)
}

// GetSiteCategories returns all unique categories from a list of sites
func GetSiteCategories(sites []TargetSite) []string {
	categories := make(map[string]bool)
	for _, site := range sites {
		if site.Category != "" {
			categories[site.Category] = true
		}
	}

	result := []string{}
	for category := range categories {
		result = append(result, category)
	}

	return result
}

// FilterSitesByCategory filters sites by category
func FilterSitesByCategory(sites []TargetSite, category string) []TargetSite {
	if category == "" {
		return sites
	}

	filtered := []TargetSite{}
	for _, site := range sites {
		if site.Category == category {
			filtered = append(filtered, site)
		}
	}

	return filtered
}

// GetSiteByName finds a site by name
func GetSiteByName(sites []TargetSite, name string) *TargetSite {
	for i, site := range sites {
		if site.Name == name {
			return &sites[i]
		}
	}
	return nil
}
