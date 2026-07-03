package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// containerRunning reports whether a Docker container state should be
// treated as actively running.
func containerRunning(state string) bool {
	return strings.EqualFold(strings.TrimSpace(state), "running")
}

// detectUnexpectedContainerStops compares the current Docker scan against
// persisted per-container state and returns names that transitioned from
// running to a non-running state since the last observation.
//
// First observation of a container establishes baseline and emits no alert.
// Steady-state stopped or created containers therefore do not notify.
func detectUnexpectedContainerStops(store storage.ContainerStateStore, docker internal.DockerInfo, at time.Time) ([]string, error) {
	if !docker.Available || len(docker.Containers) == 0 {
		return nil, nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	var unexpected []string
	for _, c := range docker.Containers {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}

		prev, err := store.GetContainerState(name)
		if err != nil {
			return nil, fmt.Errorf("load container state for %s: %w", name, err)
		}

		currentState := strings.TrimSpace(c.State)
		if prev != nil && containerRunning(prev.State) && !containerRunning(currentState) {
			unexpected = append(unexpected, name)
		}

		if err := store.SaveContainerState(storage.ContainerState{
			ContainerName: name,
			State:         currentState,
			ObservedAt:    at,
		}); err != nil {
			return nil, fmt.Errorf("save container state for %s: %w", name, err)
		}
	}

	return unexpected, nil
}