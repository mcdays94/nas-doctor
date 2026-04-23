package scheduler

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/mcdays94/nas-doctor/internal"
	"github.com/mcdays94/nas-doctor/internal/collector"
	"github.com/mcdays94/nas-doctor/internal/storage"
)

// newObservabilityScheduler builds a scheduler whose log output is captured
// in the returned buffer so tests can assert log lines produced by
// collectContainerStats (issue #226).
//
// The scheduler is constructed with a real Collector (same as
// newTestScheduler) but a text-handler logger writing to a bytes.Buffer
// at Info level so Warn / Error / Info / Debug are all observable.
func newObservabilityScheduler(t *testing.T) (*Scheduler, *storage.FakeStore, *syncBuffer) {
	t.Helper()
	store := storage.NewFakeStore()
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	col := collector.New(internal.HostPaths{}, logger)
	s := New(col, store, nil, nil, logger, 0)
	return s, store, buf
}

// syncBuffer is a goroutine-safe bytes.Buffer wrapper. slog handlers
// can write from any goroutine; tests read the accumulated output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestCollectContainerStats_LogsErrorOnCollectorFailure verifies issue
// #226's primary observability gap: when CollectDockerStats returns an
// error, the 5-minute loop must emit a WARN/ERROR line so operators can
// see why container_stats_history stopped advancing. Previously the
// error was silently swallowed across 36+ hours of real-world data loss.
func TestCollectContainerStats_LogsErrorOnCollectorFailure(t *testing.T) {
	s, _, buf := newObservabilityScheduler(t)

	wantErr := errors.New("docker ps: connection refused")
	s.SetDockerStatsFn(func() (*internal.DockerInfo, error) {
		return nil, wantErr
	})

	s.collectContainerStats()

	out := buf.String()
	if !strings.Contains(out, "container stats") {
		t.Fatalf("expected log line mentioning 'container stats', got:\n%s", out)
	}
	// Must be at WARN or ERROR level (not DEBUG/INFO) so it surfaces in
	// default operator log views.
	if !strings.Contains(out, "level=WARN") && !strings.Contains(out, "level=ERROR") {
		t.Errorf("expected WARN or ERROR level log for collector failure, got:\n%s", out)
	}
	if !strings.Contains(out, "connection refused") {
		t.Errorf("expected log to include the error message 'connection refused', got:\n%s", out)
	}
}

// TestCollectContainerStats_LogsInfoOnSuccess verifies the positive-path
// observability: on each successful cycle, an INFO log line with the
// container count confirms the loop is still firing. Without this an
// operator cannot distinguish "loop running but zero containers" from
// "loop dead".
func TestCollectContainerStats_LogsInfoOnSuccess(t *testing.T) {
	s, _, buf := newObservabilityScheduler(t)

	s.SetDockerStatsFn(func() (*internal.DockerInfo, error) {
		return &internal.DockerInfo{
			Available: true,
			Containers: []internal.ContainerInfo{
				{ID: "abc123", Name: "test-one"},
				{ID: "def456", Name: "test-two"},
				{ID: "ghi789", Name: "test-three"},
			},
		}, nil
	})

	s.collectContainerStats()

	out := buf.String()
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("expected INFO-level log on successful cycle, got:\n%s", out)
	}
	if !strings.Contains(out, "container stats") {
		t.Errorf("expected log line mentioning 'container stats', got:\n%s", out)
	}
	// Container count must be present so operators can spot a sudden
	// drop from "27 containers" to "0 containers" at a glance.
	if !strings.Contains(out, "containers=3") {
		t.Errorf("expected log to include containers=3, got:\n%s", out)
	}
}

// TestCollectContainerStats_LogsWarnOnDockerUnavailable verifies we
// distinguish "docker daemon not reachable" from both the error case
// (which is a code/permission failure) and the success case. Users on
// non-Docker platforms will hit this branch every 5 minutes, so it
// must be at WARN level (visible) but a concise one-liner.
func TestCollectContainerStats_LogsWarnOnDockerUnavailable(t *testing.T) {
	s, _, buf := newObservabilityScheduler(t)

	s.SetDockerStatsFn(func() (*internal.DockerInfo, error) {
		return &internal.DockerInfo{Available: false}, nil
	})

	s.collectContainerStats()

	out := buf.String()
	if !strings.Contains(out, "container stats") {
		t.Fatalf("expected log line mentioning 'container stats', got:\n%s", out)
	}
	if !strings.Contains(out, "docker unavailable") {
		t.Errorf("expected log to mention 'docker unavailable', got:\n%s", out)
	}
	// Must NOT claim success: info-level "saved" messages would mislead.
	if strings.Contains(out, "containers=") {
		t.Errorf("unavailable branch must not log a container count (would mimic success path), got:\n%s", out)
	}
}

// TestCollectContainerStats_LogsOnNilDocker covers the defensive nil
// branch. Today the production code collapses nil + error + unavailable
// into a single silent return; after the fix, nil must at minimum log
// a warning so the branch isn't invisible if it ever does fire.
func TestCollectContainerStats_LogsOnNilDocker(t *testing.T) {
	s, _, buf := newObservabilityScheduler(t)

	s.SetDockerStatsFn(func() (*internal.DockerInfo, error) {
		return nil, nil
	})

	s.collectContainerStats()

	out := buf.String()
	if !strings.Contains(out, "container stats") {
		t.Fatalf("expected log line mentioning 'container stats' for nil docker, got:\n%s", out)
	}
}
