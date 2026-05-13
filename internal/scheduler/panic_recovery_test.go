package scheduler

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// newTestSchedulerForPanicRecovery builds a minimal *Scheduler that's
// only valid for exercising runWithRecover. The helper only reads
// s.logger, so we leave the other fields zero.
func newTestSchedulerForPanicRecovery(buf *bytes.Buffer) *Scheduler {
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &Scheduler{logger: logger}
}

// TestScheduler_RunWithRecover_RecoversFromPanic verifies the panic-recovery
// helper introduced for #325: a panic inside the wrapped function must not
// propagate out, so the caller's outer goroutine loop continues across
// panics instead of silently dying.
//
// Surfaced by #323 / #325 audit, and originally hypothesised in #162's
// first comment back in April: prior to this fix, a panic in any of the
// scheduler goroutines killed it permanently because none of them had
// recover() coverage. The HTTP server stayed up and the dashboard
// rendered off the last cached snapshot, but new collection silently
// stopped. The fix here converts that failure mode from invisible-and-
// permanent to visible-and-self-recovering.
func TestScheduler_RunWithRecover_RecoversFromPanic(t *testing.T) {
	var buf bytes.Buffer
	s := newTestSchedulerForPanicRecovery(&buf)

	didPanic := func() (panicked bool) {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		s.runWithRecover("test-loop", func() {
			panic("deliberate test panic")
		})
		return false
	}()
	if didPanic {
		t.Fatal("runWithRecover let the panic propagate out; the goroutine would have died")
	}

	logged := buf.String()
	if !strings.Contains(logged, "deliberate test panic") {
		t.Errorf("log output missing panic value:\n%s", logged)
	}
	if !strings.Contains(logged, "test-loop") {
		t.Errorf("log output missing loop name (so operators can identify which scheduler subsystem panicked):\n%s", logged)
	}
	if !strings.Contains(logged, "level=ERROR") {
		t.Errorf("expected ERROR-level log entry; panic recovery is severe enough to warrant ERROR not WARN:\n%s", logged)
	}
	if !strings.Contains(logged, "stack") {
		t.Errorf("log output missing stack-trace key; without it operators can't tell which call site panicked:\n%s", logged)
	}
}

// TestScheduler_RunWithRecover_PassthroughOnNormalReturn verifies that
// the wrapper is transparent when the wrapped function completes
// normally (no panic). No log entry, no side effects.
func TestScheduler_RunWithRecover_PassthroughOnNormalReturn(t *testing.T) {
	var buf bytes.Buffer
	s := newTestSchedulerForPanicRecovery(&buf)

	called := false
	s.runWithRecover("test-loop", func() {
		called = true
	})

	if !called {
		t.Fatal("runWithRecover did not invoke the wrapped function on the normal path")
	}
	if buf.Len() > 0 {
		t.Errorf("runWithRecover wrote to the logger on the normal (no-panic) path:\n%s", buf.String())
	}
}

// TestScheduler_RunWithRecover_SurvivesMultiplePanics verifies that the
// wrapper can be invoked repeatedly across multiple panic events, which
// is the actual property the scheduler relies on (one bad tick must not
// poison subsequent ticks).
func TestScheduler_RunWithRecover_SurvivesMultiplePanics(t *testing.T) {
	var buf bytes.Buffer
	s := newTestSchedulerForPanicRecovery(&buf)

	for i := 0; i < 5; i++ {
		s.runWithRecover("test-loop", func() {
			panic("tick panic")
		})
	}

	// All 5 panics should be logged as separate ERROR entries.
	logged := buf.String()
	count := strings.Count(logged, "scheduler goroutine recovered from panic")
	if count != 5 {
		t.Errorf("expected 5 separate panic-recovery log entries, got %d:\n%s", count, logged)
	}
}

// TestScheduler_RunWithRecover_HandlesNilLogger ensures the helper
// does not itself panic if Scheduler.logger is nil. Production always
// wires a logger via New(), but defensive code should not assume it.
func TestScheduler_RunWithRecover_HandlesNilLogger(t *testing.T) {
	s := &Scheduler{} // logger left nil intentionally

	didPanic := func() (panicked bool) {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		s.runWithRecover("test-loop", func() {
			panic("with nil logger")
		})
		return false
	}()
	if didPanic {
		t.Fatal("runWithRecover with nil logger should silently swallow the panic, not propagate it")
	}
}
