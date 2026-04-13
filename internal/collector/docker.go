package collector

import (
	"strconv"
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

func collectDocker() (internal.DockerInfo, error) {
	info := internal.DockerInfo{}

	// Check if Docker is available
	if _, err := execCmd("docker", "version"); err != nil {
		return info, nil // Docker not available, not an error
	}
	info.Available = true

	// List all containers
	out, err := execCmd("docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.State}}")
	if err != nil {
		return info, err
	}

	containerMap := map[string]*internal.ContainerInfo{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}
		c := &internal.ContainerInfo{
			ID:     fields[0],
			Name:   fields[1],
			Image:  fields[2],
			Uptime: fields[3],
			State:  fields[4],
			Status: fields[3],
		}
		containerMap[c.Name] = c
		info.Containers = append(info.Containers, *c)
	}

	// Get resource usage for running containers
	statsOut, err := execCmd("docker", "stats", "--no-stream", "--format",
		"{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.NetIO}}\t{{.BlockIO}}")
	if err == nil {
		for _, line := range strings.Split(statsOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 6 {
				continue
			}
			name := fields[0]
			// Find container in our list and update
			for i := range info.Containers {
				if info.Containers[i].Name == name {
					cpuStr := strings.TrimSuffix(fields[1], "%")
					info.Containers[i].CPU, _ = strconv.ParseFloat(cpuStr, 64)

					// Parse mem usage: "100MiB / 1GiB"
					memParts := strings.Split(fields[2], "/")
					if len(memParts) >= 1 {
						info.Containers[i].MemMB = parseMemDocker(strings.TrimSpace(memParts[0]))
					}

					memPctStr := strings.TrimSuffix(fields[3], "%")
					info.Containers[i].MemPct, _ = strconv.ParseFloat(memPctStr, 64)

					// Parse net I/O: "1.5MB / 2.3MB"
					netParts := strings.Split(fields[4], "/")
					if len(netParts) == 2 {
						info.Containers[i].NetIn = parseDockerBytes(strings.TrimSpace(netParts[0]))
						info.Containers[i].NetOut = parseDockerBytes(strings.TrimSpace(netParts[1]))
					}

					// Parse block I/O: "100MB / 50MB"
					blkParts := strings.Split(fields[5], "/")
					if len(blkParts) == 2 {
						info.Containers[i].BlockRead = parseDockerBytes(strings.TrimSpace(blkParts[0]))
						info.Containers[i].BlockWrite = parseDockerBytes(strings.TrimSpace(blkParts[1]))
					}
					break
				}
			}
		}
	}

	return info, nil
}

func parseMemDocker(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "GiB") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "GiB"), 64)
		return v * 1024
	}
	if strings.HasSuffix(s, "MiB") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "MiB"), 64)
		return v
	}
	if strings.HasSuffix(s, "KiB") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "KiB"), 64)
		return v / 1024
	}
	return 0
}

// parseDockerBytes parses docker stats byte strings (e.g. "1.5GB", "100MB", "50kB", "0B") into bytes.
// Docker uses SI units (kB=1000, MB=1e6, GB=1e9, TB=1e12).
func parseDockerBytes(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "0B" || s == "" || s == "--" {
		return 0
	}
	suffixes := []struct {
		suffix string
		mult   float64
	}{
		{"TB", 1e12},
		{"GB", 1e9},
		{"MB", 1e6},
		{"kB", 1e3},
		{"B", 1},
	}
	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			v, _ := strconv.ParseFloat(strings.TrimSuffix(s, sf.suffix), 64)
			return v * sf.mult
		}
	}
	return 0
}
