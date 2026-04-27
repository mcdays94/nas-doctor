// Package collector — speedtest_go_lib.go is the thin adapter from
// showwin/speedtest-go's API surface onto our speedTestEngine
// interface.
//
// This file is deliberately unit-test-thin: it does real network I/O.
// The runner-level tests in speedtest_runner_test.go drive a fake
// engine instead. Slice 2 (#285) will introduce per-sample callbacks
// here via showwin's task-callback hooks; slice 1 just emits the
// final aggregate and an empty samples channel.
//
// Structured runner-boundary logging — added in v0.9.11-rc3 (issue
// #296) — emits INFO lines at every phase boundary:
//
//	speedtest-go: fetch_user_info_ok user=...
//	speedtest-go: fetch_servers_ok count=N
//	speedtest-go: server_selected name=... id=... distance_km=...
//	speedtest-go: ping_complete latency_ms=X samples_emitted=N
//	speedtest-go: download_complete dl_mbps=X samples_emitted=N
//	speedtest-go: upload_complete ul_mbps=X samples_emitted=N
//	speedtest-go: run_complete dl=X ul=Y latency=Z samples_total=N
//
// On UAT, these lines let the operator distinguish "showwin emitted
// 0 callbacks" (B1 root cause hypothesis #1 from issue #296) from
// "callbacks fired but registry dropped them" (hypothesis #2). Without
// these logs, both modes look identical from the outside (history
// row written, samples table empty).
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
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
	logger := slog.Default()
	startedAt := time.Now()
	logger.Info("speedtest-go: run_start")

	client := speedtest.New()
	user, err := client.FetchUserInfo()
	if err != nil {
		logger.Warn("speedtest-go: fetch_user_info_failed", "error", err)
		return nil, nil, fmt.Errorf("fetch user info: %w", err)
	}
	if user != nil {
		logger.Info("speedtest-go: fetch_user_info_ok",
			"isp", user.Isp,
			"ip", user.IP,
		)
	}
	servers, err := client.FetchServers()
	if err != nil {
		logger.Warn("speedtest-go: fetch_servers_failed", "error", err)
		return nil, nil, fmt.Errorf("fetch servers: %w", err)
	}
	logger.Info("speedtest-go: fetch_servers_ok", "count", len(servers))
	targets, err := servers.FindServer([]int{})
	if err != nil {
		logger.Warn("speedtest-go: find_server_failed", "error", err)
		return nil, nil, fmt.Errorf("find server: %w", err)
	}
	if len(targets) == 0 {
		logger.Warn("speedtest-go: no_servers_available")
		return nil, nil, errors.New("no speedtest servers available")
	}
	srv := targets[0]
	logger.Info("speedtest-go: server_selected",
		"name", srv.Name,
		"id", srv.ID,
		"sponsor", srv.Sponsor,
		"distance_km", srv.Distance,
	)

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

	// Per-phase emitted-sample counters. Atomic since callbacks fire
	// from showwin's internal goroutines. Issue #296 B1 — these counts
	// are the smoking gun for "did showwin actually emit anything"
	// vs "registry drained an empty buffer".
	var (
		latencyEmitted  atomic.Int64
		downloadEmitted atomic.Int64
		uploadEmitted   atomic.Int64
		droppedEmitted  atomic.Int64
	)
	emit := func(s SpeedTestSample) {
		select {
		case samples <- s:
			switch s.Phase {
			case SpeedTestPhaseLatency:
				latencyEmitted.Add(1)
			case SpeedTestPhaseDownload:
				downloadEmitted.Add(1)
			case SpeedTestPhaseUpload:
				uploadEmitted.Add(1)
			}
		default:
			// Drop — registry's slow-client policy applies at
			// the broadcast layer; here we just don't block
			// the showwin internal goroutine.
			droppedEmitted.Add(1)
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

	pingStart := time.Now()
	if err := srv.PingTestContext(ctx, pingCallback); err != nil {
		close(samples)
		logger.Warn("speedtest-go: ping_failed", "error", err,
			"samples_emitted", latencyEmitted.Load(),
			"elapsed_ms", time.Since(pingStart).Milliseconds())
		return nil, nil, fmt.Errorf("ping: %w", err)
	}
	logger.Info("speedtest-go: ping_complete",
		"latency_ms", float64(srv.Latency)/float64(time.Millisecond),
		"jitter_ms", float64(srv.Jitter)/float64(time.Millisecond),
		"samples_emitted", latencyEmitted.Load(),
		"elapsed_ms", time.Since(pingStart).Milliseconds(),
	)

	dlStart := time.Now()
	if err := srv.DownloadTestContext(ctx); err != nil {
		close(samples)
		logger.Warn("speedtest-go: download_failed", "error", err,
			"samples_emitted", downloadEmitted.Load(),
			"elapsed_ms", time.Since(dlStart).Milliseconds())
		return nil, nil, fmt.Errorf("download: %w", err)
	}
	logger.Info("speedtest-go: download_complete",
		"dl_mbps", srv.DLSpeed.Mbps(),
		"samples_emitted", downloadEmitted.Load(),
		"elapsed_ms", time.Since(dlStart).Milliseconds(),
	)

	ulStart := time.Now()
	if err := srv.UploadTestContext(ctx); err != nil {
		close(samples)
		logger.Warn("speedtest-go: upload_failed", "error", err,
			"samples_emitted", uploadEmitted.Load(),
			"elapsed_ms", time.Since(ulStart).Milliseconds())
		return nil, nil, fmt.Errorf("upload: %w", err)
	}
	logger.Info("speedtest-go: upload_complete",
		"ul_mbps", srv.ULSpeed.Mbps(),
		"samples_emitted", uploadEmitted.Load(),
		"elapsed_ms", time.Since(ulStart).Milliseconds(),
	)

	close(samples)

	dlMbps := srv.DLSpeed.Mbps()
	ulMbps := srv.ULSpeed.Mbps()
	totalSamples := latencyEmitted.Load() + downloadEmitted.Load() + uploadEmitted.Load()
	logger.Info("speedtest-go: run_complete",
		"dl_mbps", dlMbps,
		"ul_mbps", ulMbps,
		"latency_ms", float64(srv.Latency)/float64(time.Millisecond),
		"samples_latency", latencyEmitted.Load(),
		"samples_download", downloadEmitted.Load(),
		"samples_upload", uploadEmitted.Load(),
		"samples_total", totalSamples,
		"samples_dropped", droppedEmitted.Load(),
		"total_elapsed_ms", time.Since(startedAt).Milliseconds(),
	)
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
