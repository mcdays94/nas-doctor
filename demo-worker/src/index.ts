/**
 * NAS Doctor Live Demo — Cloudflare Worker
 *
 * Serves the NAS Doctor dashboard with dynamically generated demo data.
 * All write endpoints are blocked. Data varies over time using deterministic
 * functions seeded by Date.now() so charts and metrics look live.
 *
 * Platform switcher: ?platform=unraid|synology|proxmox|kubernetes
 * Stored in a cookie so it persists across page loads.
 */

import { generateStatus } from "./api/status";
import { generateSnapshot } from "./api/snapshot";
import { generateSparklines } from "./api/sparklines";
import { generateGPUHistory, generateContainerHistory, generateSystemHistory } from "./api/history";
import { generateSettings } from "./api/settings";
import { generateAlerts } from "./api/alerts";
import { generateServiceChecks } from "./api/service-checks";
import { generateFleet } from "./api/fleet";
import { generateDisks, generateDiskDetail } from "./api/disks";
import { generateSmartTrends } from "./api/smart";
import { generateIncidents } from "./api/incidents";
import { generateMisc, generateNotificationLog } from "./api/misc";
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
      // Special case: return success for chart-range and section-heights so UI doesn't error
      if (path === "/api/v1/settings/chart-range") {
        return json({ chart_range_hours: 1 });
      }
      if (path === "/api/v1/settings/section-heights") {
        return json({});
      }
      if (path === "/api/v1/findings/dismiss" || path === "/api/v1/findings/restore") {
        return json({ ok: true });
      }
      return json(
        { error: "This is a read-only demo. Write operations are disabled." },
        403
      );
    }

    // ── CORS preflight ──
    if (method === "OPTIONS") {
      return new Response(null, {
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, OPTIONS",
          "Access-Control-Allow-Headers": "Content-Type",
        },
      });
    }

    // ── API routes ──
    if (path.startsWith("/api/v1/")) {
      return handleAPI(path, url, platform);
    }

    // ── Health endpoint (always public) ──
    if (path === "/api/v1/health" || path === "/health") {
      return json({
        nas_doctor: true,
        status: "ok",
        demo: true,
        themes: ["midnight", "clean", "ember"],
        uptime: formatUptime(),
        version: "demo",
      });
    }

    // ── Prometheus metrics ──
    if (path === "/metrics") {
      return new Response("# NAS Doctor Demo - metrics endpoint disabled in demo mode\n", {
        headers: { "Content-Type": "text/plain" },
      });
    }

    // ── HTML pages: serve from static assets with banner injection ──
    // Map page routes to captured filenames
    const pageMap: Record<string, string> = {
      "/": "midnight.html",
      "/theme/midnight": "midnight.html",
      "/theme/clean": "clean.html",
      "/theme/ember": "ember.html",
      "/settings": "settings.html",
      "/alerts": "alerts.html",
      "/fleet": "fleet.html",
      "/stats": "stats.html",
      "/parity": "parity.html",
      "/service-checks": "service_checks.html",
    };

    const pageFile = pageMap[path];
    if (pageFile) {
      const html = await fetchAsset(env, url, request, pageFile);
      if (html !== null) {
        const injected = injectBanner(html, platform, path);
        return new Response(injected, {
          headers: {
            "Content-Type": "text/html; charset=utf-8",
            "Cache-Control": "public, max-age=300",
            ...platformCookie(platform),
          },
        });
      }
    }

    // ── Static assets: charts.js, shared.css ──
    if (path === "/js/charts.js") {
      const text = await fetchAsset(env, url, request, "charts.js");
      if (text !== null) return new Response(text, { headers: { "Content-Type": "application/javascript", "Cache-Control": "public, max-age=3600" } });
    }
    if (path === "/css/shared.css") {
      const text = await fetchAsset(env, url, request, "shared.css");
      if (text !== null) return new Response(text, { headers: { "Content-Type": "text/css", "Cache-Control": "public, max-age=3600" } });
    }

    // ── Report page ──
    if (path === "/api/v1/report") {
      const text = await fetchAsset(env, url, request, "report.html");
      if (text !== null) return new Response(text, { headers: { "Content-Type": "text/html; charset=utf-8" } });
    }

    // ── Favicon / icon ──
    if (path === "/icon.png" || path === "/favicon.png" || path.startsWith("/icons/")) {
      // Return a minimal 1x1 transparent PNG
      return new Response(
        Uint8Array.from(atob("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="), c => c.charCodeAt(0)),
        { headers: { "Content-Type": "image/png", "Cache-Control": "public, max-age=86400" } }
      );
    }

    // ── Fallback: serve index as default ──
    if (!path.includes(".")) {
      const html = await fetchAsset(env, url, request, "midnight.html");
      if (html !== null) {
        const injected = injectBanner(html, platform, path);
        return new Response(injected, { headers: { "Content-Type": "text/html; charset=utf-8", ...platformCookie(platform) } });
      }
    }

    return new Response("Not Found", { status: 404 });
  },
} satisfies ExportedHandler<Env>;

