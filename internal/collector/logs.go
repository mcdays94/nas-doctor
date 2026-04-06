package collector

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

var errorPattern = regexp.MustCompile(`(?i)(error|fail|reset|timeout|ata.*err|i/o|medium|offline|UNC|sector|reallocat|abort|DRDY|critical|emerg|panic|oops)`)
var warningPattern = regexp.MustCompile(`(?i)(warning|warn|degraded|retry)`)

func collectLogs(hp internal.HostPaths) (internal.LogInfo, error) {
	info := internal.LogInfo{}

	// Collect dmesg errors
	dmesgOut, err := execCmd("dmesg", "-T")
	if err == nil {
		info.DmesgErrors = filterLogEntries(dmesgOut, "dmesg", 200)
	}

	// Collect syslog errors
	syslogPaths := []string{
		filepath.Join(hp.Log, "syslog"),
		filepath.Join(hp.Log, "messages"),
		"/var/log/syslog",
		"/var/log/messages",
	}
	for _, path := range syslogPaths {
		data, err := os.ReadFile(path)
		if err == nil {
			info.SyslogErrors = filterLogEntries(string(data), "syslog", 100)
			break // use the first one that works
		}
	}

	return info, nil
}

func filterLogEntries(output, source string, maxEntries int) []internal.LogEntry {
	var entries []internal.LogEntry
	lines := strings.Split(output, "\n")

	// Process from the end (most recent) to get the latest errors
	for i := len(lines) - 1; i >= 0 && len(entries) < maxEntries; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		if errorPattern.MatchString(line) {
			entry := internal.LogEntry{
				Message: line,
				Source:  source,
				Level:   "error",
			}
			// Extract timestamp if dmesg format
			if strings.HasPrefix(line, "[") {
				if idx := strings.Index(line, "]"); idx > 0 {
					entry.Timestamp = strings.TrimSpace(line[1:idx])
					entry.Message = strings.TrimSpace(line[idx+1:])
				}
			}
			entries = append(entries, entry)
		} else if warningPattern.MatchString(line) {
			entry := internal.LogEntry{
				Message: line,
				Source:  source,
				Level:   "warning",
			}
			if strings.HasPrefix(line, "[") {
				if idx := strings.Index(line, "]"); idx > 0 {
					entry.Timestamp = strings.TrimSpace(line[1:idx])
					entry.Message = strings.TrimSpace(line[idx+1:])
				}
			}
			entries = append(entries, entry)
		}
	}

	// Reverse to get chronological order
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return entries
}
