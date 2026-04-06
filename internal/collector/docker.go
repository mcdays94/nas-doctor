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
	statsOut, err := execCmd("docker", "stats", "--no-stream", "--format", "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}")
	if err == nil {
		for _, line := range strings.Split(statsOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 4 {
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
