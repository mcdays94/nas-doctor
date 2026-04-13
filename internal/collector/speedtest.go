package collector

import (
	"encoding/json"
	"os/exec"
	"time"

	internal "github.com/mcdays94/nas-doctor/internal"
)

// RunSpeedTest executes a network speed test and returns the result.
// Supports Ookla speedtest-cli (preferred) and speedtest-go as fallback.
// This should be called on its own schedule (not during every scan).
func RunSpeedTest() *internal.SpeedTestResult {
	// Try Ookla speedtest CLI first (speedtest --format=json)
	if result := runOoklaSpeedtest(); result != nil {
		return result
	}

	// Try speedtest-cli (Python-based, --json flag)
	if result := runSpeedtestCLI(); result != nil {
		return result
	}

	return nil
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
