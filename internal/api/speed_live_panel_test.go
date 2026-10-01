package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The live speed-test panel (NasSpeedLive in ChartJS) and the dashboard
// card around it (DashboardJS) are plain browser JavaScript. These tests
// run them in node against a small fake DOM, so CI catches a broken panel
// without a browser. They skip when node isn't installed.

// speedLiveHarnessJS is a minimal DOM: elements with attributes, classes,
// text, hidden, a tiny innerHTML parser and querySelector for the
// selectors the panel uses ([attr], [attr=value], .class). The canvas
// context records what was drawn. requestAnimationFrame queues frames
// until flushFrames(), and setInterval never fires, so runs are
// deterministic. No backticks: this lives in a Go raw string.
const speedLiveHarnessJS = `
var fs = require("fs");
var failures = [];
function check(cond, msg) { if (!cond) failures.push(msg); }

function El(tag) {
  this.tagName = String(tag).toUpperCase();
  this.nodeType = 1;
  this.childNodes = [];
  this.parentNode = null;
  this.attrs = {};
  this.style = {};
  this.text = "";
  this.hidden = false;
  this.disabled = false;
  this.id = "";
  this.bg = "";
  this.clientWidth = 400;
  this.clientHeight = 120;
  this.offsetWidth = 400;
}
Object.defineProperty(El.prototype, "className", {
  get: function() { return this.attrs["class"] || ""; },
  set: function(v) { this.attrs["class"] = String(v); }
});
Object.defineProperty(El.prototype, "classList", {
  get: function() {
    var el = this;
    return {
      add: function(c) { var cs = el.className.split(/\s+/).filter(Boolean); if (cs.indexOf(c) < 0) cs.push(c); el.className = cs.join(" "); },
      contains: function(c) { return el.className.split(/\s+/).indexOf(c) >= 0; }
    };
  }
});
Object.defineProperty(El.prototype, "textContent", {
  get: function() { return this.text + this.childNodes.map(function(c) { return c.textContent; }).join(""); },
  set: function(v) { this.childNodes = []; this.text = String(v); }
});
function escText(s) { return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }
Object.defineProperty(El.prototype, "innerHTML", {
  get: function() { return escText(this.text) + this.childNodes.map(function(c) { return c.nodeType === 3 ? escText(c.textContent) : ""; }).join(""); },
  set: function(html) { this.childNodes = []; this.text = ""; parseInto(this, html); }
});
Object.defineProperty(El.prototype, "firstChild", { get: function() { return this.childNodes[0] || null; } });
El.prototype.setAttribute = function(k, v) {
  if (k === "hidden") { this.hidden = true; return; }
  if (k === "id") { this.id = String(v); return; }
  this.attrs[k] = String(v);
};
El.prototype.getAttribute = function(k) {
  if (k === "id") return this.id || null;
  return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null;
};
El.prototype.appendChild = function(c) { c.parentNode = this; this.childNodes.push(c); return c; };
El.prototype.contains = function(n) { for (; n; n = n.parentNode) if (n === this) return true; return false; };
El.prototype.closest = function(sel) { for (var n = this; n && n.nodeType === 1; n = n.parentNode) if (matches(n, sel)) return n; return null; };
El.prototype.querySelectorAll = function(sel) { var out = []; walk(this, function(n) { if (matches(n, sel)) out.push(n); }); return out; };
El.prototype.querySelector = function(sel) { return this.querySelectorAll(sel)[0] || null; };
El.prototype.getContext = function() { return ctx; };

function walk(el, fn) {
  for (var i = 0; i < el.childNodes.length; i++) {
    var c = el.childNodes[i];
    if (c.nodeType !== 1) continue;
    fn(c);
    walk(c, fn);
  }
}
function matches(n, sel) {
  var m = /^\[([\w-]+)(?:=["']?([^"'\]]*)["']?)?\]$/.exec(sel);
  if (m) return m[2] === undefined ? n.getAttribute(m[1]) !== null : n.getAttribute(m[1]) === m[2];
  if (sel.charAt(0) === ".") return n.classList.contains(sel.slice(1));
  return n.tagName === sel.toUpperCase();
}
function parseInto(root, html) {
  var stack = [root], re = /<(\/?)([a-zA-Z0-9]+)([^>]*)>|([^<]+)/g, m;
  while ((m = re.exec(html))) {
    var top = stack[stack.length - 1];
    if (m[4] !== undefined) { top.text += m[4]; continue; }
    if (m[1]) { stack.pop(); continue; }
    var el = new El(m[2]);
    var ar = /([\w-]+)(?:="([^"]*)")?/g, a;
    while ((a = ar.exec(m[3]))) el.setAttribute(a[1], a[2] === undefined ? "" : a[2]);
    top.appendChild(el);
    stack.push(el);
  }
}

var drawn = { stroke: 0, text: [] };
var ctx = new Proxy({}, {
  get: function(t, k) {
    if (k in t) return t[k];
    if (k === "createLinearGradient") return function() { return { addColorStop: function() {} }; };
    if (k === "measureText") return function(s) { return { width: String(s).length * 6 }; };
    if (k === "fillText") return function(s) { drawn.text.push(String(s)); };
    if (k === "stroke") return function() { drawn.stroke++; };
    return function() {};
  },
  set: function(t, k, v) { t[k] = v; return true; }
});

var html = new El("html"), head = new El("head"), body = new El("body");
html.appendChild(head);
html.appendChild(body);
var clickHandlers = [];
global.window = global;
window.devicePixelRatio = 1;
window.matchMedia = function() { return { matches: false }; };
window.addEventListener = function() {};
body.addEventListener = function(type, fn) { if (type === "click") clickHandlers.push(fn); };
global.document = {
  head: head,
  body: body,
  documentElement: html,
  createElement: function(tag) { return new El(tag); },
  createTextNode: function(s) { return { nodeType: 3, textContent: String(s) }; },
  getElementById: function(id) { var found = null; walk(html, function(n) { if (!found && n.id === id) found = n; }); return found; },
  querySelectorAll: function(sel) { return html.querySelectorAll(sel); },
  addEventListener: function(type, fn) { if (type === "click") clickHandlers.push(fn); }
};
global.getComputedStyle = function(n) {
  return {
    backgroundColor: n.bg || "rgba(0, 0, 0, 0)",
    fontFamily: "Inter",
    paddingLeft: "0",
    paddingRight: "0",
    getPropertyValue: function() { return ""; }
  };
};
var frames = [];
global.requestAnimationFrame = function(cb) { frames.push(cb); return frames.length; };
global.cancelAnimationFrame = function() {};
function flushFrames() { var f = frames; frames = []; f.forEach(function(cb) { cb(); }); }
global.setInterval = function() { return 1; };
global.clearInterval = function() {};
function click(el) { clickHandlers.forEach(function(fn) { fn({ target: el }); }); }
function mount(parent, id) { var el = new El("div"); el.id = id; el.hidden = true; parent.appendChild(el); return el; }
function iso(ms) { return new Date(ms).toISOString(); }
function tick() { return new Promise(function(r) { setTimeout(r, 5); }); }

(0, eval)(fs.readFileSync("charts.js", "utf8"));
`

