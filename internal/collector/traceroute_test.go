package collector

import (
	"errors"
	"strings"
	"testing"
)

// Canned output that mimics a real `mtr --report --report-cycles=5 --json --no-dns google.com`
// invocation where the trace completes successfully (destination reached).
const mtrJSONSuccess = `{
  "report": {
    "mtr": {"src":"10.0.0.10","dst":"8.8.8.8","tos":0,"psize":"64","bitpattern":"0x00","tests":"5"},
    "hubs": [
      {"count":1,"host":"10.0.0.1","Loss%":0.0,"Snt":5,"Last":0.5,"Avg":0.6,"Best":0.4,"Wrst":0.9,"StDev":0.2},
      {"count":2,"host":"192.0.2.1","Loss%":0.0,"Snt":5,"Last":5.1,"Avg":5.3,"Best":4.9,"Wrst":5.9,"StDev":0.4},
      {"count":3,"host":"8.8.8.8","Loss%":0.0,"Snt":5,"Last":12.1,"Avg":12.3,"Best":11.9,"Wrst":12.9,"StDev":0.4}
    ]
  }
}`

// Canned output where middle hops are unresponsive but the final hop DID
// respond — mtr frequently produces this and it still counts as
// "destination reached".
const mtrJSONMiddleBlackHolesButReached = `{
  "report": {
    "mtr": {"src":"10.0.0.10","dst":"1.1.1.1","tos":0,"psize":"64","bitpattern":"0x00","tests":"5"},
    "hubs": [
      {"count":1,"host":"10.0.0.1","Loss%":0.0,"Snt":5,"Last":0.5,"Avg":0.6,"Best":0.4,"Wrst":0.9,"StDev":0.2},
      {"count":2,"host":"???","Loss%":100.0,"Snt":5,"Last":0.0,"Avg":0.0,"Best":0.0,"Wrst":0.0,"StDev":0.0},
      {"count":3,"host":"1.1.1.1","Loss%":20.0,"Snt":5,"Last":8.1,"Avg":9.3,"Best":7.9,"Wrst":11.2,"StDev":1.3}
    ]
  }
}`

// Canned output where the final hop never responds — destination unreachable.
const mtrJSONUnreachable = `{
  "report": {
    "mtr": {"src":"10.0.0.10","dst":"192.0.2.99","tos":0,"psize":"64","bitpattern":"0x00","tests":"5"},
    "hubs": [
      {"count":1,"host":"10.0.0.1","Loss%":0.0,"Snt":5,"Last":0.5,"Avg":0.6,"Best":0.4,"Wrst":0.9,"StDev":0.2},
      {"count":2,"host":"???","Loss%":100.0,"Snt":5,"Last":0.0,"Avg":0.0,"Best":0.0,"Wrst":0.0,"StDev":0.0}
    ]
  }
}`

