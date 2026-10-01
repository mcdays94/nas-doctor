package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The dashboard has to fit the screen it's on: one column on a phone, the
// Settings column count on wide screens, and history charts that follow
// their card's width. The JS tests run DashboardJS and ChartJS in node
// against a small fake DOM and skip when node isn't installed. The CSS
// tests read the theme rules that make the narrow layout win.

// layoutHarnessJS is a minimal DOM: elements with attributes, classes,
// inline styles (custom properties included), sizes the scenario sets by
// hand, and querySelector for the simple selectors the code uses. The
// canvas context records clearRect widths, requestAnimationFrame queues
// frames until flushFrames(), and resized() plays the part of layout for
// ResizeObserver. No backticks: this lives in a Go raw string.
const layoutHarnessJS = `
var fs = require("fs");
var failures = [];
function check(cond, msg) { if (!cond) failures.push(msg); }

function Style() { this.props = {}; }
Style.prototype.setProperty = function(k, v) { this.props[k] = String(v); };
Style.prototype.getPropertyValue = function(k) { return this.props[k] || ""; };

function El(tag) {
  this.tagName = String(tag).toUpperCase();
  this.nodeType = 1;
  this.childNodes = [];
  this.parentNode = null;
  this.attrs = {};
  this.style = new Style();
  this.id = "";
  this.listeners = {};
  this.clientWidth = 0;
  this.offsetHeight = 0;
  this.padX = 0;
}
Object.defineProperty(El.prototype, "className", {
  get: function() { return this.attrs["class"] || ""; },
  set: function(v) { this.attrs["class"] = String(v); }
});
Object.defineProperty(El.prototype, "classList", {
  get: function() {
    var el = this;
    function list() { return el.className.split(/\s+/).filter(Boolean); }
    return {
      add: function(c) { var cs = list(); if (cs.indexOf(c) < 0) cs.push(c); el.className = cs.join(" "); },
      remove: function(c) { el.className = list().filter(function(x) { return x !== c; }).join(" "); },
      contains: function(c) { return list().indexOf(c) >= 0; }
    };
  }
});
Object.defineProperty(El.prototype, "parentElement", { get: function() { return this.parentNode && this.parentNode.nodeType === 1 ? this.parentNode : null; } });
Object.defineProperty(El.prototype, "children", { get: function() { return this.childNodes.filter(function(c) { return c.nodeType === 1; }); } });
Object.defineProperty(El.prototype, "isConnected", { get: function() { for (var n = this; n; n = n.parentNode) if (n === html) return true; return false; } });
El.prototype.setAttribute = function(k, v) { if (k === "id") this.id = String(v); else this.attrs[k] = String(v); };
El.prototype.getAttribute = function(k) {
  if (k === "id") return this.id || null;
  return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null;
};
El.prototype.appendChild = function(c) { if (c.parentNode) c.parentNode.removeChild(c); c.parentNode = this; this.childNodes.push(c); return c; };
El.prototype.removeChild = function(c) { this.childNodes = this.childNodes.filter(function(x) { return x !== c; }); c.parentNode = null; return c; };
El.prototype.remove = function() { if (this.parentNode) this.parentNode.removeChild(this); };
El.prototype.addEventListener = function(type, fn) { (this.listeners[type] = this.listeners[type] || []).push(fn); };
El.prototype.getBoundingClientRect = function() { return { left: 0, top: 0, width: this.clientWidth, height: 0 }; };
El.prototype.querySelectorAll = function(sel) { var out = []; walk(this, function(n) { if (matches(n, sel)) out.push(n); }); return out; };
El.prototype.querySelector = function(sel) { return this.querySelectorAll(sel)[0] || null; };
El.prototype.getContext = function() { return ctx; };

function walk(el, fn) { el.children.forEach(function(c) { fn(c); walk(c, fn); }); }
// tag, .class, #id, [attr], [attr=value] and [attr^=prefix]
function matches(n, sel) {
  var m = /^\[([\w-]+)(?:(\^?)=["']?([^"'\]]*)["']?)?\]$/.exec(sel);
  if (m) {
    var v = n.getAttribute(m[1]);
    if (m[3] === undefined) return v !== null;
    return m[2] ? v !== null && v.indexOf(m[3]) === 0 : v === m[3];
  }
  if (sel.charAt(0) === ".") return n.classList.contains(sel.slice(1));
  if (sel.charAt(0) === "#") return n.id === sel.slice(1);
  return n.tagName === sel.toUpperCase();
}

var drawn = { clears: [] };
var ctx = new Proxy({}, {
  get: function(t, k) {
    if (k in t) return t[k];
    if (k === "createLinearGradient") return function() { return { addColorStop: function() {} }; };
    if (k === "measureText") return function(s) { return { width: String(s).length * 6 }; };
    if (k === "clearRect") return function(x, y, w) { drawn.clears.push(w); };
    return function() {};
  },
  set: function(t, k, v) { t[k] = v; return true; }
});

var html = new El("html"), body = new El("body");
html.appendChild(body);
global.window = global;
window.devicePixelRatio = 2;
window.innerWidth = 1400;
window.matchMedia = function() { return { matches: false }; };
global.document = {
  body: body,
  documentElement: html,
  createElement: function(tag) { return new El(tag); },
  createTextNode: function(s) { return { nodeType: 3, textContent: String(s) }; },
  getElementById: function(id) { var found = null; walk(html, function(n) { if (!found && n.id === id) found = n; }); return found; },
  querySelector: function(sel) { return html.querySelector(sel); },
  querySelectorAll: function(sel) { return html.querySelectorAll(sel); },
  addEventListener: function() {}
};
global.getComputedStyle = function(n) {
  return { backgroundColor: "rgb(255, 255, 255)", paddingLeft: String(n.padX || 0), paddingRight: String(n.padX || 0) };
};

var frames = [], clock = 1000;
global.requestAnimationFrame = function(cb) { frames.push(cb); return frames.length; };
function flushFrames() {
  while (frames.length) { var f = frames; frames = []; clock += 600; f.forEach(function(cb) { cb(clock); }); }
}

var observers = [];
global.ResizeObserver = function(cb) { this.cb = cb; this.targets = []; observers.push(this); };
ResizeObserver.prototype.observe = function(el) { if (this.targets.indexOf(el) < 0) this.targets.push(el); };
ResizeObserver.prototype.unobserve = function(el) { this.targets = this.targets.filter(function(x) { return x !== el; }); };
function observed(el) { return observers.some(function(o) { return o.targets.indexOf(el) >= 0; }); }
// What layout does when el's content box becomes width px wide.
function resized(el, width) {
  observers.forEach(function(o) {
    if (o.targets.indexOf(el) >= 0) o.cb([{ target: el, contentRect: { width: width, height: 0 } }], o);
  });
}

function serve(routes) {
  global.fetch = function(url) {
    var data = null;
    Object.keys(routes).forEach(function(prefix) { if (url.indexOf(prefix) === 0) data = routes[prefix]; });
    return Promise.resolve({ ok: true, status: 200, json: function() { return Promise.resolve(data); } });
  };
}
function tick() { return new Promise(function(r) { setTimeout(r, 5); }); }
`

