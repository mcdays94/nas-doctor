/**
 * NAS Doctor Live Demo — Cloudflare Worker
 *
 * Serves the real NAS Doctor dashboard (captured from Go binary at build time)
 * with API data stored in KV (seeded from real binary + refreshed by feeder cron).
 *
 * The Worker itself has ZERO hardcoded mock data. All API responses come from KV,
 * which contains data in the exact format the real Go app produces.
 *
 * Platform switcher: ?platform=unraid|synology|proxmox|kubernetes
 */

import { Platform, getPlatformFromRequest } from "./data/platforms";
import { injectBanner } from "./html/banner";

export default {
  async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname;
    const method = request.method;
    const platform = getPlatformFromRequest(request, url);

    // ── Block ALL write requests ──
    if (method !== "GET" && method !== "HEAD" && method !== "OPTIONS") {
      // Graceful no-ops for UI controls that fire PUTs
      if (path === "/api/v1/settings/chart-range") return json({ chart_range_hours: 1 });
      if (path === "/api/v1/settings/section-heights") return json({});
      if (path === "/api/v1/findings/dismiss" || path === "/api/v1/findings/restore") return json({ ok: true });
      return json({ error: "This is a read-only demo. Write operations are disabled." }, 403);
    }

    if (method === "OPTIONS") {
      return new Response(null, {
        headers: { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Methods": "GET, OPTIONS", "Access-Control-Allow-Headers": "Content-Type" },
      });
    }

    // ── API routes: read from KV ──
    if (path.startsWith("/api/v1/")) {
      return handleAPI(path, url, platform, env);
    }

    // ── Version from deploy-time env var ──
    const version = env.VERSION || "demo";

    // ── Health endpoint ──
    if (path === "/health") {
      return json({ nas_doctor: true, status: "ok", demo: true, themes: ["midnight", "clean"], version });
    }

    // ── Prometheus metrics ──
    if (path === "/metrics") {
      return new Response("# NAS Doctor Demo - metrics disabled\n", { headers: { "Content-Type": "text/plain" } });
    }

    // ── HTML pages: serve from static assets with banner injection ──
    const pageMap: Record<string, string> = {
      "/": "_pages/midnight.html",
      "/theme/midnight": "_pages/midnight.html",
      "/theme/clean": "_pages/clean.html",
      "/settings": "_pages/settings.html",
      "/alerts": "_pages/alerts.html",
      "/fleet": "_pages/fleet.html",
      "/stats": "_pages/stats.html",
      "/parity": "_pages/parity.html",
      "/service-checks": "_pages/service_checks.html",
      "/replacement-planner": "_pages/replacement_planner.html",
    };

    const pageFile = pageMap[path];
    if (pageFile) {
      const html = await fetchAsset(env, url, request, pageFile);
      if (html !== null) {
        return new Response(injectBanner(html, platform, path, version), {
          headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-cache", ...platformCookie(platform) },
        });
      }
    }

    // ── Static assets ──
    if (path === "/js/charts.js") {
      const t = await fetchAsset(env, url, request, "charts.js");
      if (t !== null) return new Response(t, { headers: { "Content-Type": "application/javascript", "Cache-Control": "public, max-age=3600" } });
    }
    if (path === "/js/dashboard.js") {
      const t = await fetchAsset(env, url, request, "dashboard.js");
      if (t !== null) return new Response(t, { headers: { "Content-Type": "application/javascript", "Cache-Control": "public, max-age=3600" } });
    }
    if (path === "/css/shared.css") {
      const t = await fetchAsset(env, url, request, "shared.css");
      if (t !== null) return new Response(t, { headers: { "Content-Type": "text/css", "Cache-Control": "public, max-age=3600" } });
    }
    if (path === "/api/v1/report") {
      const t = await fetchAsset(env, url, request, "_pages/report.html");
      if (t !== null) return new Response(t, { headers: { "Content-Type": "text/html; charset=utf-8" } });
    }
    // Serve the NAS Doctor logo from the captured assets (see #262).
    // demo-deploy.yml curls /icon.png + /icons/icon{1,2,3}.png from
    // the live binary during capture; this path delivers those bytes.
    // Before #262 this branch returned a 1×1 transparent PNG which
    // rendered as an invisible gap next to the \"NAS Doctor\" title.
    // /favicon.png is aliased to /icon.png for browser tab icons.
    if (path === "/icon.png" || path === "/favicon.png") {
      const resp = await fetchAssetResponse(env, url, request, "icon.png");
      if (resp) return resp;
    }
    if (path.startsWith("/icons/") && path.endsWith(".png")) {
      // path is /icons/iconN.png → strip leading slash so fetchAssetResponse
      // can resolve it against the captured/ asset root.
      const assetPath = path.slice(1);
      const resp = await fetchAssetResponse(env, url, request, assetPath);
      if (resp) return resp;
    }

    // ── Fallback ──
    if (!path.includes(".")) {
      const html = await fetchAsset(env, url, request, "_pages/midnight.html");
      if (html !== null) {
        return new Response(injectBanner(html, platform, path, version), {
          headers: { "Content-Type": "text/html; charset=utf-8", ...platformCookie(platform) },
        });
      }
    }

    return new Response("Not Found", { status: 404 });
  },
} satisfies ExportedHandler<Env>;

