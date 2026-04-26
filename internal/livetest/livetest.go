// livetest.go — the LiveTest broadcast object. Owned by Manager; one
// instance per in-flight test. Subscribers attach via Subscribe()
// and receive the full sample replay (every sample emitted so far
// before the call) followed by live samples streamed as they arrive.
//
// Concurrency model:
//
//   - Single emitter goroutine (Manager.driveTest) calls emit() with
//     each sample. emit holds the per-test mutex briefly to append
//     to the buffer + push to all subscriber channels.
//
//   - Subscribers call Subscribe, which holds the same mutex while
//     replaying buffered samples into the new channel and then
//     adding the channel to the subscriber set. The mutex hold
//     ordering ensures a subscribe-during-emit cannot drop or
//     duplicate a sample.
//
//   - Slow consumers: emit uses non-blocking send (select default).
//     If a subscriber's buffer is full, that subscriber is dropped
//     (channel removed from set + closed). Other subscribers
//     receive the sample normally.
//
//   - Test completion: finishWithResult / finishWithError set the
//     terminal state, close every subscriber channel, then close
//     the Done channel. After this, Subscribe still works — late
//     subscribers see the full replay + already-closed channel,
//     which is the desired behaviour for SSE clients reconnecting
//     to a just-completed test.
package livetest

import (
	"sync"
	"time"
)

// LiveTest is the broadcast object handed back from Registry.StartTest.
// Each test has its own LiveTest; the Manager holds at most one in
// active state at a time.
type LiveTest struct {
	id        int64
	startedAt time.Time

	mu          sync.Mutex
	samples     []Sample            // replay buffer; append-only during run
	subscribers map[chan Sample]struct{}
	closed      bool                // true after finish*; subscribers should not be added to broadcast set
	result      *Result             // set on successful completion
	err         error               // set on failed completion
	complete    chan struct{}       // closed when the runner returns; Done is closed simultaneously
	done        chan struct{}       // closed when test ends + all subscriber channels closed
}

// ID returns the test's stable identifier. Used by SSE handler to
// route /stream/{id} requests.
func (t *LiveTest) ID() int64 { return t.id }

// StartedAt is when StartTest was called (UTC monotonic clock).
func (t *LiveTest) StartedAt() time.Time { return t.startedAt }

// Subscribe returns a channel of samples. The new subscriber receives:
//
//  1. Every sample emitted so far (replay-on-subscribe), in order.
//  2. Every subsequently emitted sample, in order, until completion.
//  3. The channel is closed when the test completes (success or
//     error). A late subscriber attaching AFTER completion still
//     gets the full replay, then a closed channel — they should
//     check OnComplete / Result / Err for the terminal state.
//
// If the subscriber's buffer fills (slow consumer), the subscriber is
// dropped from the broadcast set and the channel closed. Callers that
// need to differentiate "test ended" from "subscriber dropped" can
// poll Done / OnComplete.
func (t *LiveTest) Subscribe() <-chan Sample {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Size the channel so the entire replay buffer fits, with
	// headroom for live samples that arrive while the SSE flusher
	// is mid-write. Replay must be lossless — a partial replay
	// would mislead the dashboard chart into thinking samples
	// were missing.
	bufSize := len(t.samples) + SubscribeBufferSize
	ch := make(chan Sample, bufSize)
	for _, s := range t.samples {
		ch <- s
	}
	if t.closed {
		// Test already ended: deliver replay then close.
		close(ch)
		return ch
	}
	t.subscribers[ch] = struct{}{}
	return ch
}

// emit appends a sample to the replay buffer and fans it out to all
// current subscribers. Slow subscribers (full channels) are dropped
// from the set + their channel is closed. The emitter thread is
// never blocked.
func (t *LiveTest) emit(s Sample) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		// Defensive: ignore samples arriving after finish*. This
		// shouldn't happen if the runner contract is honoured
		// (samples channel closes before Run returns) but a
		// misbehaving runner shouldn't corrupt registry state.
		return
	}
	t.samples = append(t.samples, s)
	for ch := range t.subscribers {
		select {
		case ch <- s:
			// delivered
		default:
			// slow client — drop from set + close channel.
			// Doing this inside the loop is safe: Go map
			// iteration tolerates concurrent delete of the
			// current key.
			delete(t.subscribers, ch)
			close(ch)
		}
	}
}

// finishWithResult transitions the test to its terminal state on
// success: stamps the result, closes every remaining subscriber
// channel, and closes the Done channel. Idempotent — a second call
// is a no-op (defensive against runner contract violations).
func (t *LiveTest) finishWithResult(res *Result) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.result = res
	t.closed = true
	for ch := range t.subscribers {
		close(ch)
		delete(t.subscribers, ch)
	}
	close(t.complete)
	close(t.done)
}

// finishWithError transitions the test to its terminal state on
// failure. Same shape as finishWithResult except err is set instead
// of result.
func (t *LiveTest) finishWithError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.err = err
	t.closed = true
	for ch := range t.subscribers {
		close(ch)
		delete(t.subscribers, ch)
	}
	close(t.complete)
	close(t.done)
}

// Done returns a channel that closes when the test has fully ended
// AND all subscriber channels are closed. Useful for "wait until
// the test is fully wrapped up before checking final result".
func (t *LiveTest) Done() <-chan struct{} { return t.done }

// OnComplete returns a channel that closes when the runner has
// returned (success or error). Identical to Done in this v1 — both
// close in the same finish call — but kept as a separate name for
// API stability if we later want to distinguish "runner returned"
// from "all subscribers drained".
func (t *LiveTest) OnComplete() <-chan struct{} { return t.complete }

// Result returns the terminal result if the test completed
// successfully. Nil before completion or on error.
func (t *LiveTest) Result() *Result {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.result
}

// Err returns the terminal error if the test failed. Nil before
// completion or on success.
func (t *LiveTest) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

// Engine returns the engine name from the result, or empty string
// if no result is available yet. Used for the SSE start event's
// engine field — emitted before the runner completes, so this is
// best-effort. SSE handler emits start events with engine="" in
// the pre-result window; subscribers can rely on the result event
// for the authoritative engine value.
func (t *LiveTest) Engine() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.result == nil {
		return ""
	}
	return t.result.Engine
}

// SnapshotSamples returns a copy of the in-memory replay buffer at
// call time. Used by tests for state assertions. Production callers
// should always go through Subscribe.
func (t *LiveTest) SnapshotSamples() []Sample {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Sample, len(t.samples))
	copy(out, t.samples)
	return out
}
