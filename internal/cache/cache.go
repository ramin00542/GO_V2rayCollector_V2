// Package cache provides caching utilities for the collector
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache provides a simple in-memory cache with TTL support
type Cache struct {
	mu       sync.RWMutex
	data     map[string]cacheEntry
	basePath string
	defaultTTL time.Duration
}

type cacheEntry struct {
	value      []byte
	expiresAt  time.Time
	lastAccess time.Time
}

// NewCache creates a new cache with the specified TTL
func NewCache(basePath string, defaultTTL time.Duration) (*Cache, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	
	return &Cache{
		data:        make(map[string]cacheEntry),
		basePath:    basePath,
		defaultTTL: defaultTTL,
	}, nil
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, ok := c.data[key]
	if !ok {
		return nil, false
	}
	
	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	
	// Update last access time
	c.mu.RUnlock()
	c.mu.Lock()
	entry.lastAccess = time.Now()
	c.data[key] = entry
	c.mu.Unlock()
	c.mu.RLock()
	
	return entry.value, true
}

// Set stores a value in the cache
func (c *Cache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.data[key] = cacheEntry{
		value:      value,
		expiresAt:  time.Now().Add(c.defaultTTL),
		lastAccess: time.Now(),
	}
}

// SetWithTTL stores a value in the cache with a custom TTL
func (c *Cache) SetWithTTL(key string, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.data[key] = cacheEntry{
		value:      value,
		expiresAt:  time.Now().Add(ttl),
		lastAccess: time.Now(),
	}
}

// Delete removes a value from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	delete(c.data, key)
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.data = make(map[string]cacheEntry)
}

// Size returns the number of entries in the cache
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return len(c.data)
}

// Cleanup removes expired entries from the cache
func (c *Cache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	for key, entry := range c.data {
		if now.After(entry.expiresAt) {
			delete(c.data, key)
		}
	}
}

// SaveToDisk saves the cache to disk
func (c *Cache) SaveToDisk(filename string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Convert cache to serializable format
	type cacheData map[string]struct {
		Value      []byte   `json:"value"`
		ExpiresAt  string   `json:"expires_at"`
		LastAccess string   `json:"last_access"`
	}
	
	data := make(cacheData)
	for key, entry := range c.data {
		data[key] = struct {
			Value      []byte
			ExpiresAt  string
			LastAccess string
		}{
			Value:      entry.value,
			ExpiresAt:  entry.expiresAt.Format(time.RFC3339),
			LastAccess: entry.lastAccess.Format(time.RFC3339),
		}
	}
	
	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}
	
	// Write to file
	path := filepath.Join(c.basePath, filename)
	tmpPath := path + ".tmp"
	
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}
	
	return os.Rename(tmpPath, path)
}

// LoadFromDisk loads the cache from disk
func (c *Cache) LoadFromDisk(filename string) error {
	path := filepath.Join(c.basePath, filename)
	
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, that's fine
		}
		return fmt.Errorf("failed to read cache file: %w", err)
	}
	
	// Unmarshal JSON
	type cacheData map[string]struct {
		Value      []byte `json:"value"`
		ExpiresAt  string `json:"expires_at"`
		LastAccess string `json:"last_access"`
	}
	
	var loadedData cacheData
	if err := json.Unmarshal(data, &loadedData); err != nil {
		return fmt.Errorf("failed to unmarshal cache: %w", err)
	}
	
	// Convert to cache entries
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for key, entry := range loadedData {
		expiresAt, err := time.Parse(time.RFC3339, entry.ExpiresAt)
		if err != nil {
			continue
		}
		lastAccess, err := time.Parse(time.RFC3339, entry.LastAccess)
		if err != nil {
			continue
		}
		
		c.data[key] = cacheEntry{
			value:      entry.Value,
			expiresAt:  expiresAt,
			lastAccess: lastAccess,
		}
	}
	
	return nil
}

// HTTPResponseCache provides caching for HTTP responses
type HTTPResponseCache struct {
	cache *Cache
}

