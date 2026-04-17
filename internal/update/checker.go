package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	githubReleasesURL = "https://api.github.com/repos/senseylabs/kaizen-cli/releases/latest"
	httpTimeout       = 2 * time.Second
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// CheckForUpdate checks if a newer version is available.
// Returns the latest version string (e.g. "v0.2.0") if newer, or empty string otherwise.
// Silently returns empty on any error.
func CheckForUpdate(currentVersion string) string {
	if currentVersion == "" || currentVersion == "dev" {
		return ""
	}

	// Check cache first
	cached, ok := loadCache()
	if ok {
		return compareVersions(currentVersion, cached.LatestVersion)
	}

	// Fetch from GitHub
	latestVersion := fetchLatestVersion()
	if latestVersion == "" {
		return ""
	}

	// Save to cache
	saveCache(latestVersion)

	return compareVersions(currentVersion, latestVersion)
}

func fetchLatestVersion() string {
	httpClient := &http.Client{Timeout: httpTimeout}

	req, err := http.NewRequest("GET", githubReleasesURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	if release.TagName == "" {
		return ""
	}

	return release.TagName
}

// compareVersions returns latestVersion if it is newer than currentVersion, empty string otherwise.
func compareVersions(current, latest string) string {
	currentParts := parseSemver(current)
	latestParts := parseSemver(latest)

	if currentParts == nil || latestParts == nil {
		return ""
	}

	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return latest
		}
		if latestParts[i] < currentParts[i] {
			return ""
		}
	}

	return ""
}

// parseSemver parses a version string like "v1.2.3" or "1.2.3" into [major, minor, patch].
func parseSemver(version string) []int {
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return nil
	}

	result := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		result[i] = n
	}
	return result
}

// FormatNotice returns the update notice string.
func FormatNotice(currentVersion, latestVersion string, useColor bool) string {
	if useColor {
		bold := "\033[1m"
		yellow := "\033[33m"
		reset := "\033[0m"
		return fmt.Sprintf("\n%s%sUpdate available! %s → %s%s\nRun: brew upgrade kaizen-cli\n", bold, yellow, currentVersion, latestVersion, reset)
	}
	return fmt.Sprintf("\nUpdate available! %s → %s\nRun: brew upgrade kaizen-cli\n", currentVersion, latestVersion)
}