// ── API handler: reads everything from KV ──
async function handleAPI(path: string, url: URL, platform: Platform, env: Env): Promise<Response> {
  // Map API paths to KV keys
  const kvMap: Record<string, string> = {
    "/api/v1/health": "_health",
    "/api/v1/status": "status",
    "/api/v1/settings": "settings",
    "/api/v1/snapshot/latest": "snapshot",
    "/api/v1/snapshots": "_empty_array",
    "/api/v1/sparklines": "sparklines",
    "/api/v1/history/gpu": "gpu_history",
    "/api/v1/history/containers": "container_history",
    "/api/v1/history/system": "system_history",
    "/api/v1/history/processes": "process_history",
    "/api/v1/history/speedtest": "speedtest_history",
    "/api/v1/alerts": "alerts",
    "/api/v1/service-checks": "service_checks",
    "/api/v1/service-checks/history": "service_checks", // reuse full list as history
    "/api/v1/fleet": "fleet",
    "/api/v1/fleet/servers": "fleet_servers",
    "/api/v1/disks": "disks",
    "/api/v1/smart/trends": "smart_trends",
    "/api/v1/incidents/timeline": "incidents",
    "/api/v1/incidents/correlation": "_empty_array",
    "/api/v1/notifications/log": "notifications_log",
    "/api/v1/db/stats": "db_stats",
    "/api/v1/replacement-plan": "replacement_plan",
    "/api/v1/capacity-forecast": "capacity_forecast",
    "/api/v1/disk-usage-history": "_empty_array",
    "/api/v1/backup": "_empty_array",
    "/api/v1/icons": "_icons",
  };

  // Health is always generated live
  if (path === "/api/v1/health") {
    return json({ nas_doctor: true, status: "ok", demo: true, themes: ["midnight", "clean"], uptime: "14d " + new Date().getHours() + "h", version: env.VERSION || "demo" });
  }

  // Match alerts by ID
  if (path.match(/^\/api\/v1\/alerts\/[^/]+\/events/)) return json([]);
  if (path.match(/^\/api\/v1\/alerts\/[^/]+$/)) return json({});
  // Match disk detail by serial
  if (path.match(/^\/api\/v1\/disks\/[^/]+$/)) {
    const data = await env.DEMO_DATA.get(`api:${platform}:disks`, "text");
    return json(data ? JSON.parse(data) : []);
  }
  // PRD #283 slice 3 / issue #286 — per-sample telemetry for the
  // /service-checks expanded log row's mini-chart. The demo carries
  // one canonical dataset per platform (the feeder synthesises it
  // under api:<platform>:speedtest_samples), so the test_id is
  // ignored for routing and trusted for the round-trip echo.
  if (path.match(/^\/api\/v1\/speedtest\/samples\/[^/]+$/)) {
    const data = await env.DEMO_DATA.get(`api:${platform}:speedtest_samples`, "text");
    if (data) return json(JSON.parse(data));
    return json({ test_id: 0, samples: [], count: 0 });
  }

  const kvKey = kvMap[path];
  if (!kvKey) return json({ error: "not found" }, 404);

  // Special hardcoded responses
  if (kvKey === "_empty_array") return json([]);
  if (kvKey === "_icons") return json({ icons: ["default", "blue", "green", "red"], current: "default" });

  // Read from KV: try platform-specific key first, then fallback to unraid (base data)
  let data = await env.DEMO_DATA.get(`api:${platform}:${kvKey}`, "text");
  if (!data) {
    data = await env.DEMO_DATA.get(`api:unraid:${kvKey}`, "text");
  }

  if (data) {
    let parsed = JSON.parse(data);

    // Patch settings: force theme to midnight (subpages read theme from settings)
    if (kvKey === "settings" && parsed.theme) {
      parsed.theme = "midnight";
    }

    return json(parsed);
  }

  // Final fallback: empty
  return json(path.includes("history") || path.includes("alerts") || path.includes("fleet") ? [] : {});
}

// ── Helpers ──
function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", "Access-Control-Allow-Origin": "*" },
  });
}

function platformCookie(platform: Platform): Record<string, string> {
  return { "Set-Cookie": `nas_demo_platform=${platform}; Path=/; Max-Age=86400; SameSite=Lax` };
}

async function fetchAsset(env: Env, baseUrl: URL, request: Request, filename: string): Promise<string | null> {
  const attempts = [`${baseUrl.origin}/${filename}`, `https://fake-host/${filename}`];
  for (const u of attempts) {
    try {
      const resp = await env.ASSETS.fetch(new Request(u, { method: "GET", headers: request.headers }));
      if (resp.ok) return await resp.text();
    } catch { /* try next */ }
  }
  return null;
}

// fetchAssetResponse — binary-safe variant of fetchAsset. Returns the
// raw Response (preserving Content-Type + binary body) rather than
// decoding as text. Used for PNG icons and anything else that isn't
// guaranteed to be UTF-8. Added for #262.
async function fetchAssetResponse(env: Env, baseUrl: URL, request: Request, filename: string): Promise<Response | null> {
  const attempts = [`${baseUrl.origin}/${filename}`, `https://fake-host/${filename}`];
  for (const u of attempts) {
    try {
      const resp = await env.ASSETS.fetch(new Request(u, { method: "GET", headers: request.headers }));
      if (resp.ok) {
        // Re-emit so we can add Cache-Control + CORS. Cloudflare's
        // ASSETS binding already sets Content-Type from the file
        // extension; we preserve it by copying headers.
        const headers = new Headers(resp.headers);
        headers.set("Cache-Control", "public, max-age=86400");
        return new Response(resp.body, { status: resp.status, headers });
      }
    } catch { /* try next */ }
  }
  return null;
}

interface Env {
  ASSETS: { fetch(request: Request): Promise<Response> };
  DEMO_DATA: KVNamespace;
  VERSION: string;
}