// NewHTTPResponseCache creates a new HTTP response cache
func NewHTTPResponseCache(basePath string, defaultTTL time.Duration) (*HTTPResponseCache, error) {
	cache, err := NewCache(basePath, defaultTTL)
	if err != nil {
		return nil, err
	}
	return &HTTPResponseCache{cache: cache}, nil
}

// Get retrieves a cached HTTP response
func (c *HTTPResponseCache) Get(key string) ([]byte, bool) {
	return c.cache.Get(key)
}

// Set stores an HTTP response in the cache
func (c *HTTPResponseCache) Set(key string, response []byte) {
	c.cache.Set(key, response)
}

// SetWithTTL stores an HTTP response with a custom TTL
func (c *HTTPResponseCache) SetWithTTL(key string, response []byte, ttl time.Duration) {
	c.cache.SetWithTTL(key, response, ttl)
}

// Cleanup removes expired entries
func (c *HTTPResponseCache) Cleanup() {
	c.cache.Cleanup()
}

// Size returns the number of entries in the cache
func (c *HTTPResponseCache) Size() int {
	return c.cache.Size()
}

// Clear clears all entries
func (c *HTTPResponseCache) Clear() {
	c.cache.Clear()
}

// CachedFetcher wraps a fetch function with caching
type CachedFetcher struct {
	fetchFunc func(ctx context.Context, url string) ([]byte, error)
	cache     *HTTPResponseCache
}

// NewCachedFetcher creates a new cached fetcher
func NewCachedFetcher(fetchFunc func(ctx context.Context, url string) ([]byte, error), cache *HTTPResponseCache) *CachedFetcher {
	return &CachedFetcher{
		fetchFunc: fetchFunc,
		cache:     cache,
	}
}

// Fetch fetches data with caching
func (f *CachedFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	// Check cache first
	if data, ok := f.cache.Get(url); ok {
		return data, nil
	}
	
	// Fetch from source
	data, err := f.fetchFunc(ctx, url)
	if err != nil {
		return nil, err
	}
	
	// Store in cache
	f.cache.Set(url, data)
	
	return data, nil
}

// ContextCache provides a context-aware cache
type ContextCache struct {
	cache *Cache
}

// NewContextCache creates a new context-aware cache
func NewContextCache(basePath string, defaultTTL time.Duration) (*ContextCache, error) {
	cache, err := NewCache(basePath, defaultTTL)
	if err != nil {
		return nil, err
	}
	return &ContextCache{cache: cache}, nil
}

// GetOrLoad retrieves a value from cache or loads it using the loader function
func (c *ContextCache) GetOrLoad(ctx context.Context, key string, loader func() ([]byte, error)) ([]byte, error) {
	// Check cache first
	if data, ok := c.cache.Get(key); ok {
		return data, nil
	}
	
	// Load the data
	data, err := loader()
	if err != nil {
		return nil, err
	}
	
	// Store in cache
	c.cache.Set(key, data)
	
	return data, nil
}

// GetOrLoadWithTTL retrieves a value from cache or loads it with a custom TTL
func (c *ContextCache) GetOrLoadWithTTL(ctx context.Context, key string, ttl time.Duration, loader func() ([]byte, error)) ([]byte, error) {
	// Check cache first
	if data, ok := c.cache.Get(key); ok {
		return data, nil
	}
	
	// Load the data
	data, err := loader()
	if err != nil {
		return nil, err
	}
	
	// Store in cache with custom TTL
	c.cache.SetWithTTL(key, data, ttl)
	
	return data, nil
}

// PeriodicCleanup starts a goroutine that periodically cleans up the cache
func (c *Cache) PeriodicCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			c.Cleanup()
		}
	}()
}

// StartPeriodicSave starts a goroutine that periodically saves the cache to disk
func (c *Cache) StartPeriodicSave(interval time.Duration, filename string) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			if err := c.SaveToDisk(filename); err != nil {
				// Log error but continue
				// In a real implementation, use proper logging
			}
		}
	}()
}
