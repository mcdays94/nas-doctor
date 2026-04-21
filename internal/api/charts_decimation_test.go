package api

import (
	"os/exec"
	"strings"
	"testing"
)

// TestChartJS_DrawAxes_UsesDecimation guards against regression of the fix
// for issue #165 (x-axis label overlap on /disk/<serial>). drawAxes() must:
//  1. measure real label widths via ctx.measureText(), and
//  2. call the exported decimateLabels() helper to pick which indices to
//     render so that no two rendered labels overlap.
//
// If this test fails, someone reverted drawAxes() back to an index-only
// stride formula (e.g. the historic Math.floor(labels.length / (cw / 50))
// that caused #165 in the first place) or bypassed decimateLabels().
func TestChartJS_DrawAxes_UsesDecimation(t *testing.T) {
	js := ChartJS
	const marker = "/* x labels"
	idx := strings.Index(js, marker)
	if idx < 0 {
		t.Fatalf("ChartJS: could not locate %q marker — drawAxes() structure changed unexpectedly", marker)
	}
	end := idx + 1400
	if end > len(js) {
		end = len(js)
	}
	xBlock := js[idx:end]

	mustContain := []struct {
		substr string
		why    string
	}{
		{"measureText", "drawAxes must measure real label widths — hardcoded pixel budgets cause overlap (issue #165)"},
		{"maxLabelWidth", "drawAxes must track the widest measured label (issue #165)"},
		{"decimateLabels(", "drawAxes must call decimateLabels() to pick non-overlapping label indices (issue #165)"},
	}
	for _, tc := range mustContain {
		if !strings.Contains(xBlock, tc.substr) {
			t.Errorf("ChartJS x-labels block missing %q — %s", tc.substr, tc.why)
		}
	}

	mustNotContain := []struct {
		substr string
		why    string
	}{
		{"cw/50", "the old hardcoded-50 stride formula must not come back — root cause of #165"},
		{"labels.length/(cw/50)", "the old hardcoded-50 stride formula must not come back — root cause of #165"},
	}
	for _, tc := range mustNotContain {
		if strings.Contains(xBlock, tc.substr) {
			t.Errorf("ChartJS x-labels block still contains %q — %s", tc.substr, tc.why)
		}
	}
}

