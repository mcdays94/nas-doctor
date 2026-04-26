// Package collector — speedtest_go_lib.go is the thin adapter from
// showwin/speedtest-go's API surface onto our speedTestEngine
// interface.
//
// This file is deliberately unit-test-thin: it does real network I/O.
// The runner-level tests in speedtest_runner_test.go drive a fake
// engine instead. Slice 2 (#285) will introduce per-sample callbacks
// here via showwin's task-callback hooks; slice 1 just emits the
// final aggregate and an empty samples channel.
package collector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
	"github.com/showwin/speedtest-go/speedtest"
)

// runSpeedtestGoLibrary fetches the closest server, runs the three
// phases sequentially, and returns the composed SpeedTestResult.
//
// Slice 2 (#285) wires showwin's per-sample callbacks
// (SetCallbackDownload, SetCallbackUpload, PingTestContext callback)
// to emit SpeedTestSample values into the returned samples channel.
// The channel is buffered generously and closed by the producer
// goroutine when all three phases complete — subscribers must drain
// it to avoid leaking the goroutine.
//
// Errors propagate verbatim (the runner layer wraps them). Defense-
// in-depth zero-throughput guard mirrors the legacy Ookla-CLI path —
// returning a result with download==upload==0 would corrupt the
// dashboard's "Latest" widget.
func runSpeedtestGoLibrary(ctx context.Context) (*internal.SpeedTestResult, <-chan SpeedTestSample, error) {
	client := speedtest.New()
	if _, err := client.FetchUserInfo(); err != nil {
		return nil, nil, fmt.Errorf("fetch user info: %w", err)
	}
	servers, err := client.FetchServers()
	if err != nil {
		return nil, nil, fmt.Errorf("fetch servers: %w", err)
	}
	targets, err := servers.FindServer([]int{})
	if err != nil {
		return nil, nil, fmt.Errorf("find server: %w", err)
	}
	if len(targets) == 0 {
		return nil, nil, errors.New("no speedtest servers available")
	}
	srv := targets[0]

	// Bound the entire test by the outer context so a stuck phase
	// can't hang the scheduler. 120s mirrors the legacy Ookla CLI
	// timeout in speedtest.go.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
	}

	// Buffered samples channel. Showwin's callbacks fire from the
	// upload/download data goroutines — we don't want them to
	// block. Sized to comfortably hold a 60-second test's worth of
	// per-second samples plus headroom; if the registry's broadcast
	// fan-out lags, samples back up here briefly and then catch up.
	samples := make(chan SpeedTestSample, 256)
	emit := func(s SpeedTestSample) {
		select {
		case samples <- s:
		default:
			// Drop — registry's slow-client policy applies at
			// the broadcast layer; here we just don't block
			// the showwin internal goroutine.
		}
	}

	client.SetCallbackDownload(func(rate speedtest.ByteRate) {
		emit(SpeedTestSample{
			Phase: SpeedTestPhaseDownload,
			At:    time.Now(),
			Mbps:  float64(rate.Mbps()),
		})
	})
	client.SetCallbackUpload(func(rate speedtest.ByteRate) {
		emit(SpeedTestSample{
			Phase: SpeedTestPhaseUpload,
			At:    time.Now(),
			Mbps:  float64(rate.Mbps()),
		})
	})

	pingCallback := func(latency time.Duration) {
		emit(SpeedTestSample{
			Phase:     SpeedTestPhaseLatency,
			At:        time.Now(),
			LatencyMs: float64(latency) / float64(time.Millisecond),
		})
	}

	if err := srv.PingTestContext(ctx, pingCallback); err != nil {
		close(samples)
		return nil, nil, fmt.Errorf("ping: %w", err)
	}
	if err := srv.DownloadTestContext(ctx); err != nil {
		close(samples)
		return nil, nil, fmt.Errorf("download: %w", err)
	}
	if err := srv.UploadTestContext(ctx); err != nil {
		close(samples)
		return nil, nil, fmt.Errorf("upload: %w", err)
	}
	close(samples)

	dlMbps := srv.DLSpeed.Mbps()
	ulMbps := srv.ULSpeed.Mbps()
	if dlMbps == 0 && ulMbps == 0 {
		return nil, nil, errors.New("speedtest-go returned zero throughput on both phases")
	}

	// Parse server ID. showwin stores it as a string; our model uses
	// int. Best-effort — we don't fail the test if this can't parse.
	serverID, _ := strconv.Atoi(srv.ID)

	res := &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: dlMbps,
		UploadMbps:   ulMbps,
		LatencyMs:    float64(srv.Latency) / float64(time.Millisecond),
		JitterMs:     float64(srv.Jitter) / float64(time.Millisecond),
		ServerName:   srv.Name,
		ServerID:     serverID,
		ISP:          firstNonEmpty(client.User.Isp, srv.Sponsor),
		ExternalIP:   client.User.IP,
	}
	return res, samples, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
