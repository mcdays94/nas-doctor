package api

import (
	"strings"
	"testing"
)

// TestChartJS_DrawAxes_UsesMeasureText guards against regression of the fix
// for issue #165 (x-axis label overlap on /disk/<serial>). drawAxes() must
// size its label stride based on the actual rendered text width rather than
// a hardcoded pixel budget, otherwise datetime labels like "4/17 23:11"
// (65-75px wide in the default 11px sans-serif) collide into an unreadable
// wall once history density grows.
//
// If this test fails, it means someone reverted drawAxes() back to the old
// `Math.floor(labels.length / (cw / 50))` logic or removed measureText()
// entirely — the charts on /disk/<serial>, /stats, /parity and /service_checks
// will start overlapping again.
func TestChartJS_DrawAxes_UsesMeasureText(t *testing.T) {
	js := ChartJS

	// Locate the x-labels block inside drawAxes. We assert against a slice
	// of the source rather than the whole blob so that the test pinpoints
	// the regression instead of matching a measureText call elsewhere.
	const marker = "/* x labels */"
	idx := strings.Index(js, marker)
	if idx < 0 {
		t.Fatalf("ChartJS: could not locate %q marker — drawAxes() structure changed unexpectedly", marker)
	}
	// Slice a generous window (next 1200 chars should comfortably cover
	// the x-labels block even after refactors).
	end := idx + 1200
	if end > len(js) {
		end = len(js)
	}
	xBlock := js[idx:end]

	checks := []struct {
		name   string
		substr string
		why    string
	}{
		{
			"measureText call for actual label width",
			"measureText",
			"drawAxes() must measure real label widths via ctx.measureText() — hardcoded pixel budgets cause overlap once labels exceed ~50px",
		},
		{
			"tracks the widest label",
			"maxLabelWidth",
			"drawAxes() must derive stride from the widest measured label so the worst-case label never overlaps its neighbour",
		},
		{
			"hardcoded-50 stride formula is gone",
			"cw/50",
			"the old `labels.length/(cw/50)` stride formula must not come back — it was the root cause of issue #165",
		},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "hardcoded-50 stride formula is gone" {
				if strings.Contains(xBlock, tc.substr) {
					t.Errorf("ChartJS x-labels block still contains the old hardcoded stride formula %q — %s", tc.substr, tc.why)
				}
				return
			}
			if !strings.Contains(xBlock, tc.substr) {
				t.Errorf("ChartJS x-labels block missing %q — %s", tc.substr, tc.why)
			}
		})
	}
}
