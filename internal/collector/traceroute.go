package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// MTRHop is a single hop reported by `mtr --report --json`.
//
// NOTE on the `Loss%` field: mtr genuinely emits the key with a literal
// percent sign in the JSON (e.g. `"Loss%":0.0`). Go's encoding/json
// looks up struct tags verbatim, so the tag below must also be `Loss%`.
// This is not a typo; removing the percent sign would silently leave
// the field at zero on every parse.
type MTRHop struct {
	Count   int     `json:"count"`
	Host    string  `json:"host"`
	LossPct float64 `json:"Loss%"`
	Sent    int     `json:"Snt"`
	Last    float64 `json:"Last"`
	AvgMs   float64 `json:"Avg"`
	Best    float64 `json:"Best"`
	Worst   float64 `json:"Wrst"`
	StDev   float64 `json:"StDev"`
}

// MTRResult is the parsed output of a traceroute run.
type MTRResult struct {
	Source          string   `json:"source"`
	Target          string   `json:"target"`
	Hops            []MTRHop `json:"hops"`
	EndToEndLossPct float64  `json:"end_to_end_loss_pct"`
	FinalRTTMs      float64  `json:"final_rtt_ms"`
}

// Reached reports whether the final hop appears to have responded.
// mtr marks unresponsive hops with host="???" and 100% loss; the
// check is "up" when the final hop is not in that shape.
//
// Middle hops are frequently black-holed even on healthy networks
// (routers configured to drop ICMP TTL-exceeded), so we only consider
// the final hop when deciding reachability.
func (r *MTRResult) Reached() bool {
	if r == nil || len(r.Hops) == 0 {
		return false
	}
	final := r.Hops[len(r.Hops)-1]
	return !isBlackHoleHop(final)
}

// isBlackHoleHop reports whether the hop represents no response.
// mtr uses "???" as a sentinel hostname for unresponsive hops. A hop
// with host="???" AND Loss%==100 is definitively a black hole; in
// practice mtr always pairs them, so checking either is sufficient.
func isBlackHoleHop(h MTRHop) bool {
	host := strings.TrimSpace(h.Host)
	if host == "" || host == "???" {
		return true
	}
	return false
}

// mtrRawReport is the shape of `mtr --report --json` stdout.
type mtrRawReport struct {
	Report struct {
		MTR struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		} `json:"mtr"`
		Hubs []MTRHop `json:"hubs"`
	} `json:"report"`
}

// ParseMTRReport converts raw `mtr --report --json` stdout into an MTRResult.
// Returns an error when the payload is not valid JSON or contains no hops.
func ParseMTRReport(raw []byte) (*MTRResult, error) {
	var parsed mtrRawReport
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse mtr json: %w", err)
	}
	if len(parsed.Report.Hubs) == 0 {
		return nil, fmt.Errorf("mtr report contains no hops")
	}

	result := &MTRResult{
		Source: parsed.Report.MTR.Src,
		Target: parsed.Report.MTR.Dst,
		Hops:   parsed.Report.Hubs,
	}
	final := result.Hops[len(result.Hops)-1]
	if !isBlackHoleHop(final) {
		result.FinalRTTMs = final.AvgMs
		result.EndToEndLossPct = final.LossPct
	}
	return result, nil
}

// mtrExecFn is the seam tests use to inject canned mtr output without
// invoking a real subprocess. Default implementation shells out to
// `mtr --report --report-cycles=<cycles> --json --no-dns <target>`
// with a hard 90s timeout (covers 10-cycle runs on slow networks).
var mtrExecFn = func(target string, cycles int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := []string{
		"--report",
		fmt.Sprintf("--report-cycles=%d", cycles),
		"--json",
		"--no-dns",
		target,
	}
	cmd := exec.CommandContext(ctx, "mtr", args...)
	return cmd.Output()
}

// RunMTR invokes mtr against the given target and returns the parsed
// report. cycles is the number of probes per hop (--report-cycles).
// Scheduled runs typically use 5; the Test button uses 10 for a richer
// sample.
//
// Returns a descriptive error when mtr is not installed, when the
// process fails, or when the output cannot be parsed.
func RunMTR(target string, cycles int) (*MTRResult, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("traceroute target is required")
	}
	if cycles <= 0 {
		cycles = 5
	}

	out, err := mtrExecFn(target, cycles)
	if err != nil {
		return nil, fmt.Errorf("mtr execution failed: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("mtr produced no output (binary may be missing or failed silently)")
	}
	return ParseMTRReport(out)
}
