package collector

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// collectBackups detects and queries backup tools: Borg, Restic, PBS, Duplicati, Rclone.
func collectBackups() *internal.BackupInfo {
	info := &internal.BackupInfo{Available: false}

	// Try each provider
	borgJobs := collectBorg()
	resticJobs := collectRestic()
	pbsJobs := collectPBS()
	duplicatiJobs := collectDuplicati()

	info.Jobs = append(info.Jobs, borgJobs...)
	info.Jobs = append(info.Jobs, resticJobs...)
	info.Jobs = append(info.Jobs, pbsJobs...)
	info.Jobs = append(info.Jobs, duplicatiJobs...)

	if len(info.Jobs) > 0 {
		info.Available = true
	}
	return info
}

// ---------- Borg ----------

func collectBorg() []internal.BackupJob {
	// Check if borg is available
	if _, err := exec.LookPath("borg"); err != nil {
		return nil
	}

	// Try common repo locations
	repos := findBorgRepos()
	var jobs []internal.BackupJob

	for _, repo := range repos {
		job := queryBorgRepo(repo)
		if job != nil {
			jobs = append(jobs, *job)
		}
	}
	return jobs
}

func findBorgRepos() []string {
	var repos []string
	// Check BORG_REPO env
	if out, err := execCmd("sh", "-c", "echo $BORG_REPO"); err == nil && strings.TrimSpace(out) != "" {
		repos = append(repos, strings.TrimSpace(out))
	}
	// Scan common locations
	for _, pattern := range []string{
		"/mnt/*/backups/borg",
		"/mnt/backup*/borg",
		"/backup/borg",
		"/volume*/backups/borg",
	} {
		matches, _ := filepath.Glob(pattern)
		repos = append(repos, matches...)
	}
	return repos
}

func queryBorgRepo(repo string) *internal.BackupJob {
	// borg info --json <repo>
	out, err := execCmd("borg", "info", "--json", repo)
	if err != nil {
		return nil
	}

	var borgInfo struct {
		Repository struct {
			Location string `json:"location"`
		} `json:"repository"`
		Archives []struct {
			Name  string `json:"name"`
			Start string `json:"start"`
			End   string `json:"end"`
			Stats struct {
				OriginalSize int64 `json:"original_size"`
				NFiles       int   `json:"nfiles"`
			} `json:"stats"`
		} `json:"archives"`
		Encryption struct {
			Mode string `json:"mode"`
		} `json:"encryption"`
	}

	if err := json.Unmarshal([]byte(out), &borgInfo); err != nil {
		return nil
	}

	job := &internal.BackupJob{
		Provider:      "borg",
		Name:          filepath.Base(repo),
		Repository:    repo,
		SnapshotCount: len(borgInfo.Archives),
		Encrypted:     borgInfo.Encryption.Mode != "none" && borgInfo.Encryption.Mode != "",
	}

	if len(borgInfo.Archives) > 0 {
		latest := borgInfo.Archives[len(borgInfo.Archives)-1]
		if t, err := time.Parse("2006-01-02T15:04:05.000000", latest.Start); err == nil {
			job.LastRun = t
			job.LastSuccess = t
		}
		if t1, err1 := time.Parse("2006-01-02T15:04:05.000000", latest.Start); err1 == nil {
			if t2, err2 := time.Parse("2006-01-02T15:04:05.000000", latest.End); err2 == nil {
				job.Duration = t2.Sub(t1).Seconds()
			}
		}
		job.SizeBytes = latest.Stats.OriginalSize
		job.FilesCount = latest.Stats.NFiles
	}

	// Determine status based on age
	job.Status = backupStatus(job.LastSuccess)
	return job
}

// ---------- Restic ----------

