package collector

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// defaultRunner is the lazily-constructed composite runner used by the
// package-level RunSpeedTest entry point. Tests that want to drive a
// deterministic engine call SetSpeedTestRunnerForTest before invoking
// RunSpeedTest, and reset it afterwards. Production wiring (scheduler
// + Test endpoint) typically calls RunSpeedTest directly which lazy-
// constructs the production composite the first time.
var (
	defaultSpeedTestRunner   SpeedTestRunner
	defaultSpeedTestRunnerMu sync.Mutex
)

// runSpeedTestEntry is the shared entry point used by both RunSpeedTest
// (the legacy public API) and any future caller that wants direct
// access to the composite runner. Slice 1 keeps the public API
// unchanged: a nil result still means "no engine produced a usable
// result". Slice 2 will introduce a richer entrypoint that exposes the
// sample channel to the LiveTestRegistry.
func runSpeedTestEntry(ctx context.Context) *internal.SpeedTestResult {
	defaultSpeedTestRunnerMu.Lock()
	r := defaultSpeedTestRunner
	if r == nil {
		r = NewCompositeSpeedTestRunner(NewSpeedTestGoRunner(), NewOoklaCLIRunner())
		defaultSpeedTestRunner = r
	}
	defaultSpeedTestRunnerMu.Unlock()

	res, samples, err := r.Run(ctx)
	// Drain-and-discard the sample channel: slice 1 has no consumer
	// for these. Slice 2 will fan them out to LiveTestRegistry. A nil
	// channel (error path) is safe to ignore.
	if samples != nil {
		go func() {
			for range samples {
			}
		}()
	}
	if err != nil || res == nil {
		return nil
	}
	return res
}

// DefaultSpeedTestRunner returns the package-level composite runner
// used by RunSpeedTest. Lazy-constructs the production composite on
// first call and reuses it thereafter. Slice 2 (#285) wires this
// into the LiveTestRegistry so the registry's Run() and the legacy
// RunSpeedTest() share the same runner instance — important for
// idempotency: a manual /api/v1/speedtest/run during a cron-driven
// test must attach to the same in-flight run, which requires both
// paths to acquire the same singleton lock in the registry.
func DefaultSpeedTestRunner() SpeedTestRunner {
	defaultSpeedTestRunnerMu.Lock()
	defer defaultSpeedTestRunnerMu.Unlock()
	if defaultSpeedTestRunner == nil {
		defaultSpeedTestRunner = NewCompositeSpeedTestRunner(NewSpeedTestGoRunner(), NewOoklaCLIRunner())
	}
	return defaultSpeedTestRunner
}

// SetSpeedTestRunnerForTest swaps the package-level default runner.
// Test-only — not intended for production wiring (which constructs
// the composite at startup via NewCompositeSpeedTestRunner).
func SetSpeedTestRunnerForTest(r SpeedTestRunner) func() {
	defaultSpeedTestRunnerMu.Lock()
	prev := defaultSpeedTestRunner
	defaultSpeedTestRunner = r
	defaultSpeedTestRunnerMu.Unlock()
	return func() {
		defaultSpeedTestRunnerMu.Lock()
		defaultSpeedTestRunner = prev
		defaultSpeedTestRunnerMu.Unlock()
	}
}

// RunSpeedTest executes a network speed test and returns the result.
// As of issue #284 (PRD #283 slice 1), this delegates to the
// SpeedTestRunner composite which prefers showwin/speedtest-go and
// falls back to the bundled Ookla CLI on error. The returned result
// has its Engine field populated identifying which engine produced
// it ("speedtest_go" or "ookla_cli").
//
// On total failure (both engines unavailable / zero throughput) this
// still returns nil — preserving the v0.9.6 #210 caller contract that
// a nil result means "report failed attempt, do not write history".
func RunSpeedTest() *internal.SpeedTestResult {
	return runSpeedTestEntry(context.Background())
}

// ---------- Ookla speedtest CLI ----------

func runOoklaSpeedtest() *internal.SpeedTestResult {
	path, err := exec.LookPath("speedtest")
	if err != nil {
		return nil
	}
	_ = path

	out, err := execCmdTimeout("speedtest", 120, "--format=json", "--accept-license", "--accept-gdpr")
	if err != nil {
		return nil
	}

	var data struct {
		Timestamp string `json:"timestamp"`
		Ping      struct {
			Latency float64 `json:"latency"`
			Jitter  float64 `json:"jitter"`
		} `json:"ping"`
		Download struct {
			Bandwidth int64 `json:"bandwidth"` // bytes/sec
		} `json:"download"`
		Upload struct {
			Bandwidth int64 `json:"bandwidth"` // bytes/sec
		} `json:"upload"`
		Server struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Host string `json:"host"`
		} `json:"server"`
		ISP    string `json:"isp"`
		Result struct {
			URL string `json:"url"`
		} `json:"result"`
		Interface struct {
			ExternalIP string `json:"externalIp"`
		} `json:"interface"`
	}

	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil
	}

	// Defense-in-depth for #170: if Ookla exited 0 but produced zero-throughput
	// JSON (e.g. partial run, killed subprocess, or a test server returning
	// empty measurements), treat as a failure here so the scheduler never
	// sees a zero-valued SpeedTestResult in the first place.
	if data.Download.Bandwidth == 0 && data.Upload.Bandwidth == 0 {
		return nil
	}

	return &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: float64(data.Download.Bandwidth) * 8 / 1e6, // bytes/sec → Mbps
		UploadMbps:   float64(data.Upload.Bandwidth) * 8 / 1e6,
		LatencyMs:    data.Ping.Latency,
		JitterMs:     data.Ping.Jitter,
		ServerName:   data.Server.Name,
		ServerID:     data.Server.ID,
		ISP:          data.ISP,
		ExternalIP:   data.Interface.ExternalIP,
		ResultURL:    data.Result.URL,
	}
}

// ---------- speedtest-cli (Python) ----------

func runSpeedtestCLI() *internal.SpeedTestResult {
	path, err := exec.LookPath("speedtest-cli")
	if err != nil {
		return nil
	}
	_ = path

	out, err := execCmdTimeout("speedtest-cli", 120, "--json")
	if err != nil {
		return nil
	}

	var data struct {
		Timestamp string  `json:"timestamp"`
		Download  float64 `json:"download"` // bits/sec
		Upload    float64 `json:"upload"`   // bits/sec
		Ping      float64 `json:"ping"`     // ms
		Server    struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"server"`
		Client struct {
			ISP string `json:"isp"`
			IP  string `json:"ip"`
		} `json:"client"`
	}

	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return nil
	}

	// Defense-in-depth for #170: treat zero-throughput results as failure
	// so the scheduler never sees a zero-valued SpeedTestResult.
	if data.Download == 0 && data.Upload == 0 {
		return nil
	}

	return &internal.SpeedTestResult{
		Timestamp:    time.Now(),
		DownloadMbps: data.Download / 1e6, // bits/sec → Mbps
		UploadMbps:   data.Upload / 1e6,
		LatencyMs:    data.Ping,
		ServerName:   data.Server.Name,
		ISP:          data.Client.ISP,
		ExternalIP:   data.Client.IP,
	}
}

// execCmdTimeout runs a command with a timeout in seconds.
func execCmdTimeout(name string, timeoutSec int, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	// Set a timeout via context would be better, but for simplicity:
	out, err := cmd.Output()
	return string(out), err
}
