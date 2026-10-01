package api

import "testing"

// The dashboard sections wrap text the host reports in util.esc, inside
// attribute values too: process command lines, finding titles, container
// and service check names. A quote in that text has to stay inside its
// attribute. Otherwise a process can add an event handler to the
// dashboard with a command line like `sleep 600 x" onmouseover="...`.
// These tests load DashboardJS in node with runSpeedLiveScenario from
// speed_live_panel_test.go and read the HTML the sections return the way
// a browser tokenizes it.

// escapeCheckJS reads the markup the sections emit the way a browser's
// tokenizer does. tags(html, name) returns each start tag's attribute
// names in order, their values with character references decoded,
// keeping the first of a repeated name, and the decoded text up to the
// next tag. No backticks: this lives in a Go raw string.
const escapeCheckJS = `
function decode(s) {
  var named = { amp: "&", lt: "<", gt: ">", quot: "\"", apos: "'", nbsp: " " };
  return s.replace(/&(#x[0-9a-f]+|#[0-9]+|[a-z]+);/gi, function(m, ref) {
    if (ref.charAt(0) !== "#") return Object.prototype.hasOwnProperty.call(named, ref) ? named[ref] : m;
    return String.fromCodePoint(ref.charAt(1) === "x" || ref.charAt(1) === "X" ? parseInt(ref.slice(2), 16) : parseInt(ref.slice(1), 10));
  });
}
function tags(html, name) {
  var out = [], re = new RegExp("<" + name + "(?=[\\s/>])", "gi"), m;
  while ((m = re.exec(html))) {
    var i = m.index + m[0].length, names = [], attrs = {};
    for (;;) {
      while (i < html.length && /[\s\/]/.test(html.charAt(i))) i++;
      if (i >= html.length || html.charAt(i) === ">") break;
      var start = i;
      while (i < html.length && !/[\s\/>=]/.test(html.charAt(i))) i++;
      var an = html.slice(start, i).toLowerCase(), av = "";
      while (i < html.length && /\s/.test(html.charAt(i))) i++;
      if (html.charAt(i) === "=") {
        i++;
        while (i < html.length && /\s/.test(html.charAt(i))) i++;
        var q = html.charAt(i);
        if (q === "\"" || q === "'") {
          var end = html.indexOf(q, i + 1);
          if (end < 0) end = html.length;
          av = html.slice(i + 1, end);
          i = end + 1;
        } else {
          start = i;
          while (i < html.length && !/[\s>]/.test(html.charAt(i))) i++;
          av = html.slice(start, i);
        }
      }
      if (names.indexOf(an) < 0) { names.push(an); attrs[an] = decode(av); }
    }
    var next = html.indexOf("<", i + 1);
    out.push({ names: names, attrs: attrs, text: decode(html.slice(i + 1, next < 0 ? html.length : next)) });
  }
  return out;
}
`

func TestDashboardJS_Esc_EscapesQuotes(t *testing.T) {
	runSpeedLiveScenario(t, true, `
  var esc = NasDashboard.util.esc;
  // [input, escaped]
  var cases = [
    ["plain text", "plain text"],
    ["a \"quoted\" word", "a &quot;quoted&quot; word"],
    ["it's", "it&#39;s"],
    ["<b>Tom & Jerry</b>", "&lt;b&gt;Tom &amp; Jerry&lt;/b&gt;"],
    ["&quot; stays as typed", "&amp;quot; stays as typed"],
    [0, "0"],
    [42, "42"],
    [null, ""],
    [undefined, ""],
    ["", ""]
  ];
  cases.forEach(function(c) {
    var got = esc(c[0]);
    check(got === c[1], "esc(" + JSON.stringify(c[0]) + ") should be " + JSON.stringify(c[1]) + ", got " + JSON.stringify(got));
  });
`)
}

