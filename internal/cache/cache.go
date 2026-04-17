package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/senseylabs/kaizen-cli/internal/client"
)

var (
	cachePath string
	mu        sync.Mutex
	uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func init() {
	home, _ := os.UserHomeDir()
	cachePath = filepath.Join(home, ".kaizen", "cache.json")
}

func loadFile() (*CacheFile, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &CacheFile{Entries: make(map[string]CacheEntry)}, nil
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cf CacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		// Corrupted cache — start fresh
		return &CacheFile{Entries: make(map[string]CacheEntry)}, nil
	}
	if cf.Entries == nil {
		cf.Entries = make(map[string]CacheEntry)
	}
	return &cf, nil
}

func saveFile(cf *CacheFile) error {
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize cache: %w", err)
	}

	return os.WriteFile(cachePath, data, 0600)
}

// Get retrieves a cached entry if it exists and hasn't expired.
func Get(key string, ttl time.Duration) (json.RawMessage, bool) {
	mu.Lock()
	defer mu.Unlock()

	cf, err := loadFile()
	if err != nil {
		return nil, false
	}

	entry, ok := cf.Entries[key]
	if !ok {
		return nil, false
	}

	if time.Since(entry.CachedAt) > ttl {
		return nil, false
	}

	return entry.Data, true
}

// Set stores data in the cache under the given key.
func Set(key string, data interface{}) error {
	mu.Lock()
	defer mu.Unlock()

	cf, err := loadFile()
	if err != nil {
		cf = &CacheFile{Entries: make(map[string]CacheEntry)}
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize cache data: %w", err)
	}

	cf.Entries[key] = CacheEntry{
		Data:     raw,
		CachedAt: time.Now(),
	}

	return saveFile(cf)
}

// Delete removes a single entry from the cache.
func Delete(key string) error {
	mu.Lock()
	defer mu.Unlock()

	cf, err := loadFile()
	if err != nil {
		return nil
	}

	delete(cf.Entries, key)
	return saveFile(cf)
}

// Clear removes all cache entries.
func Clear() error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear cache: %w", err)
	}
	return nil
}

// boardInfo is a lightweight board representation for caching.
type boardInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

// boardWithChildren extends boardInfo with child boards for resolution.
type boardWithChildren struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Prefix      string      `json:"prefix"`
	ChildBoards []boardInfo `json:"childBoards"`
}

// findBoardByName searches parent and child boards by name (case-insensitive).
func findBoardByName(boards []boardWithChildren, nameOrID string) (string, bool) {
	for _, b := range boards {
		if strings.EqualFold(b.Name, nameOrID) {
			return b.ID, true
		}
		for _, ch := range b.ChildBoards {
			if strings.EqualFold(ch.Name, nameOrID) {
				return ch.ID, true
			}
		}
	}
	return "", false
}

// collectBoardNames returns all parent and child board names for error messages.
func collectBoardNames(boards []boardWithChildren) []string {
	var names []string
	for _, b := range boards {
		names = append(names, b.Name)
		for _, ch := range b.ChildBoards {
			names = append(names, ch.Name)
		}
	}
	return names
}

// ResolveBoard resolves a board name or UUID to a board UUID.
// If the input is already a UUID, it is returned as-is.
// Otherwise, boards are fetched (with caching) and the name is matched case-insensitively.
// Both parent and child boards are searched.
func ResolveBoard(nameOrID string, c *client.KaizenClient) (string, error) {
	// UUID — return directly
	if uuidRegex.MatchString(strings.ToLower(nameOrID)) {
		return nameOrID, nil
	}

	boardsTTL := 10 * time.Minute

	// Try cache first
	cached, ok := Get("boards", boardsTTL)
	if ok {
		var boards []boardWithChildren
		if json.Unmarshal(cached, &boards) == nil {
			if id, found := findBoardByName(boards, nameOrID); found {
				return id, nil
			}
		}
	}

	// Cache miss or name not found — fetch from API
	body, err := c.Get("/kaizen/boards?includeChildren=true&amount=100")
	if err != nil {
		return "", fmt.Errorf("failed to fetch boards: %w", err)
	}

	var resp client.APIResponse[[]boardWithChildren]
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse boards response: %w", err)
	}

	// Update cache
	_ = Set("boards", resp.Data)

	if id, found := findBoardByName(resp.Data, nameOrID); found {
		return id, nil
	}

	names := collectBoardNames(resp.Data)
	return "", fmt.Errorf("board %q not found. Available boards: %s", nameOrID, strings.Join(names, ", "))
}
