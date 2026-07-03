package scheduler

import (
	"testing"
	"time"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

func TestDetectUnexpectedContainerStops_BaselineNoAlert(t *testing.T) {
	store := storage.NewFakeStore()
	docker := internal.DockerInfo{
		Available: true,
		Containers: []internal.ContainerInfo{
			{Name: "firefox", State: "created"},
			{Name: "plex", State: "exited"},
		},
	}

	got, err := detectUnexpectedContainerStops(store, docker, time.Now().UTC())
	if err != nil {
		t.Fatalf("detectUnexpectedContainerStops: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no alerts on first observation, got %v", got)
	}
}

func TestDetectUnexpectedContainerStops_OnlyRunningToStopped(t *testing.T) {
	store := storage.NewFakeStore()
	now := time.Now().UTC()

	if err := store.SaveContainerState(storage.ContainerState{
		ContainerName: "plex",
		State:         "running",
		ObservedAt:    now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed plex state: %v", err)
	}
	if err := store.SaveContainerState(storage.ContainerState{
		ContainerName: "firefox",
		State:         "created",
		ObservedAt:    now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed firefox state: %v", err)
	}

	docker := internal.DockerInfo{
		Available: true,
		Containers: []internal.ContainerInfo{
			{Name: "plex", State: "exited"},
			{Name: "firefox", State: "created"},
			{Name: "radarr", State: "running"},
		},
	}

	got, err := detectUnexpectedContainerStops(store, docker, now)
	if err != nil {
		t.Fatalf("detectUnexpectedContainerStops: %v", err)
	}
	if len(got) != 1 || got[0] != "plex" {
		t.Fatalf("expected only plex transition alert, got %v", got)
	}
}

func TestEvalDocker_StoppedRequiresUnexpectedTransition(t *testing.T) {
	docker := internal.DockerInfo{
		Available: true,
		Containers: []internal.ContainerInfo{
			{Name: "firefox", State: "created", Image: "firefox"},
			{Name: "plex", State: "exited", Image: "plex"},
		},
	}

	steady := evalDocker("stopped", "", docker, unexpectedStopSet(nil))
	if len(steady) != 0 {
		t.Fatalf("expected no steady-state stopped alerts, got %+v", steady)
	}

	transition := evalDocker("stopped", "", docker, unexpectedStopSet([]string{"plex"}))
	if len(transition) != 1 {
		t.Fatalf("expected 1 transition alert, got %+v", transition)
	}
	if transition[0].Title != "Container stopped: plex" {
		t.Fatalf("unexpected title: %q", transition[0].Title)
	}
}