// runLayoutScenario runs body in node after the harness and the named
// scripts ("charts.js" for ChartJS, "dashboard.js" for DashboardJS), and
// fails the test with every failed check. body may await.
func runLayoutScenario(t *testing.T, scripts []string, body string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available; skipping JS behaviour test")
	}
	dir := t.TempDir()
	sources := map[string]string{"charts.js": ChartJS, "dashboard.js": DashboardJS}
	runner := layoutHarnessJS
	for _, name := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sources[name]), 0o644); err != nil {
			t.Fatal(err)
		}
		runner += "\n(0, eval)(fs.readFileSync(" + strconv.Quote(name) + ", \"utf8\"));\n"
	}
	runner += "\nasync function main() {\n" + body + "\n}\n" +
		"main().then(function() { console.log(JSON.stringify({ failures: failures })); }," +
		" function(e) { console.log(JSON.stringify({ failures: failures.concat([\"threw: \" + (e && e.stack || e)]) })); });\n"
	if err := os.WriteFile(filepath.Join(dir, "runner.js"), []byte(runner), 0o644); err != nil {
		t.Fatal(err)
	}
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

// distributeSections hands the column count to CSS as --dash-cols. An
// inline grid-template-columns beats the themes' 900px media query, which
// squeezed three ~100px columns onto a phone.
func TestDashboardJS_DistributeSections_ColumnCountIsACSSVariable(t *testing.T) {
	runLayoutScenario(t, []string{"dashboard.js"}, `
  NasDashboard.NasScrollFade.init = function() {};
  var names = ["findings", "drives", "docker", "network", "speedtest", "services"];
  // [dash_columns setting, columns the dashboard should ask for]
  var cases = [[0, 3], [1, 1], [2, 2], [3, 3], [4, 4]];
  for (var i = 0; i < cases.length; i++) {
    var setting = cases[i][0], want = cases[i][1];
    serve({ "/api/v1/status": { sections: { dash_columns: setting } }, "/api/v1/snapshot/latest": {} });
    await NasDashboard.polling.loadAll();

    // What a theme's render() writes before it calls distributeSections.
    body.childNodes = [];
    var app = new El("div"); app.id = "app"; app.className = "container"; body.appendChild(app);
    var grid = new El("div"); grid.id = "two-col"; grid.className = "two-col"; app.appendChild(grid);
    var cols = ["col-left", "col-right"];
    for (var c = 3; c <= want; c++) cols.push("col-" + c);
    cols.forEach(function(id) { var col = new El("div"); col.id = id; grid.appendChild(col); });
    var staging = new El("div"); staging.id = "section-staging"; app.appendChild(staging);
    names.forEach(function(name, k) {
      var s = new El("div"); s.className = "section-block"; s.setAttribute("data-section", name); s.offsetHeight = 100 + 10 * k;
      staging.appendChild(s);
    });

    NasDashboard.distributeSections();
    var label = "dash_columns " + setting + ": ";
    check(grid.style.getPropertyValue("--dash-cols") === String(want),
      label + "--dash-cols should be " + want + ", got " + JSON.stringify(grid.style.getPropertyValue("--dash-cols")));
    check(grid.style.gridTemplateColumns === undefined && grid.style.getPropertyValue("grid-template-columns") === "",
      label + "an inline grid-template-columns would beat the themes' 900px media query, got " +
      JSON.stringify(grid.style.gridTemplateColumns || grid.style.getPropertyValue("grid-template-columns")));
    check(app.classList.contains("dash-wide") === (want >= 3), label + "dash-wide should be set from 3 columns, class is " + JSON.stringify(app.className));
    check(grid.querySelectorAll(".section-block").length === names.length && !document.getElementById("section-staging"),
      label + "every section should land in a column");
  }
`)
}

