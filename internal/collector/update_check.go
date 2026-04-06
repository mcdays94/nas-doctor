// OS update checker — compares installed version against latest available.
// Fetches latest version from GitHub releases, caches for 24 hours.
package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Platform-to-GitHub-repo mapping for version checks.
var platformRepos = map[string]string{
	"unraid":  "unraid/webgui",      // Unraid WebGUI releases track OS versions
	"truenas": "truenas/middleware", // TrueNAS SCALE middleware releases
}

// Cached latest version info per platform (refreshed every 24h).
var (
	versionCache   = make(map[string]*cachedVersion)
	versionCacheMu sync.RWMutex
)

type cachedVersion struct {
	version   string
	name      string
	url       string
	checkedAt time.Time
}

const versionCacheTTL = 24 * time.Hour

// collectUpdateInfo checks if the current OS version is up to date.
func collectUpdateInfo(platform, installedVersion string) *internal.UpdateInfo {
	info := &internal.UpdateInfo{
		Platform:         platform,
		InstalledVersion: installedVersion,
	}

	if installedVersion == "" {
		info.Error = "installed version not detected"
		return info
	}

	repo, ok := platformRepos[platform]
	if !ok {
		// Platform not supported for update checks
		info.Error = "update checks not supported for " + platform
		return info
	}

	latest, err := getLatestVersion(platform, repo)
	if err != nil {
		info.Error = "failed to check: " + err.Error()
		return info
	}

	info.LatestVersion = latest.version
	info.ReleaseName = latest.name
	info.ReleaseURL = latest.url
	info.CheckedAt = latest.checkedAt.Format(time.RFC3339)

	// Compare versions
	if latest.version != "" && installedVersion != "" {
		info.UpdateAvailable = isNewerVersion(latest.version, installedVersion)
	}

	return info
}

// getLatestVersion returns the latest version from cache or GitHub.
func getLatestVersion(platform, repo string) (*cachedVersion, error) {
	versionCacheMu.RLock()
	cached, ok := versionCache[platform]
	versionCacheMu.RUnlock()

	if ok && time.Since(cached.checkedAt) < versionCacheTTL {
		return cached, nil
	}

	// Fetch from GitHub releases API
	latest, err := fetchGitHubLatestRelease(repo)
	if err != nil {
		// If we have a stale cache, return it
		if ok {
			return cached, nil
		}
		return nil, err
	}

	versionCacheMu.Lock()
	versionCache[platform] = latest
	versionCacheMu.Unlock()

	return latest, nil
}

// githubRelease represents a GitHub API release response (minimal fields).
type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

// fetchGitHubLatestRelease fetches the latest release from GitHub API.
func fetchGitHubLatestRelease(repo string) (*cachedVersion, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "nas-doctor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	version := normalizeVersion(release.TagName)

	return &cachedVersion{
		version:   version,
		name:      release.Name,
		url:       release.HTMLURL,
		checkedAt: time.Now(),
	}, nil
}

// normalizeVersion strips common prefixes ("v", "V") and whitespace.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// isNewerVersion returns true if latest is newer than installed.
// Uses simple semantic versioning comparison (major.minor.patch).
func isNewerVersion(latest, installed string) bool {
	latestParts := parseVersion(latest)
	installedParts := parseVersion(installed)

	for i := 0; i < len(latestParts) && i < len(installedParts); i++ {
		if latestParts[i] > installedParts[i] {
			return true
		}
		if latestParts[i] < installedParts[i] {
			return false
		}
	}
	// If all compared parts are equal, newer if latest has more parts
	return len(latestParts) > len(installedParts)
}

// parseVersion splits a version string into numeric parts.
// "7.1.4" -> [7, 1, 4], "6.12.10-Unraid" -> [6, 12, 10]
func parseVersion(v string) []int {
	// Strip anything after a dash or plus (pre-release tags)
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	var nums []int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		nums = append(nums, n)
	}
	return nums
}