func TestParseMTRReport_Success(t *testing.T) {
	result, err := ParseMTRReport([]byte(mtrJSONSuccess))
	if err != nil {
		t.Fatalf("ParseMTRReport: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Target != "8.8.8.8" {
		t.Errorf("Target: expected 8.8.8.8, got %q", result.Target)
	}
	if len(result.Hops) != 3 {
		t.Fatalf("expected 3 hops, got %d", len(result.Hops))
	}
	final := result.Hops[len(result.Hops)-1]
	if final.Host != "8.8.8.8" {
		t.Errorf("final hop host: expected 8.8.8.8, got %q", final.Host)
	}
	if final.LossPct != 0 {
		t.Errorf("final hop loss: expected 0, got %v", final.LossPct)
	}
	if final.AvgMs != 12.3 {
		t.Errorf("final hop avg: expected 12.3, got %v", final.AvgMs)
	}
	if !result.Reached() {
		t.Error("Reached() should be true for a success trace")
	}
	if result.EndToEndLossPct != 0 {
		t.Errorf("EndToEndLossPct: expected 0, got %v", result.EndToEndLossPct)
	}
}

func TestParseMTRReport_MiddleBlackHolesButReached(t *testing.T) {
	result, err := ParseMTRReport([]byte(mtrJSONMiddleBlackHolesButReached))
	if err != nil {
		t.Fatalf("ParseMTRReport: %v", err)
	}
	if !result.Reached() {
		t.Fatalf("Reached() should be true when only middle hops are black-holed; final hop responded. Final=%+v", result.Hops[len(result.Hops)-1])
	}
	if result.EndToEndLossPct != 20.0 {
		t.Errorf("EndToEndLossPct: expected 20, got %v", result.EndToEndLossPct)
	}
}

func TestParseMTRReport_Unreachable(t *testing.T) {
	result, err := ParseMTRReport([]byte(mtrJSONUnreachable))
	if err != nil {
		t.Fatalf("ParseMTRReport: %v", err)
	}
	if result.Reached() {
		t.Fatalf("Reached() should be false when final hop never responds. Final=%+v", result.Hops[len(result.Hops)-1])
	}
}

func TestParseMTRReport_GarbageJSON(t *testing.T) {
	_, err := ParseMTRReport([]byte("not json"))
	if err == nil {
		t.Fatal("expected error parsing garbage input")
	}
}

func TestParseMTRReport_EmptyHops(t *testing.T) {
	_, err := ParseMTRReport([]byte(`{"report":{"mtr":{"dst":"x"},"hubs":[]}}`))
	if err == nil {
		t.Fatal("expected error on empty hops (no trace data)")
	}
}

// ── RunMTR (process invocation) tests using the mtrExecFn injection seam ──

func TestRunMTR_ParsesJSONOutput(t *testing.T) {
	orig := mtrExecFn
	t.Cleanup(func() { mtrExecFn = orig })

	mtrExecFn = func(target string, cycles int) ([]byte, error) {
		if target != "8.8.8.8" {
			t.Errorf("expected target 8.8.8.8, got %q", target)
		}
		if cycles != 5 {
			t.Errorf("expected 5 cycles, got %d", cycles)
		}
		return []byte(mtrJSONSuccess), nil
	}

	result, err := RunMTR("8.8.8.8", 5)
	if err != nil {
		t.Fatalf("RunMTR: %v", err)
	}
	if result == nil || len(result.Hops) == 0 {
		t.Fatalf("expected populated result, got %+v", result)
	}
	if !result.Reached() {
		t.Error("expected Reached() to be true")
	}
}

func TestRunMTR_HandlesMissingBinary(t *testing.T) {
	orig := mtrExecFn
	t.Cleanup(func() { mtrExecFn = orig })

	mtrExecFn = func(_ string, _ int) ([]byte, error) {
		return nil, errors.New(`exec: "mtr": executable file not found in $PATH`)
	}

	_, err := RunMTR("example.com", 5)
	if err == nil {
		t.Fatal("expected error when mtr binary is missing")
	}
	if !strings.Contains(err.Error(), "mtr") {
		t.Errorf("expected error to mention mtr, got %q", err.Error())
	}
}

func TestRunMTR_EmptyTarget(t *testing.T) {
	_, err := RunMTR("", 5)
	if err == nil {
		t.Fatal("expected error on empty target")
	}
}

func TestRunMTR_RejectsInvalidCycles(t *testing.T) {
	// cycles <= 0 is an invalid programming error; we floor it in the
	// runner to a safe default rather than invoking mtr with garbage.
	orig := mtrExecFn
	t.Cleanup(func() { mtrExecFn = orig })
	called := false
	mtrExecFn = func(_ string, cycles int) ([]byte, error) {
		called = true
		if cycles <= 0 {
			t.Errorf("expected RunMTR to floor invalid cycles, got %d", cycles)
		}
		return []byte(mtrJSONSuccess), nil
	}
	_, err := RunMTR("8.8.8.8", 0)
	if err != nil {
		t.Fatalf("RunMTR with cycles=0: %v", err)
	}
	if !called {
		t.Error("mtrExecFn was never invoked")
	}
}