// A fluid chart takes its card's width and redraws when the card changes
// width without a window resize: the grid dropping to one column below
// 900px, or a section moving to another column.
func TestChartJS_FluidChartFollowsItsBox(t *testing.T) {
	runLayoutScenario(t, []string{"charts.js"}, `
  var card = new El("div"); card.clientWidth = 420; card.padX = 10; body.appendChild(card);
  var canvas = new El("canvas"); canvas.id = "hist"; card.appendChild(canvas);
  var labels = ["L0", "L1", "L2", "L3", "L4"];
  var opts = { datasets: [{ data: [1, 5, 3, 8, 2], color: "#3b82f6", label: "Download" }], labels: labels, height: 60, fluid: true };
  NasChart.area("hist", opts);
  check(canvas.width === 800 && canvas.style.width === "100%",
    "a fluid chart should fill its card's 400px content box at 100% CSS width, got a " + canvas.width + "px bitmap and width " + canvas.style.width);
  check(observed(canvas), "a fluid chart should be watched for size changes");
  var bodyKids = body.children.length;
  var introFrames = frames.length;
  check(introFrames > 0, "the first draw plays the intro animation");

  // The card narrows while the intro animation is still running.
  card.clientWidth = 270;
  drawn.clears = [];
  resized(canvas, 250);
  check(canvas.width === 500, "after the card narrows to 250px the chart should be redrawn at 250px, got a " + canvas.width + "px bitmap");
  check(drawn.clears.length > 0 && drawn.clears.every(function(w) { return w === 250; }),
    "the redraw should paint at once at the new width, cleared " + JSON.stringify(drawn.clears));
  check(frames.length === introFrames, "the redraw shouldn't replay the intro animation");
  drawn.clears = [];
  flushFrames();
  check(drawn.clears.indexOf(400) < 0, "the first draw's intro animation painted over the redraw at the old width");

  check(body.children.length === bodyKids, "redraws should reuse the chart's tooltip, body went from " + bodyKids + " to " + body.children.length + " children");
  check(canvas.listeners.mousemove.length === 1, "want one mousemove listener on the canvas, got " + canvas.listeners.mousemove.length);
  // Hover the right edge of the redrawn plot (compact margins: 32 left, 8 right).
  canvas.listeners.mousemove[0]({ clientX: 242, clientY: 30 });
  var tip = body.children[body.children.length - 1];
  check(String(tip.innerHTML).indexOf("L4") >= 0, "the tooltip should answer for the redrawn chart (L4 at its right edge), got " + tip.innerHTML);

  drawn.clears = [];
  resized(canvas, 250);
  check(drawn.clears.length === 0, "a size report with the drawn width shouldn't redraw");
  resized(canvas, 0);
  check(canvas.width === 500 && drawn.clears.length === 0, "a hidden chart (0px wide) shouldn't be redrawn");

  // A dashboard re-render swaps in a new canvas with the same id.
  card.removeChild(canvas);
  var fresh = new El("canvas"); fresh.id = "hist"; card.appendChild(fresh);
  NasChart.area("hist", opts);
  check(observed(fresh) && !observed(canvas), "the new canvas should be watched and the replaced one let go");

  ["line", "bar"].forEach(function(kind) {
    var box = new El("div"); box.clientWidth = 300; body.appendChild(box);
    var cv = new El("canvas"); box.appendChild(cv);
    NasChart[kind](cv, kind === "bar"
      ? { data: [1, 2], labels: ["a", "b"], height: 120, fluid: true }
      : { datasets: [{ data: [1, 2] }], labels: ["a", "b"], height: 120, fluid: true });
    box.clientWidth = 200;
    resized(cv, 200);
    check(cv.width === 400, kind + " charts should follow their box too, got a " + cv.width + "px bitmap");
  });

  var fixedCard = new El("div"); fixedCard.clientWidth = 420; body.appendChild(fixedCard);
  var fixed = new El("canvas"); fixed.id = "fixed"; fixedCard.appendChild(fixed);
  NasChart.area("fixed", { datasets: [{ data: [1, 2, 3] }], labels: ["a", "b", "c"], width: 300, height: 60 });
  check(fixed.width === 600 && fixed.style.width === "300px", "a chart drawn with a width should keep it, got " + fixed.width + " / " + fixed.style.width);
  check(!observed(fixed), "a chart without opts.fluid shouldn't be watched");

  // Redrawn at a fixed width, a fluid canvas stops following its box.
  NasChart.area(fresh, { datasets: [{ data: [1, 2, 3] }], labels: ["a", "b", "c"], width: 300, height: 60 });
  card.clientWidth = 520;
  resized(fresh, 500);
  check(fresh.width === 600 && fresh.style.width === "300px", "a fixed-width redraw shouldn't be resized back to the old fluid chart, got " + fresh.width + " / " + fresh.style.width);
`)
}

