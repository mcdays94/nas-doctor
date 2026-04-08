// Package collector — platform detection singleton.
//
// Detected once on first Collect() call and cached for the process lifetime.
// All collectors query this to gate platform-specific logic.
package collector

import (
	"bufio"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
)

// Platform holds the detected NAS operating system and version.
type Platform struct {
	Name    string // "unraid", "synology", "truenas", "qnap", "proxmox", "linux"
	Version string // e.g. "7.1.4", "7.2-64570"
}

// Normalized platform name constants.
const (
	PlatformUnraid   = "unraid"
	PlatformSynology = "synology"
	PlatformTrueNAS  = "truenas"
	PlatformQNAP     = "qnap"
	PlatformProxmox  = "proxmox"
	PlatformLinux    = "linux"
)

func (p Platform) IsUnraid() bool   { return p.Name == PlatformUnraid }
func (p Platform) IsSynology() bool { return p.Name == PlatformSynology }
func (p Platform) IsTrueNAS() bool  { return p.Name == PlatformTrueNAS }
func (p Platform) IsQNAP() bool     { return p.Name == PlatformQNAP }
func (p Platform) IsProxmox() bool  { return p.Name == PlatformProxmox }

var (
	detectedPlatform Platform
	platformOnce     sync.Once
)

// DetectPlatform runs OS detection and caches the result.
// Safe to call from multiple goroutines; detection runs only once.
func DetectPlatform(hp internal.HostPaths) Platform {
	platformOnce.Do(func() {
		detectedPlatform = runDetection(hp)
	})
	return detectedPlatform
}

// GetPlatform returns the cached platform. Must call DetectPlatform first.
func GetPlatform() Platform {
	return detectedPlatform
}

func runDetection(hp internal.HostPaths) Platform {
	p := Platform{}

	// ── 1. Unraid ────────────────────────────────────────────────────
	// Check Unraid ident.cfg on flash drive
	if _, err := os.Stat(hp.Boot + "/config/ident.cfg"); err == nil {
		p.Name = PlatformUnraid
	}
	// Try /etc/unraid-version (host or bind-mounted)
	for _, path := range []string{"/etc/unraid-version", "/host/etc/unraid-version"} {
		if data, err := os.ReadFile(path); err == nil {
			p.Name = PlatformUnraid
			raw := strings.TrimSpace(string(data))
			if strings.Contains(raw, "=") {
				parts := strings.SplitN(raw, "=", 2)
				raw = parts[len(parts)-1]
			}
			p.Version = strings.Trim(raw, "\"'")
			break
		}
	}
	// Fallback: kernel version string contains "-Unraid"
	if p.Name == "" {
		if data, err := os.ReadFile("/proc/version"); err == nil {
			if strings.Contains(string(data), "-Unraid") {
				p.Name = PlatformUnraid
			}
		}
	}
	if p.Name == PlatformUnraid {
		return p
	}

	// ── 2. Synology DSM ─────────────────────────────────────────────
	// Primary: /etc/synoinfo.conf exists on all Synology DSM installs.
	// Secondary: /etc.defaults/synoinfo.conf (also Synology-specific).
	// Inside a container, check both host-mounted and local paths.
	for _, path := range []string{
		"/etc/synoinfo.conf",
		"/host/etc/synoinfo.conf",
		"/etc.defaults/synoinfo.conf",
	} {
		if _, err := os.Stat(path); err == nil {
			p.Name = PlatformSynology
			break
		}
	}
	// Also detect via /etc/os-release ID or kernel version
	if p.Name == "" {
		if data, err := os.ReadFile("/proc/version"); err == nil {
			vs := string(data)
			if strings.Contains(vs, "synology") || strings.Contains(vs, "Synology") {
				p.Name = PlatformSynology
			}
		}
	}
	if p.Name == PlatformSynology {
		// Try to get DSM version from /etc/VERSION or /etc.defaults/VERSION
		for _, path := range []string{"/etc/VERSION", "/etc.defaults/VERSION", "/host/etc/VERSION"} {
			if data, err := os.ReadFile(path); err == nil {
				p.Version = parseSynoVersion(string(data))
				if p.Version != "" {
					break
				}
			}
		}
		return p
	}

	// ── 3. TrueNAS SCALE ────────────────────────────────────────────
	if procVer, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(string(procVer), "+truenas") {
			p.Name = PlatformTrueNAS
			for _, path := range []string{"/host/etc/version", "/etc/version"} {
				if data, err := os.ReadFile(path); err == nil {
					ver := strings.TrimSpace(string(data))
					if ver != "" {
						p.Version = ver
						break
					}
				}
			}
			if p.Version == "" {
				p.Version = fetchTrueNASVersionAPI()
			}
			return p
		}
	}

	// ── 4. QNAP ─────────────────────────────────────────────────────
	for _, path := range []string{"/etc/config/uLinux.conf", "/host/etc/config/uLinux.conf"} {
		if _, err := os.Stat(path); err == nil {
			p.Name = PlatformQNAP
			break
		}
	}
	if p.Name == PlatformQNAP {
		// Try reading QTS version from /etc/config/uLinux.conf
		if data, err := os.ReadFile("/etc/config/uLinux.conf"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "QTS_VER=") || strings.HasPrefix(line, "FIRMWARE_VER=") {
					p.Version = strings.Trim(strings.SplitN(line, "=", 2)[1], "\"' \n")
					break
				}
			}
		}
		return p
	}

	// ── 5. Proxmox VE ───────────────────────────────────────────────
	for _, path := range []string{"/etc/pve", "/host/etc/pve"} {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			p.Name = PlatformProxmox
			break
		}
	}
	if p.Name == PlatformProxmox {
		// pveversion output: "pve-manager/8.2.4/..."
		if out, err := execCmd("pveversion"); err == nil {
			parts := strings.Split(strings.TrimSpace(out), "/")
			if len(parts) >= 2 {
				p.Version = parts[1]
			}
		}
		return p
	}

	// ── 6. Generic Linux (fallback via /etc/os-release) ─────────────
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "ID=") {
				p.Name = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			}
			if strings.HasPrefix(line, "VERSION_ID=") {
				p.Version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
	}
	if p.Name == "" {
		p.Name = PlatformLinux
	}
	return p
}

// parseSynoVersion parses Synology's /etc/VERSION file format:
//
//	majorversion="7"
//	minorversion="2"
//	buildnumber="64570"
func parseSynoVersion(content string) string {
	var major, minor, build string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "majorversion=") {
			major = strings.Trim(strings.TrimPrefix(line, "majorversion="), "\"")
		}
		if strings.HasPrefix(line, "minorversion=") {
			minor = strings.Trim(strings.TrimPrefix(line, "minorversion="), "\"")
		}
		if strings.HasPrefix(line, "buildnumber=") {
			build = strings.Trim(strings.TrimPrefix(line, "buildnumber="), "\"")
		}
	}
	if major == "" {
		return ""
	}
	ver := major
	if minor != "" {
		ver += "." + minor
	}
	if build != "" {
		ver += "-" + build
	}
	return ver
}

// fetchTrueNASVersionAPI queries the TrueNAS local API for system version.
func fetchTrueNASVersionAPI() string {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost/api/v2.0/system/version")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	ver := strings.Trim(strings.TrimSpace(string(buf[:n])), "\"")
	return ver
}
