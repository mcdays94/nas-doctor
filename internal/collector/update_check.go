// OS update checker — compares installed version against latest available.
// Uses platform-specific sources: Unraid PLG endpoint, GitHub API for others.
// Caches results for 24 hours.
package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

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
		InstalledVersion: normalizeVersion(installedVersion),
	}

	if info.InstalledVersion == "" {
		info.Error = "installed version not detected"
		return info
	}

	latest, err := getLatestVersion(platform)
	if err != nil {
		info.Error = "failed to check: " + err.Error()
		return info
	}

	info.LatestVersion = latest.version
	info.ReleaseName = latest.name
	info.ReleaseURL = latest.url
	info.CheckedAt = latest.checkedAt.Format(time.RFC3339)

	if latest.version != "" && info.InstalledVersion != "" {
		info.UpdateAvailable = isNewerVersion(latest.version, info.InstalledVersion)
	}

	return info
}

// getLatestVersion returns the latest version from cache or fresh fetch.
func getLatestVersion(platform string) (*cachedVersion, error) {
	versionCacheMu.RLock()
	cached, ok := versionCache[platform]
	versionCacheMu.RUnlock()

	if ok && time.Since(cached.checkedAt) < versionCacheTTL {
		return cached, nil
	}

	var latest *cachedVersion
	var err error

	switch platform {
	case "unraid":
		latest, err = fetchUnraidLatest()
	case "truenas":
		latest, err = fetchGitHubLatestRelease("truenas/middleware")
	default:
		return nil, fmt.Errorf("update checks not supported for %s", platform)
	}

	if err != nil {
		if ok {
			return cached, nil // return stale cache on failure
		}
		return nil, err
	}

	versionCacheMu.Lock()
	versionCache[platform] = latest
	versionCacheMu.Unlock()

	return latest, nil
}

// ── Unraid: fetch from official PLG endpoint ────────────────────────

// Unraid publishes its latest version in the stable PLG file at:
// https://stable.dl.unraid.net/unRAIDServer.plg
// The version is in an XML entity: <!ENTITY version "7.0.1">
var unraidVersionRegex = regexp.MustCompile(`<!ENTITY\s+version\s+"([^"]+)"`)

func fetchUnraidLatest() (*cachedVersion, error) {
	url := "https://stable.dl.unraid.net/unRAIDServer.plg"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch Unraid PLG: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Unraid PLG returned %d", resp.StatusCode)
	}

	// Read first 2KB — the version entity is near the top
	buf := make([]byte, 2048)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	matches := unraidVersionRegex.FindStringSubmatch(body)
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not parse version from Unraid PLG")
	}

	version := matches[1]

	// Also extract the release notes URL from CHANGES section
	releaseURL := fmt.Sprintf("https://docs.unraid.net/unraid-os/release-notes/%s/", version)

	return &cachedVersion{
		version:   version,
		name:      "Unraid " + version,
		url:       releaseURL,
		checkedAt: time.Now(),
	}, nil
}

// ── GitHub: for TrueNAS and other platforms ─────────────────────────

type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

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

	return &cachedVersion{
		version:   normalizeVersion(release.TagName),
		name:      release.Name,
		url:       release.HTMLURL,
		checkedAt: time.Now(),
	}, nil
}

// ── Version comparison utilities ────────────────────────────────────

// normalizeVersion strips common prefixes ("v", "V"), quotes, and key= prefixes.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	// Handle Unraid format: version="7.1.4"
	if strings.Contains(v, "=") {
		parts := strings.SplitN(v, "=", 2)
		v = parts[len(parts)-1]
	}
	v = strings.Trim(v, "\"'")
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return strings.TrimSpace(v)
}

// isNewerVersion returns true if latest is newer than installed.
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
	return len(latestParts) > len(installedParts)
}

// parseVersion splits a version string into numeric parts.
// "7.1.4" -> [7, 1, 4], "6.12.10-Unraid" -> [6, 12, 10]
func parseVersion(v string) []int {
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