// The dashboard's history charts are fluid, so they follow their card.
func TestDashboardJS_HistoryChartsAreFluid(t *testing.T) {
	runLayoutScenario(t, []string{"dashboard.js"}, `
  var areas = [];
  window.NasChart = { area: function(id, o) { areas.push({ id: id, o: o }); }, sparkline: function() {} };
  var t = new Date().toISOString();
  serve({
    "/api/v1/history/gpu": [{ gpu_index: 0, usage_percent: 40, timestamp: t }, { gpu_index: 0, usage_percent: 60, timestamp: t }],
    "/api/v1/history/containers": [{ name: "plex", cpu_percent: 5, mem_mb: 100, timestamp: t }, { name: "plex", cpu_percent: 7, mem_mb: 120, timestamp: t }],
    "/api/v1/history/speedtest": [{ download_mbps: 900, upload_mbps: 400, timestamp: t }, { download_mbps: 880, upload_mbps: 410, timestamp: t }]
  });
  var card = new El("div"); body.appendChild(card);
  ["gpu-chart-0", "cmetrics-chart-0", "speedtest-chart"].forEach(function(id) { var c = new El("canvas"); c.id = id; card.appendChild(c); });
  document.getElementById("cmetrics-chart-0").setAttribute("data-container", "plex");

  NasDashboard.charts.loadGPU(1);
  NasDashboard.charts.loadContainers(1);
  NasDashboard.charts.loadSpeedTest(1);
  for (var i = 0; i < 10; i++) await tick();
  var ids = areas.map(function(a) { return a.id; }).sort();
  check(JSON.stringify(ids) === JSON.stringify(["cmetrics-chart-0", "gpu-chart-0", "speedtest-chart"]), "want the three history charts drawn, got " + JSON.stringify(ids));
  areas.forEach(function(a) {
    check(a.o.fluid === true, a.id + " should be drawn with fluid: true so it follows its card's width");
    check(a.o.width === undefined, a.id + " shouldn't pin a width, got " + a.o.width);
  });
`)
}