// The Top Processes link keeps the whole command line in its title and
// shows its first 40 characters, whatever quotes or markup it holds.
func TestDashboardJS_TopProcesses_CommandStaysInTitle(t *testing.T) {
	runSpeedLiveScenario(t, true, escapeCheckJS+`
  var cmds = [
    "sleep 600 x\" onmouseover=\"window.hit=1",
    "sleep 600 x' onmouseover='window.hit=1",
    "<img src=x onerror=window.hit=1>",
    "sh -c \"while true; do echo \\\"tick\\\" >> /tmp/log & sleep 1; done\""
  ];
  var sn = { system: { top_processes: cmds.map(function(c) { return { command: c, cpu_percent: 1, mem_percent: 1, user: "root" }; }) } };
  var html = NasDashboard.sections.processes(sn);
  var links = tags(html, "a").filter(function(a) { return (a.attrs.href || "").indexOf("/stats?process=") === 0; });
  check(links.length === cmds.length, "want one process link per command, got " + links.length);
  links.forEach(function(a, i) {
    var label = "command " + JSON.stringify(cmds[i]) + ": ";
    check(a.names.join(" ") === "href style title", label + "the link should only have href, style and title, got " + a.names.join(" "));
    check(a.attrs.title === cmds[i], label + "the title should be the whole command, got " + JSON.stringify(a.attrs.title));
    var shown = cmds[i].length > 40 ? cmds[i].substring(0, 40) + "…" : cmds[i];
    check(a.text === shown, label + "the link should show " + JSON.stringify(shown) + ", got " + JSON.stringify(a.text));
  });
  check(tags(html, "img").length === 0, "a command line should not add an element");
`)
}

// Dismiss hands the finding's own title to _dismissFinding. Container
// findings put the name in single quotes, as in "High CPU: Container
// 'plex' (95%)", so the title has to survive quotes and backslashes
// inside an onclick.
func TestDashboardJS_FindingDismiss_PassesTheTitle(t *testing.T) {
	runSpeedLiveScenario(t, true, escapeCheckJS+`
  var titles = [
    "High CPU: Container 'plex' (95%)",
    "SMART Health FAILED: sdb (WDC \"Red\" 4TB)",
    "x');window.hit=1;('",
    "x\" onmouseover=\"window.hit=1",
    "back\\slash \\' & </a>"
  ];
  var sn = { findings: titles.map(function(t) { return { severity: "warning", title: t, description: "d" }; }) };
  var html = NasDashboard.sections.findings(sn, { dismissed_findings: [] });

  var shown = tags(html, "span").filter(function(s) { return s.attrs["class"] === "finding-title"; }).map(function(s) { return s.text; });
  check(JSON.stringify(shown) === JSON.stringify(titles), "each finding should show its title, got " + JSON.stringify(shown));

  var dismissed = [];
  window._dismissFinding = function(title) { dismissed.push(title); };
  var links = tags(html, "a").filter(function(a) { return a.text === "Dismiss"; });
  check(links.length === titles.length, "want one Dismiss link per finding, got " + links.length);
  links.forEach(function(a, i) {
    var label = "title " + JSON.stringify(titles[i]) + ": ";
    check(a.names.join(" ") === "href onclick style", label + "Dismiss should only have href, onclick and style, got " + a.names.join(" "));
    var stopped = false;
    try {
      var ret = new Function("event", a.attrs.onclick)({ stopPropagation: function() { stopped = true; } });
      check(stopped && ret === false, label + "the onclick should stop the click and return false");
    } catch (e) {
      check(false, label + "the onclick throws " + e);
    }
  });
  check(JSON.stringify(dismissed) === JSON.stringify(titles), "Dismiss should pass each title as it is, got " + JSON.stringify(dismissed));
  check(window.hit === undefined, "a finding title ran script");
`)
}

// The K8s events list cuts a message to its first 100 characters before
// escaping it, so the cut can't land inside &quot; or &amp;.
func TestDashboardJS_K8sEvents_MessageCutBeforeEscaping(t *testing.T) {
	runSpeedLiveScenario(t, true, escapeCheckJS+`
  var msg = "Failed to pull image \"registry.local/app:v2\": rpc error: code = NotFound & desc = <manifest unknown> for \"linux/arm64\", retrying";
  var sn = { kubernetes: { connected: true, events: [{ type: "Warning", reason: "Failed", object: "Pod/web-7d4b9c", message: msg }] } };
  var html = NasDashboard.sections.kubernetes(sn);
  var texts = tags(html, "span").map(function(s) { return s.text; });
  var want = msg.substring(0, 100);
  check(texts.indexOf(want) >= 0, "the event should show the first 100 characters of its message, " + JSON.stringify(want) + ", got " + JSON.stringify(texts));
`)
}