func collectRestic() []internal.BackupJob {
	if _, err := exec.LookPath("restic"); err != nil {
		return nil
	}

	// Check RESTIC_REPOSITORY env
	repo := ""
	if out, err := execCmd("sh", "-c", "echo $RESTIC_REPOSITORY"); err == nil {
		repo = strings.TrimSpace(out)
	}
	if repo == "" {
		return nil
	}

	// restic snapshots --json --latest 1
	out, err := execCmd("restic", "snapshots", "--json", "--latest", "1")
	if err != nil {
		return nil
	}

	var snapshots []struct {
		Time     string   `json:"time"`
		Hostname string   `json:"hostname"`
		Paths    []string `json:"paths"`
		ShortID  string   `json:"short_id"`
	}
	if err := json.Unmarshal([]byte(out), &snapshots); err != nil {
		return nil
	}

	job := &internal.BackupJob{
		Provider:   "restic",
		Name:       "restic",
		Repository: repo,
	}

	if len(snapshots) > 0 {
		latest := snapshots[0]
		if t, err := time.Parse(time.RFC3339Nano, latest.Time); err == nil {
			job.LastRun = t
			job.LastSuccess = t
		}
		job.Name = strings.Join(latest.Paths, ", ")
	}

	// Get stats
	if statsOut, err := execCmd("restic", "stats", "--json"); err == nil {
		var stats struct {
			TotalSize      int64 `json:"total_size"`
			TotalFileCount int   `json:"total_file_count"`
			SnapshotsCount int   `json:"snapshots_count"`
		}
		if json.Unmarshal([]byte(statsOut), &stats) == nil {
			job.SizeBytes = stats.TotalSize
			job.FilesCount = stats.TotalFileCount
			job.SnapshotCount = stats.SnapshotsCount
		}
	}

	job.Status = backupStatus(job.LastSuccess)
	return []internal.BackupJob{*job}
}

// ---------- Proxmox Backup Server (PBS) ----------

func collectPBS() []internal.BackupJob {
	if _, err := exec.LookPath("proxmox-backup-client"); err != nil {
		return nil
	}

	out, err := execCmd("proxmox-backup-client", "snapshot", "list", "--output-format", "json")
	if err != nil {
		return nil
	}

	var snapshots []struct {
		BackupType string `json:"backup-type"`
		BackupID   string `json:"backup-id"`
		BackupTime int64  `json:"backup-time"`
		Size       int64  `json:"size"`
	}
	if err := json.Unmarshal([]byte(out), &snapshots); err != nil {
		return nil
	}

	if len(snapshots) == 0 {
		return nil
	}

	// Group by backup-id
	latest := snapshots[len(snapshots)-1]
	job := &internal.BackupJob{
		Provider:      "pbs",
		Name:          fmt.Sprintf("%s/%s", latest.BackupType, latest.BackupID),
		Repository:    "PBS",
		LastRun:       time.Unix(latest.BackupTime, 0),
		LastSuccess:   time.Unix(latest.BackupTime, 0),
		SizeBytes:     latest.Size,
		SnapshotCount: len(snapshots),
		Encrypted:     true,
	}
	job.Status = backupStatus(job.LastSuccess)
	return []internal.BackupJob{*job}
}

// ---------- Duplicati ----------

func collectDuplicati() []internal.BackupJob {
	if _, err := exec.LookPath("duplicati-cli"); err != nil {
		return nil
	}

	// Duplicati stores its DB in a known location
	// Try to query via the CLI
	out, err := execCmd("duplicati-cli", "list-broken-files", "--dbpath=/data/duplicati-config")
	if err != nil {
		// Try reading the server API instead
		out, err = execCmd("curl", "-sf", "http://localhost:8200/api/v1/backups")
		if err != nil {
			return nil
		}
	}
	_ = out // Parse Duplicati API response if available
	return nil
}

// ---------- Helpers ----------

func backupStatus(lastSuccess time.Time) string {
	if lastSuccess.IsZero() {
		return "failed"
	}
	age := time.Since(lastSuccess)
	switch {
	case age < 25*time.Hour:
		return "ok"
	case age < 49*time.Hour:
		return "warning"
	default:
		return "stale"
	}
}

// formatBytes formats bytes into human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