// TestChartJS_DecimateLabels_Behavior runs the decimateLabels function in a
// headless node runtime against a table of representative inputs. This is
// the "real" TDD test — it verifies the algorithm produces correct output,
// not just that certain substrings are present.
//
// The test is skipped if node is not on PATH (CI and local dev both have it).
func TestChartJS_DecimateLabels_Behavior(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available; skipping JS behavior test")
	}

	// Runner: injects a fake window/document, evaluates ChartJS, then calls
	// NasChart._decimateLabels with a list of cases and prints JSON results.
	runner := `
const cases = [
  // name, N, cw, L, gap, expectedFirst, expectedLast, expectedMaxGapPx (<=), mustIncludeAll (bool)
  // --- degenerate ---
  { name: "N=0",              n: 0,  cw: 1400, L: 60, expectEmpty: true },
  { name: "N=1",              n: 1,  cw: 1400, L: 60, expect: [0] },
  { name: "cw=0",             n: 10, cw: 0,    L: 60, expect: [0,9] },
  { name: "L=0",              n: 10, cw: 1400, L: 0,  expect: [0,9] },
  { name: "label wider than chart", n: 10, cw: 50, L: 100, expect: [0,9] },

  // --- all fit (1D with small N or small L) ---
  { name: "all-fit tiny",     n: 5,  cw: 1400, L: 60, expect: [0,1,2,3,4] },

  // --- stress ranges from the issue ---
  { name: "1D @ 48 points 1400px L=60", n: 48,   cw: 1400, L: 60,
    mustIncludeFirstLast: true, maxOverlapAllowedPx: 0, minGapPx: 60 },
  { name: "1W @ 336 points 1400px L=60", n: 336,  cw: 1400, L: 60,
    mustIncludeFirstLast: true, maxOverlapAllowedPx: 0, minGapPx: 60 },
  { name: "1M @ 1440 points 1400px L=60", n: 1440, cw: 1400, L: 60,
    mustIncludeFirstLast: true, maxOverlapAllowedPx: 0, minGapPx: 60 },
  { name: "1Y @ 17520 points 1400px L=60", n: 17520, cw: 1400, L: 60,
    mustIncludeFirstLast: true, maxOverlapAllowedPx: 0, minGapPx: 60 },

  // --- narrow container ---
  { name: "narrow 768px 48 points", n: 48, cw: 768, L: 60,
    mustIncludeFirstLast: true, minGapPx: 60 },
];

// Minimal stubs for the browser APIs ChartJS touches at module-load time.
global.window = { matchMedia: () => ({ matches: false }), devicePixelRatio: 1 };
global.document = {
  body: {},
  createElement: () => ({ style: {}, appendChild: () => {} }),
};
global.getComputedStyle = () => ({ backgroundColor: "rgb(255,255,255)", paddingLeft: "0", paddingRight: "0" });
global.requestAnimationFrame = () => {};

const fs = require("fs");
const path = require("path");
const src = fs.readFileSync(path.resolve("internal/api/charts.go"), "utf8");
const m = src.match(/var ChartJS = ` + "`" + `([\s\S]*?)` + "`" + `/);
if (!m) { console.error("could not extract ChartJS"); process.exit(2); }
eval(m[1]);
const decimate = window.NasChart._decimateLabels;

const results = [];
for (const c of cases) {
  try {
    const got = decimate(c.n, c.cw, c.L, 12);
    const entry = { name: c.name, got };

    if (c.expectEmpty) {
      entry.ok = Array.isArray(got) && got.length === 0;
      if (!entry.ok) entry.why = "expected empty array";
    } else if (c.expect) {
      entry.ok = JSON.stringify(got) === JSON.stringify(c.expect);
      if (!entry.ok) entry.why = "expected " + JSON.stringify(c.expect);
    } else {
      // Assertions derived from N,cw,L
      entry.ok = true;
      entry.why = "";
      if (c.mustIncludeFirstLast) {
        if (got[0] !== 0) { entry.ok = false; entry.why = "missing first (0) — got " + got[0]; }
        else if (got[got.length-1] !== c.n - 1) {
          entry.ok = false;
          entry.why = "missing last (" + (c.n-1) + ") — got " + got[got.length-1];
        }
      }
      if (entry.ok && c.minGapPx != null) {
        // pixel gap between adjacent kept labels must be >= L + 12
        const intervals = c.n - 1 || 1;
        const pxPerInterval = c.cw / intervals;
        const needPx = c.L + 12;
        for (let i = 1; i < got.length; i++) {
          const gap = (got[i] - got[i-1]) * pxPerInterval;
          if (gap + 0.0001 < needPx) {
            entry.ok = false;
            entry.why = "pixel gap " + gap.toFixed(2) + " < required " + needPx + " between " + got[i-1] + "→" + got[i];
            break;
          }
        }
      }
      // strictly increasing, in-range
      for (let i = 0; i < got.length; i++) {
        if (got[i] < 0 || got[i] >= c.n) { entry.ok = false; entry.why = "out-of-range index " + got[i]; break; }
        if (i > 0 && got[i] <= got[i-1]) { entry.ok = false; entry.why = "not strictly increasing at " + i; break; }
      }
    }
    results.push(entry);
  } catch (err) {
    results.push({ name: c.name, ok: false, why: "threw: " + err.message });
  }
}
console.log(JSON.stringify(results));
`

	cmd := exec.Command("node", "-e", runner)
	// run from the repo root so the readFileSync path resolves
	cmd.Dir = findRepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node runner failed: %v\noutput:\n%s", err, string(out))
	}
	// Last line is the JSON blob.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	jsonLine := lines[len(lines)-1]

	// Parse by substring search (avoids encoding/json dependency bump).
	// Each result has name + ok. We fail if any "ok":false appears.
	if !strings.Contains(jsonLine, "\"ok\":false") && !strings.Contains(jsonLine, "\"ok\": false") {
		t.Logf("decimateLabels behavior: all cases passed")
		return
	}
	t.Errorf("decimateLabels behavior failures:\n%s", jsonLine)
}

// findRepoRoot walks up from CWD looking for go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	// From internal/api test binary, CWD is that package. Walk up.
	// We know ChartJS is read from "internal/api/charts.go" relative to repo root.
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}
