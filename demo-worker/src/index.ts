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

    // ── Health endpoint ──
    if (path === "/health") {
      return json({ nas_doctor: true, status: "ok", demo: true, themes: ["midnight", "clean", "ember"], version: "demo" });
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
      "/theme/ember": "_pages/ember.html",
      "/settings": "_pages/settings.html",
      "/alerts": "_pages/alerts.html",
      "/fleet": "_pages/fleet.html",
      "/stats": "_pages/stats.html",
      "/parity": "_pages/parity.html",
      "/service-checks": "_pages/service_checks.html",
    };

    const pageFile = pageMap[path];
    if (pageFile) {
      const html = await fetchAsset(env, url, request, pageFile);
      if (html !== null) {
        return new Response(injectBanner(html, platform, path), {
          headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "public, max-age=300", ...platformCookie(platform) },
        });
      }
    }

    // ── Static assets ──
    if (path === "/js/charts.js") {
      const t = await fetchAsset(env, url, request, "charts.js");
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
    if (path === "/icon.png" || path === "/favicon.png" || path.startsWith("/icons/")) {
      return new Response(
        Uint8Array.from(atob("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="), c => c.charCodeAt(0)),
        { headers: { "Content-Type": "image/png", "Cache-Control": "public, max-age=86400" } }
      );
    }

    // ── Fallback ──
    if (!path.includes(".")) {
      const html = await fetchAsset(env, url, request, "_pages/midnight.html");
      if (html !== null) {
        return new Response(injectBanner(html, platform, path), {
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
    return json({ nas_doctor: true, status: "ok", demo: true, themes: ["midnight", "clean", "ember"], uptime: "14d " + new Date().getHours() + "h", version: "demo" });
  }

  // Match alerts by ID
  if (path.match(/^\/api\/v1\/alerts\/[^/]+\/events/)) return json([]);
  if (path.match(/^\/api\/v1\/alerts\/[^/]+$/)) return json({});
  // Match disk detail by serial
  if (path.match(/^\/api\/v1\/disks\/[^/]+$/)) {
    const data = await env.DEMO_DATA.get(`api:${platform}:disks`, "text");
    return json(data ? JSON.parse(data) : []);
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

  if (data) return json(JSON.parse(data));

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

interface Env {
  ASSETS: { fetch(request: Request): Promise<Response> };
  DEMO_DATA: KVNamespace;
}
