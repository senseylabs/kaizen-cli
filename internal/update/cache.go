package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const cacheTTL = 24 * time.Hour

type updateCache struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

func cachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kaizen", "update-check.json")
}

// loadCache reads the cached update check result.
// Returns the cache and true if valid (not expired), false otherwise.
func loadCache() (*updateCache, bool) {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return nil, false
	}

	var cached updateCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}

	if time.Since(cached.CheckedAt) > cacheTTL {
		return nil, false
	}

	return &cached, true
}

// saveCache writes the update check result to disk.
func saveCache(latestVersion string) {
	cached := updateCache{
		LatestVersion: latestVersion,
		CheckedAt:     time.Now(),
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}

	dir := filepath.Dir(cachePath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}

	_ = os.WriteFile(cachePath(), data, 0600)
}
