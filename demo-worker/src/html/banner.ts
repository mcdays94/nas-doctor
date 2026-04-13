/**
 * Platform switcher banner injected at the top of every HTML page.
 * Allows users to switch between Unraid, Synology, Proxmox, and Kubernetes demo profiles.
 */

import { Platform } from "../data/platforms";

const PLATFORMS: { key: Platform; label: string; icon: string }[] = [
  { key: "unraid", label: "Unraid", icon: "💾" },
  { key: "synology", label: "Synology", icon: "📦" },
  { key: "truenas", label: "TrueNAS", icon: "🐠" },
  { key: "proxmox", label: "Proxmox VE", icon: "🖥️" },
  { key: "kubernetes", label: "Kubernetes", icon: "☸️" },
];

function bannerHTML(currentPlatform: Platform, currentPath: string): string {
  const isSettings = currentPath === "/settings";

  const buttons = PLATFORMS.map((p) => {
    const active = p.key === currentPlatform;
    return `<a href="#" onclick="document.cookie='nas_demo_platform=${p.key};path=/;max-age=86400;SameSite=Lax';window.location.href=window.location.pathname+'?platform=${p.key}&_='+Date.now();return false" class="dp-btn${active ? " dp-active" : ""}">${p.icon} ${p.label}</a>`;
  }).join("");

  return `
<div id="nas-demo-banner">
  <style>
    #nas-demo-banner {
      position: sticky; top: 0; z-index: 9999;
      background: linear-gradient(135deg, #1e293b 0%, #0f172a 100%);
      border-bottom: 1px solid rgba(99,102,241,0.3);
      padding: 10px 16px;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      font-size: 13px;
      color: #e2e8f0;
      display: flex;
      align-items: center;
      justify-content: space-between;
      flex-wrap: wrap;
      gap: 8px;
    }
    #nas-demo-banner .dp-left {
      display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
    }
    #nas-demo-banner .dp-badge {
      background: linear-gradient(135deg, #6366f1, #8b5cf6);
      color: white; font-weight: 700; font-size: 11px;
      padding: 3px 10px; border-radius: 4px; text-transform: uppercase;
      letter-spacing: 0.5px; white-space: nowrap;
    }
    #nas-demo-banner .dp-label {
      color: #94a3b8; font-size: 12px; white-space: nowrap;
    }
    #nas-demo-banner .dp-btns {
      display: flex; gap: 4px; flex-wrap: wrap;
    }
    #nas-demo-banner .dp-btn {
      padding: 5px 12px; border-radius: 6px; font-size: 12px;
      font-weight: 500; text-decoration: none; color: #94a3b8;
      background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.08);
      transition: all 0.15s; white-space: nowrap; cursor: pointer;
    }
    #nas-demo-banner .dp-btn:hover {
      background: rgba(255,255,255,0.1); color: #e2e8f0;
      border-color: rgba(255,255,255,0.15);
    }
    #nas-demo-banner .dp-active {
      background: rgba(99,102,241,0.2) !important;
      color: #a5b4fc !important;
      border-color: rgba(99,102,241,0.4) !important;
      font-weight: 600;
    }
    #nas-demo-banner .dp-right {
      display: flex; align-items: center; gap: 10px;
    }
    #nas-demo-banner .dp-gh {
      color: #94a3b8; text-decoration: none; font-size: 12px;
      display: flex; align-items: center; gap: 4px;
      transition: color 0.15s;
    }
    #nas-demo-banner .dp-gh:hover { color: #e2e8f0; }
    #nas-demo-banner .dp-ro {
      color: #f59e0b; font-size: 11px; font-weight: 500;
      background: rgba(245,158,11,0.1); padding: 2px 8px;
      border-radius: 4px; white-space: nowrap;
    }
    @media (max-width: 640px) {
      #nas-demo-banner { flex-direction: column; align-items: flex-start; }
      #nas-demo-banner .dp-right { width: 100%; justify-content: space-between; }
    }
  </style>
  <div class="dp-left">
    <span class="dp-badge">Live Demo</span>
    <span class="dp-label">Platform:</span>
    <div class="dp-btns">${buttons}</div>
  </div>
  <div class="dp-right">
    <span class="dp-ro">Read-Only</span>
    <a class="dp-gh" href="https://github.com/mcdays94/nas-doctor" target="_blank" rel="noopener">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
      GitHub
    </a>
  </div>
</div>`;
}

/**
 * Inject the platform switcher banner into an HTML page.
 * Also greys out sensitive settings inputs in the settings page.
 */
export function injectBanner(html: string, platform: Platform, path: string): string {
  const banner = bannerHTML(platform, path);

  // Inject after <body> tag
  const bodyIdx = html.indexOf("<body");
  if (bodyIdx !== -1) {
    const closeIdx = html.indexOf(">", bodyIdx);
    if (closeIdx !== -1) {
      html = html.slice(0, closeIdx + 1) + banner + html.slice(closeIdx + 1);
    }
  } else {
    // No <body> tag — prepend
    html = banner + html;
  }

  // For settings page: inject script to disable sensitive inputs
  if (path === "/settings") {
    const disableScript = `
<script>
document.addEventListener('DOMContentLoaded', function() {
  // Grey out API key, webhooks, fleet, proxmox/k8s connection fields
  var selectors = [
    '#api-key-display', '#api-key-btn',
    '[id*="webhook"]', '[id*="fleet"]',
    '#pve-url', '#pve-user', '#pve-pass', '#pve-token',
    '#k8s-url', '#k8s-token', '#k8s-kubeconfig',
    '#ret-snapshot-days', '#ret-notify-days',
    '[onclick*="testWebhook"]', '[onclick*="testPVE"]', '[onclick*="testK8s"]',
    '[onclick*="runServiceChecks"]', '[onclick*="createBackup"]'
  ];
  setTimeout(function() {
    selectors.forEach(function(sel) {
      var els = document.querySelectorAll(sel);
      els.forEach(function(el) {
        el.style.opacity = '0.4';
        el.style.pointerEvents = 'none';
        el.setAttribute('disabled', 'true');
        el.setAttribute('title', 'Disabled in demo mode');
      });
    });
    // Add demo notice to settings
    var firstCard = document.querySelector('.card');
    if (firstCard) {
      var notice = document.createElement('div');
      notice.style.cssText = 'background:rgba(245,158,11,0.1);border:1px solid rgba(245,158,11,0.3);border-radius:8px;padding:10px 14px;margin-bottom:16px;font-size:12px;color:#f59e0b;text-align:center';
      notice.textContent = 'This is a read-only demo. Settings changes are not saved.';
      firstCard.parentNode.insertBefore(notice, firstCard);
    }
  }, 500);
});
</script>`;
    html = html.replace("</body>", disableScript + "</body>");
  }

  return html;
}