// runSpeedLiveScenario runs body after the harness and ChartJS (plus
// DashboardJS when withDashboard is set) in node, and fails the test with
// every failed check. body may be async: it runs inside an async main.
func runSpeedLiveScenario(t *testing.T, withDashboard bool, body string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available; skipping JS behaviour test")
	}
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("charts.js", ChartJS)
	write("dashboard.js", DashboardJS)
	runner := speedLiveHarnessJS
	if withDashboard {
		runner += "\n(0, eval)(fs.readFileSync(\"dashboard.js\", \"utf8\"));\n"
	}
	runner += "\nasync function main() {\n" + body + "\n}\n" +
		"main().then(function() { console.log(JSON.stringify({ failures: failures })); }," +
		" function(e) { console.log(JSON.stringify({ failures: failures.concat([\"threw: \" + (e && e.stack || e)]) })); });\n"
	write("runner.js", runner)

	cmd := exec.Command("node", "runner.js")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node runner failed: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var res struct {
		Failures []string `json:"failures"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &res); err != nil {
		t.Fatalf("runner output is not JSON: %v\n%s", err, out)
	}
	for _, f := range res.Failures {
		t.Error(f)
	}
}

// The dashboard card's panel: compact, Cancel while running, Close after.
// It has to survive the dashboard replacing its HTML mid-test.
func TestNasSpeedLive_DashboardRun(t *testing.T) {
	runSpeedLiveScenario(t, false, `
  var card = new El("div");
  card.bg = "rgb(25, 26, 27)";
  body.appendChild(card);
  var root = mount(card, "live");
  function q(role) { return root.querySelector('[data-role="' + role + '"]'); }
  function step(name) { return root.querySelector("[data-step=" + name + "]").getAttribute("data-state"); }
  var stops = 0, closes = 0;
  var p = NasSpeedLive.create({ id: "live", compact: true, stopLabel: "Cancel",
    onStop: function() { stops++; }, onClose: function() { closes++; } });
  check(!p.isOpen() && root.hidden, "a new panel stays hidden");

  p.start({});
  check(!root.hidden, "start shows the panel");
  check(root.classList.contains("speed-live") && root.classList.contains("speed-live-compact"), "start adds the panel classes: " + root.className);
  check(root.getAttribute("data-tone") === "dark", "a dark card gives the dark tone, got " + root.getAttribute("data-tone"));
  check(document.getElementById("speed-live-css") !== null, "start injects the stylesheet");
  check(q("status").textContent === "Connecting…", "status before the first phase: " + q("status").textContent);
  var stop = q("stop");
  check(!stop.hidden && !stop.disabled && stop.textContent === "Cancel", "Cancel is live while the test runs");
  click(stop);
  check(stops === 1 && closes === 0, "Cancel calls onStop");

  var now = Date.now();
  p.phase("latency");
  p.sample({ phase: "latency", latency_ms: 10, ts: iso(now) });
  p.sample({ phase: "latency", latency_ms: 14, ts: iso(now) });
  check(q("latency").textContent === "14.0", "latency shows the newest ping, got " + q("latency").textContent);
  check(step("latency") === "active", "latency step is active");
  p.phase("download");
  check(q("latency").textContent === "12.0", "the finished latency phase settles to its average, got " + q("latency").textContent);
  check(step("latency") === "done" && step("download") === "active", "the stepper moves on to download");

  for (var i = 0; i < 8; i++) p.sample({ phase: "download", mbps: 100 * (i + 1), ts: iso(now + i * 500) });
  check(q("download").textContent === "800", "download shows the newest sample, got " + q("download").textContent);
  check(/^\d+\.\d s$/.test(q("status").textContent), "compact status is the elapsed time, got " + q("status").textContent);
  drawn.stroke = 0;
  flushFrames();
  check(drawn.stroke > 0, "the next frame draws the chart");

  // The dashboard re-renders: the old element is gone and an empty one
  // takes its place. redraw() must bring the running test back.
  card.childNodes = [];
  root = mount(card, "live");
  p.redraw();
  check(!root.hidden && q("download").textContent === "800", "redraw refills a fresh element with the running test");
  check(step("latency") === "done" && step("download") === "active", "redraw restores the stepper");
  check(q("stop").textContent === "Cancel" && !q("stop").disabled, "redraw restores the Cancel button");

  p.phase("upload");
  check(q("download").textContent === "550", "download settles to the average without the ramp-up quarter, got " + q("download").textContent);
  for (var j = 0; j < 4; j++) p.sample({ phase: "upload", mbps: 40 + j, ts: iso(now + 5000 + j * 500) });
  p.setStop(false, "Cancelling...");
  check(q("stop").disabled && q("stop").textContent === "Cancelling...", "setStop disables the button and relabels it");

  p.finish({ outcome: "complete", seconds: 31.24, download: 905.4, upload: 410.2, latency: 8.14 });
  check(!p.isRunning() && p.isOpen() && !root.hidden, "the finished test stays in the card");
  check(q("download").textContent === "905" && q("upload").textContent === "410" && q("latency").textContent === "8.1",
    "the readouts show the result: " + q("download").textContent + "/" + q("upload").textContent + "/" + q("latency").textContent);
  check(step("latency") === "done" && step("download") === "done" && step("upload") === "done", "every step is done");
  check(q("status").textContent === "Done in 31.2 s", "finished status, got " + q("status").textContent);
  stop = q("stop");
  check(!stop.hidden && !stop.disabled && stop.textContent === "Close", "Cancel turns into Close, got " + stop.textContent);
  click(stop);
  check(closes === 1 && stops === 1, "Close calls onClose, not onStop");

  p.result({ lines: [{ text: "Jitter 0.3 ms · <b>Coimbra</b>" }, { tone: "warn", text: "careful" }] });
  var res = q("result");
  check(!res.hidden, "the result area shows");
  check(res.childNodes.length === 2 && res.childNodes[0].className === "speed-live-meta" && res.childNodes[1].className === "speed-live-warn",
    "result lines get the meta and warn classes");
  check(res.childNodes[0].textContent === "Jitter 0.3 ms · <b>Coimbra</b>" && res.childNodes[0].childNodes.length === 0,
    "result text is set as text, never parsed as HTML");

  card.childNodes = [];
  root = mount(card, "live");
  p.redraw();
  check(q("status").textContent === "Done in 31.2 s" && q("result").childNodes.length === 2, "redraw restores the finished test and its result");

  p.hide();
  check(root.hidden && !p.isOpen(), "hide closes the panel");
  p.redraw();
  check(root.hidden, "redraw keeps a closed panel hidden");
`)
}

// The Settings Test panel: full size, contracted lines, Stop that hides
// after the test, a verdict badge.
func TestNasSpeedLive_SettingsRun(t *testing.T) {
	runSpeedLiveScenario(t, false, `
  var form = new El("div");
  body.appendChild(form);
  var root = mount(form, "sc");
  root.bg = "rgb(250, 250, 250)";
  function q(role) { return root.querySelector('[data-role="' + role + '"]'); }
  var p = NasSpeedLive.create({ id: "sc", stopLabel: "Stop" });
  p.start({ contractedDown: 500, contractedUp: 100 });
  check(!root.classList.contains("speed-live-compact"), "the settings panel isn't compact");
  check(root.getAttribute("data-tone") === "light", "a light surface gives the light tone, got " + root.getAttribute("data-tone"));
  var now = Date.now();
  p.phase("download");
  p.sample({ phase: "download", mbps: 300, ts: iso(now) });
  check(/^Measuring download · \d+\.\d s$/.test(q("status").textContent), "full status names the phase, got " + q("status").textContent);
  drawn.text = [];
  flushFrames();
  check(drawn.text.indexOf("contracted") >= 0, "the chart labels the contracted speed lines");

  p.finish({ outcome: "stopped", seconds: 12.34 });
  check(q("status").textContent === "Stopped after 12.3 s", "stopped status, got " + q("status").textContent);
  check(q("stop").hidden, "without onClose the button hides after the test");
  check(root.querySelector("[data-step=download]").getAttribute("data-state") === "", "an unfinished step isn't marked done");
  check(q("download").textContent === "300", "a stopped test keeps the live readout");

  p.result({ badge: { status: "degraded", label: "Degraded" }, summary: "Download 300 Mbps", lines: [{ text: "Took 12.3 s" }] });
  var head = q("result").childNodes[0];
  check(head && head.className === "speed-live-result-head", "badge and summary share the head row");
  check(head && head.childNodes[0].getAttribute("data-status") === "degraded" && head.childNodes[0].textContent === "Degraded", "the badge carries the verdict");

  p.start({});
  check(q("result").hidden && q("download").textContent === "–", "a new test clears the old result and readouts");
  p.finish({ outcome: "failed" });
  check(q("status").textContent === "Failed", "failed status, got " + q("status").textContent);
`)
}

// A panel that attaches mid-test shows the test's real age from the
// server's start time, whatever the clock difference to the browser.
func TestNasSpeedLive_ElapsedUsesServerStart(t *testing.T) {
	runSpeedLiveScenario(t, false, `
  var root = mount(body, "live");
  var p = NasSpeedLive.create({ id: "live", compact: true });
  var skew = 60000;
  var serverNow = Date.now() + skew;
  p.start({ startedAt: iso(serverNow - 20000) });
  p.phase("download");
  p.sample({ phase: "download", mbps: 5, ts: iso(serverNow - 15000) });
  p.sample({ phase: "download", mbps: 9, ts: iso(serverNow) });
  var secs = parseFloat(root.querySelector('[data-role="status"]').textContent);
  check(secs >= 19.5 && secs <= 21, "elapsed should be about 20 s with the server 60 s ahead, got " + secs);
`)
}

// Every branch of the dashboard card emits the live panel's element, so
// Run now can show the test whatever state the card is in.
func TestDashboardJS_SpeedTestSection_EveryBranchHasLivePanel(t *testing.T) {
	runSpeedLiveScenario(t, true, `
  var latest = { download_mbps: 900, upload_mbps: 400, latency_ms: 8, jitter_ms: 0.3, server_name: "Coimbra", isp: "NOS",
    engine: "speedtest_go", timestamp: new Date().toISOString() };
  var cases = {
    latest: { speed_test: { available: true, latest: latest } },
    disabled: { speed_test: { last_attempt: { status: "disabled" } } },
    pending: { speed_test: { last_attempt: { status: "pending" } } },
    empty: {}
  };
  Object.keys(cases).forEach(function(name) {
    var h = NasDashboard.sections.speedtest(cases[name]);
    check(h.split('id="speedtest-live"').length === 2, name + ": want exactly one #speedtest-live");
    check(h.indexOf("data-speedtest-summary") >= 0, name + ": want the summary the panel hides");
    check(h.indexOf('data-action="speedtest-run-now"') >= 0, name + ": want the Run now button");
  });
  check(NasDashboard.sections.speedtest(cases.latest).indexOf('id="speedtest-chart-note"') >= 0, "the history chart has a note line");
`)
}

// The history chart widens to the first window with real tests and says
// so; cancelled tests (all-zero rows) don't count.
func TestDashboardJS_SpeedTestHistory_Fallback(t *testing.T) {
	runSpeedLiveScenario(t, true, `
  var canvas = new El("canvas"); canvas.id = "speedtest-chart"; body.appendChild(canvas);
  var note = new El("div"); note.id = "speedtest-chart-note"; note.hidden = true; body.appendChild(note);
  var drawnAreas = [];
  NasChart.area = function(id, o) { drawnAreas.push({ id: id, n: o.datasets[0].data.length }); };
  var history = {}, asked = [];
  global.fetch = function(url) {
    var h = /hours=(\d+)/.exec(url)[1];
    asked.push(Number(h));
    return Promise.resolve({ ok: true, json: function() { return Promise.resolve(history[h] || []); } });
  };
  var t = new Date().toISOString();
  function pt(d, u) { return { download_mbps: d, upload_mbps: u, latency_ms: 8, timestamp: t }; }

  history = { "24": [pt(0, 0)], "168": [pt(900, 400), pt(880, 410)] };
  NasDashboard.charts.loadSpeedTest(1);
  for (var i = 0; i < 10; i++) await tick();
  check(asked.join(",") === "1,24,168", "1H falls back past a window with only a cancelled test, asked " + asked.join(","));
  check(!note.hidden && note.textContent === "No tests in the last hour, showing the last 7 days.", "fallback note, got " + JSON.stringify(note.textContent));
  check(drawnAreas.length === 1 && drawnAreas[0].n === 2, "draws the 7-day points: " + JSON.stringify(drawnAreas));
  check(canvas.style.display === "", "the chart is visible");

  history = {}; asked = []; drawnAreas = [];
  NasDashboard.charts.loadSpeedTest(24);
  for (var j = 0; j < 10; j++) await tick();
  check(asked.join(",") === "24,168,720", "an empty history tries every wider window, asked " + asked.join(","));
  check(!note.hidden && note.textContent === "No speed tests in the last 30 days.", "empty note, got " + JSON.stringify(note.textContent));
  check(canvas.style.display === "none" && drawnAreas.length === 0, "nothing to draw hides the chart");

  history = { "168": [pt(900, 400)] }; asked = []; drawnAreas = [];
  NasDashboard.charts.loadSpeedTest(168);
  for (var k = 0; k < 10; k++) await tick();
  check(asked.join(",") === "168", "a window with tests needs one request, asked " + asked.join(","));
  check(note.hidden && canvas.style.display === "" && drawnAreas.length === 1, "no note when the chosen window has tests");
`)
}

// ChartJS and DashboardJS are served as standalone scripts; a syntax
// error in either blanks the dashboard.
func TestChartJSAndDashboardJS_Parse(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available; skipping JS parse check")
	}
	for name, src := range map[string]string{"charts.js": ChartJS, "dashboard.js": DashboardJS} {
		cmd := exec.Command("node", "--check", "-")
		cmd.Stdin = strings.NewReader(src)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s does not parse:\n%s", name, out)
		}
	}
}
