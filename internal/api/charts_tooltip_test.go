package api

import "testing"

// Each line, area and bar chart has a tooltip, a position:fixed div in
// <body>. Pages that rebuild their HTML replace their canvases (the disk
// page on a range button, the dashboard on a new scan), so the tooltips of
// the canvases they drop have to go too, or <body> gains a set per rebuild.
func TestChartJS_ReplacedCanvasesTakeTheirTooltips(t *testing.T) {
	runLayoutScenario(t, []string{"charts.js"}, `
  function tips() { return body.children.filter(function(n) { return /z-index:9999/.test(n.style.cssText || ""); }); }
  function shown() { return tips().filter(function(n) { return n.style.opacity === "1"; }); }
  function hover(cv) { cv.listeners.mousemove.forEach(function(fn) { fn({ clientX: 120, clientY: 40 }); }); }

  // A chart that stays on the page through the rebuilds.
  var side = new El("div"); body.appendChild(side);
  var kept = new El("canvas"); kept.id = "kept"; side.appendChild(kept);
  NasChart.line("kept", { datasets: [{ data: [1, 2, 3], label: "Kept" }], labels: ["a", "b", "c"] });

  // What the disk page does on a range button: new HTML with the same
  // canvas ids, then a chart on each canvas.
  var app = new El("div"); body.appendChild(app);
  var kinds = { chartTemp: "line", chartRealloc: "area", chartPending: "area", chartCrc: "line", chartCt: "bar" };
  function rebuild(round) {
    app.children.forEach(function(n) { n.remove(); });
    Object.keys(kinds).forEach(function(id) {
      var card = new El("div"); app.appendChild(card);
      var cv = new El("canvas"); cv.id = id; card.appendChild(cv);
      var label = id + " #" + round;
      NasChart[kinds[id]](id, kinds[id] === "bar"
        ? { data: [4, 6], labels: [label, "b"] }
        : { datasets: [{ data: [1, 3, 2], label: label }], labels: ["a", "b", "c"] });
    });
  }

  rebuild(0);
  check(tips().length === 6, "want a tooltip for each of the 6 charts, got " + tips().length);
  // The pointer is over a chart when the page rebuilds. No mouseleave
  // reaches a canvas that has left the page, so its tooltip stays showing.
  hover(document.getElementById("chartTemp"));
  check(shown().length === 1, "hovering a chart should show its tooltip, " + shown().length + " showing");
  for (var round = 1; round <= 3; round++) rebuild(round);
  check(tips().length === 6, "3 rebuilds should leave 6 tooltips, one per chart on the page, got " + tips().length);
  check(shown().length === 0, "the tooltip showing over a replaced chart should go with it, " + shown().length + " still showing");

  // Every chart on the page, new and kept, still shows its own tooltip.
  Object.keys(kinds).concat(["kept"]).forEach(function(id) {
    var cv = document.getElementById(id);
    var want = id === "kept" ? "Kept" : id + " #3";
    hover(cv);
    var on = shown();
    check(on.length === 1 && String(on[0].innerHTML).indexOf(want) >= 0,
      id + ": hovering should show one tooltip naming " + want + ", got " + JSON.stringify(on.map(function(n) { return n.innerHTML; })));
    cv.listeners.mouseleave.forEach(function(fn) { fn({}); });
    check(shown().length === 0, id + ": leaving the chart should hide its tooltip");
  });
`)
}
