package scheduler

import (
	"fmt"
	"runtime/debug"
)

// runWithRecover wraps a single tick's work in a panic recovery so a
// panic in one cycle does not kill the goroutine and silently stop
// collection (issue #325, surfaced by #323).
//
// Background: prior to this fix, every goroutine started by
// Scheduler.Start had the pattern
//
//	go func() {
//	    for {
//	        select {
//	        case <-ticker.C:
//	            s.RunOnce() // ← if this panics, the goroutine dies
//	        case <-s.stop:
//	            return
//	        }
//	    }
//	}()
//
// with no recover() anywhere. A panic in any sub-collector (a stale
// row in smart_history referencing a vanished device path, an unexpected
// JSON shape in a settings field, etc.) would kill the loop. The HTTP
// server stayed up, the dashboard rendered off the last cached
// snapshot, but new collection silently stopped. From the user's
// perspective the only signal was "trend chart flat-lines on date X" —
// hence #162 and #323.
//
// After this fix, every per-tick invocation is wrapped:
//
//	case <-ticker.C:
//	    s.runWithRecover("main-scan", s.RunOnce)
//
// A panic is logged at Error level with the panic value, the loop name,
// and the full stack trace. The wrapper returns normally so the outer
// select loop continues on the next tick. The underlying bug isn't
// fixed, but the failure mode is now visible-and-self-recovering
// instead of invisible-and-permanent.
//
// The helper is defensive against a nil logger so it's safe to call
// from any code path, including future ones that might construct a
// bare Scheduler in a test harness.
func (s *Scheduler) runWithRecover(loop string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			if s.logger != nil {
				s.logger.Error("scheduler goroutine recovered from panic",
					"loop", loop,
					"panic", fmt.Sprint(r),
					"stack", string(debug.Stack()),
				)
			}
		}
	}()
	fn()
}
