package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ContainerState records the last-observed Docker container state so the
// scheduler can detect running → stopped transitions across scans.
type ContainerState struct {
	ContainerName string
	State         string
	ObservedAt    time.Time
}

// GetContainerState returns the last-known state for containerName, or
// (nil, nil) if no state has been recorded yet.
func (d *DB) GetContainerState(containerName string) (*ContainerState, error) {
	var state ContainerState
	err := d.db.QueryRow(
		`SELECT container_name, state, observed_at
		 FROM container_state WHERE container_name = ?`,
		containerName,
	).Scan(&state.ContainerName, &state.State, &state.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query container_state: %w", err)
	}
	return &state, nil
}

// SaveContainerState UPSERTs the last-observed state for a container.
func (d *DB) SaveContainerState(state ContainerState) error {
	if state.ContainerName == "" {
		return fmt.Errorf("container_name is required")
	}
	if state.ObservedAt.IsZero() {
		state.ObservedAt = time.Now().UTC()
	}
	_, err := d.db.Exec(
		`INSERT INTO container_state (container_name, state, observed_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(container_name) DO UPDATE SET
		   state = excluded.state,
		   observed_at = excluded.observed_at`,
		state.ContainerName, state.State, state.ObservedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert container_state: %w", err)
	}
	return nil
}