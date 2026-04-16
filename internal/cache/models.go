package cache

import (
	"encoding/json"
	"time"
)

// CacheEntry represents a single cached item with its timestamp.
type CacheEntry struct {
	Data     json.RawMessage `json:"data"`
	CachedAt time.Time       `json:"cachedAt"`
}

// CacheFile represents the on-disk cache structure.
type CacheFile struct {
	Entries map[string]CacheEntry `json:"entries"`
}