/** Fetch a static asset from the ASSETS binding, trying multiple URL formats */
async function fetchAsset(env: Env, baseUrl: URL, request: Request, filename: string): Promise<string | null> {
  // Try fetching with the asset binding
  const attempts = [
    `${baseUrl.origin}/${filename}`,
    `https://fake-host/${filename}`,
  ];
  for (const u of attempts) {
    try {
      const resp = await env.ASSETS.fetch(new Request(u, { method: "GET", headers: request.headers }));
      if (resp.ok) return await resp.text();
    } catch { /* try next */ }
  }
  return null;
}

// ── API handler ──
function handleAPI(path: string, url: URL, platform: Platform): Response {
  const hours = parseInt(url.searchParams.get("hours") || "1");

  // Status & config
  if (path === "/api/v1/health") return json({ nas_doctor: true, status: "ok", demo: true, themes: ["midnight", "clean", "ember"], uptime: formatUptime(), version: "demo" });
  if (path === "/api/v1/status") return json(generateStatus(platform));
  if (path === "/api/v1/settings") return json(generateSettings(platform));
  if (path === "/api/v1/snapshot/latest") return json(generateSnapshot(platform));
  if (path === "/api/v1/snapshots") return json([]);

  // History / charts
  if (path === "/api/v1/sparklines") return json(generateSparklines(platform));
  if (path === "/api/v1/history/gpu") return json(generateGPUHistory(hours));
  if (path === "/api/v1/history/containers") return json(generateContainerHistory(platform, hours));
  if (path === "/api/v1/history/system") return json(generateSystemHistory());

  // Alerts & incidents
  if (path === "/api/v1/alerts") return json(generateAlerts(platform));
  if (path.match(/^\/api\/v1\/alerts\/[^/]+\/events/)) return json([]);
  if (path.match(/^\/api\/v1\/alerts\/[^/]+$/)) return json({});
  if (path === "/api/v1/incidents/timeline") return json(generateIncidents(platform));
  if (path === "/api/v1/incidents/correlation") return json([]);

  // Service checks
  if (path === "/api/v1/service-checks") return json(generateServiceChecks());
  if (path === "/api/v1/service-checks/history") return json([]);

  // Disks
  if (path === "/api/v1/disks") return json(generateDisks(platform));
  if (path.match(/^\/api\/v1\/disks\/[^/]+$/)) return json(generateDiskDetail(platform));
  if (path === "/api/v1/smart/trends") return json(generateSmartTrends(platform));
  if (path === "/api/v1/replacement-plan") return json(generateMisc("replacement-plan", platform));
  if (path === "/api/v1/capacity-forecast") return json(generateMisc("capacity-forecast", platform));
  if (path === "/api/v1/disk-usage-history") return json([]);

  // Fleet
  if (path === "/api/v1/fleet") return json(generateFleet());
  if (path === "/api/v1/fleet/servers") return json([]);

  // Notifications & DB
  if (path === "/api/v1/notifications/log") return json(generateNotificationLog());
  if (path === "/api/v1/db/stats") return json({ size_bytes: 4_521_984, snapshots: 78, oldest: new Date(Date.now() - 30 * 86400000).toISOString() });
  if (path === "/api/v1/backup") return json([]);
  if (path === "/api/v1/icons") return json({ icons: ["default", "blue", "green", "red"], current: "default" });

  return json({ error: "not found" }, 404);
}

// ── Helpers ──
function json(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", "Access-Control-Allow-Origin": "*" },
  });
}

function formatUptime(): string {
  // Pretend the server has been up for 14 days
  const days = 14;
  const hours = Math.floor((Date.now() / 3600000) % 24);
  return `${days}d ${hours}h`;
}

function platformCookie(platform: Platform): Record<string, string> {
  return { "Set-Cookie": `nas_demo_platform=${platform}; Path=/; Max-Age=86400; SameSite=Lax` };
}

interface Env {
  ASSETS: { fetch(request: Request): Promise<Response> };
}