type cssBlock struct {
	start, end int // byte range of the whole block in the compacted CSS
	body       string
}

// themeCSS returns a theme's <style> contents without comments or
// whitespace, so the checks don't depend on formatting.
func themeCSS(t *testing.T, tpl string) string {
	t.Helper()
	start := strings.Index(tpl, "<style>")
	end := strings.Index(tpl, "</style>")
	if start < 0 || end < start {
		t.Fatal("theme template has no <style> block")
	}
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(tpl[start+len("<style>"):end], "")
	return regexp.MustCompile(`\s+`).ReplaceAllString(css, "")
}

// mediaBlocks returns the @media blocks in compacted css whose query is
// query, or every block when query is empty.
func mediaBlocks(css, query string) []cssBlock {
	var out []cssBlock
	for i := 0; ; {
		j := strings.Index(css[i:], "@media(")
		if j < 0 {
			return out
		}
		start := i + j
		open := strings.Index(css[start:], "{")
		if open < 0 {
			return out
		}
		q := css[start+len("@media(") : start+open-1]
		k, depth := start+open+1, 1
		for ; k < len(css) && depth > 0; k++ {
			switch css[k] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if query == "" || q == query {
			out = append(out, cssBlock{start: start, end: k, body: css[start+open+1 : k-1]})
		}
		i = k
	}
}

// The base .two-col rule takes its column count from --dash-cols, and a
// later 900px media query sets one column. With nothing inline, the media
// query wins on narrow screens and the Settings count applies above it.
func TestDashboardThemes_NarrowScreensGetOneColumn(t *testing.T) {
	for name, tpl := range map[string]string{"midnight": DashboardMidnight, "clean": DashboardClean} {
		t.Run(name, func(t *testing.T) {
			css := themeCSS(t, tpl)
			all := mediaBlocks(css, "")
			base := -1
			for i := 0; ; {
				j := strings.Index(css[i:], ".two-col{")
				if j < 0 {
					break
				}
				at, nested := i+j, false
				for _, b := range all {
					if at > b.start && at < b.end {
						nested = true
					}
				}
				if !nested {
					base = at
					break
				}
				i = at + 1
			}
			if base < 0 {
				t.Fatal("no top-level .two-col rule")
			}
			rule := css[base : base+strings.Index(css[base:], "}")]
			if !strings.Contains(rule, "grid-template-columns:repeat(var(--dash-cols") {
				t.Errorf(".two-col should take its column count from --dash-cols, got %s}", rule)
			}
			found := false
			for _, b := range mediaBlocks(css, "max-width:900px") {
				if b.start > base && strings.Contains(b.body, ".two-col{grid-template-columns:1fr}") {
					found = true
				}
			}
			if !found {
				t.Error("want a @media(max-width:900px) block after the .two-col rule that sets grid-template-columns:1fr")
			}
		})
	}
}

// One-column cards aren't enough on a phone: the header's nav row is
// about 500px wide, midnight's stats row overflowed up to about 850px, and
// midnight's three-column side padding cost 48px. These rules keep a
// 320-900px screen from scrolling sideways.
func TestDashboardThemes_NarrowScreensDontScrollSideways(t *testing.T) {
	cases := []struct {
		theme string
		tpl   string
		query string
		want  []string
	}{
		{"midnight", DashboardMidnight, "max-width:768px", []string{".header{flex-direction:column;", ".nav-links{flex-wrap:wrap}"}},
		{"clean", DashboardClean, "max-width:768px", []string{".header{flex-direction:column;", ".nav-links{flex-wrap:wrap}"}},
		{"midnight", DashboardMidnight, "max-width:900px", []string{".top-bar{flex-wrap:wrap;", ".container.dash-wide{padding:0}"}},
		{"clean", DashboardClean, "max-width:900px", []string{".top-bar{flex-wrap:wrap;"}},
	}
	for _, c := range cases {
		css := themeCSS(t, c.tpl)
		var bodies []string
		for _, b := range mediaBlocks(css, c.query) {
			bodies = append(bodies, b.body)
		}
		joined := strings.Join(bodies, "")
		for _, w := range c.want {
			if !strings.Contains(joined, w) {
				t.Errorf("%s: no @media(%s) rule contains %s", c.theme, c.query, w)
			}
		}
	}
}
