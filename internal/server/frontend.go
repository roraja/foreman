package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// FrontendFS holds the embedded frontend assets. Set by main.go before creating the server.
// When empty (zero value), the inline fallback frontend is served.
var FrontendFS embed.FS

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	// Try to serve from embedded frontend if available
	subFS, err := fs.Sub(FrontendFS, "frontend/dist")
	if err == nil {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if data, readErr := fs.ReadFile(subFS, path); readErr == nil && len(data) > 100 {
			http.ServeFileFS(w, r, subFS, path)
			return
		}
		// Try index.html for SPA fallback
		if path != "index.html" {
			if data, readErr := fs.ReadFile(subFS, "index.html"); readErr == nil && len(data) > 100 {
				http.ServeFileFS(w, r, subFS, "index.html")
				return
			}
		}
	}

	// Fallback: serve inline HTML
	s.serveInlineFrontend(w, r)
}

func (s *Server) serveInlineFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(inlineHTML))
}

// inlineHTML is a self-contained fallback UI used when no compiled frontend is embedded.
const inlineHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Foreman</title>
<style>
  :root { --bg: #0d1117; --surface: #161b22; --surface-hover: #1c2129; --border: #30363d; --text: #e6edf3; --muted: #8b949e; --green: #3fb950; --red: #f85149; --yellow: #d29922; --blue: #58a6ff; --purple: #bc8cff; --orange: #f0883e; }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); min-height: 100vh; }

  /* Header */
  .header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 10px 24px; display: flex; align-items: center; justify-content: space-between; position: sticky; top: 0; z-index: 50; }
  .header-left { display: flex; align-items: center; gap: 16px; }
  .header h1 { font-size: 18px; font-weight: 600; }
  .header h1 span { color: var(--blue); }
  .header-stats { display: flex; gap: 10px; font-size: 12px; }
  .stat-pill { padding: 3px 10px; border-radius: 12px; font-weight: 500; }
  .stat-running { background: rgba(63,185,80,0.15); color: var(--green); }
  .stat-stopped { background: rgba(139,148,158,0.15); color: var(--muted); }
  .stat-error { background: rgba(248,81,73,0.15); color: var(--red); }
  .toolbar { display: flex; gap: 6px; }

  /* Buttons */
  .btn { padding: 6px 14px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); cursor: pointer; font-size: 13px; transition: all 0.15s; display: inline-flex; align-items: center; gap: 6px; position: relative; }
  .btn:hover:not(:disabled) { background: var(--surface-hover); }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-green { border-color: var(--green); color: var(--green); }
  .btn-green:hover:not(:disabled) { background: rgba(63,185,80,0.1); }
  .btn-red { border-color: var(--red); color: var(--red); }
  .btn-red:hover:not(:disabled) { background: rgba(248,81,73,0.1); }
  .btn-blue { border-color: var(--blue); color: var(--blue); }
  .btn-blue:hover:not(:disabled) { background: rgba(88,166,255,0.1); }
  .btn-sm { padding: 4px 10px; font-size: 12px; }

  /* Spinner */
  .spinner { width: 14px; height: 14px; border: 2px solid transparent; border-top-color: currentColor; border-radius: 50%; animation: spin 0.6s linear infinite; display: inline-block; }
  .spinner-sm { width: 10px; height: 10px; border-width: 1.5px; }
  @keyframes spin { to { transform: rotate(360deg); } }

  /* Container */
  .container { max-width: 1200px; margin: 0 auto; padding: 20px; }

  /* Login */
  .login { display: flex; align-items: center; justify-content: center; min-height: 80vh; }
  .login-box { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 32px; width: 360px; }
  .login-box h2 { margin-bottom: 20px; font-size: 20px; }
  .login-box input { width: 100%; padding: 10px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 14px; margin-bottom: 16px; outline: none; }
  .login-box input:focus { border-color: var(--blue); }
  .login-box button { width: 100%; padding: 10px; border: none; border-radius: 6px; background: var(--blue); color: #fff; font-size: 14px; cursor: pointer; font-weight: 600; }
  .login-box .login-error { color: var(--red); font-size: 13px; margin-bottom: 12px; }

  /* Service cards */
  .service-card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; margin-bottom: 6px; overflow: hidden; transition: border-color 0.2s; }
  .service-card:hover { border-color: #484f58; }
  .service-card.pending { border-color: var(--yellow); }
  .service-row { display: flex; align-items: center; padding: 10px 16px; cursor: pointer; gap: 12px; }
  .service-row:hover { background: rgba(255,255,255,0.02); }

  /* Status indicator */
  .status-indicator { display: flex; align-items: center; gap: 8px; min-width: 90px; }
  .status-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
  .status-dot.running { background: var(--green); box-shadow: 0 0 6px var(--green); }
  .status-dot.stopped { background: var(--muted); }
  .status-dot.crashed { background: var(--red); box-shadow: 0 0 6px var(--red); }
  .status-dot.unhealthy { background: var(--orange); box-shadow: 0 0 6px var(--orange); }
  .status-dot.starting, .status-dot.building, .status-dot.restarting { background: var(--yellow); animation: pulse 1s infinite; }
  .status-dot.stopping { background: var(--yellow); animation: pulse 1.5s infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
  .status-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; font-weight: 600; }
  .status-label.running { color: var(--green); }
  .status-label.stopped { color: var(--muted); }
  .status-label.crashed { color: var(--red); }
  .status-label.unhealthy { color: var(--orange); }
  .status-label.starting, .status-label.building, .status-label.restarting { color: var(--yellow); }
  .status-label.stopping { color: var(--yellow); }

  .service-name { font-weight: 500; flex: 1; font-size: 14px; }
  .service-type { color: var(--muted); font-size: 11px; background: var(--bg); padding: 2px 8px; border-radius: 4px; text-transform: uppercase; letter-spacing: 0.5px; }
  .service-uptime { color: var(--muted); font-size: 12px; min-width: 80px; text-align: right; font-family: 'SF Mono', monospace; }
  .service-actions { display: flex; gap: 4px; }

  /* Expanded panel */
  .expanded { border-top: 1px solid var(--border); padding: 16px; background: var(--bg); }
  .expanded-meta { display: flex; gap: 16px; margin-bottom: 12px; flex-wrap: wrap; }
  .meta-item { font-size: 12px; color: var(--muted); }
  .meta-item span { color: var(--text); font-family: monospace; }

  /* Log viewer */
  .log-viewer { background: #010409; border: 1px solid var(--border); border-radius: 6px; padding: 12px; font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace; font-size: 12px; line-height: 1.6; max-height: 400px; overflow-y: auto; white-space: pre-wrap; word-break: break-all; }
  .log-line { }
  .log-line .ts { color: var(--muted); margin-right: 8px; user-select: none; }
  .log-line.stderr { color: var(--red); }
  .stdin-input { display: flex; margin-top: 8px; gap: 8px; }
  .stdin-input input { flex: 1; padding: 8px 12px; background: #010409; border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-family: monospace; font-size: 13px; outline: none; }
  .stdin-input input:focus { border-color: var(--blue); }

  /* Groups */
  .group-header { color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: 1.5px; margin: 20px 0 8px 4px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
  .group-count { background: var(--bg); padding: 1px 6px; border-radius: 8px; font-size: 10px; }

  /* Env modal */
  .env-modal { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); display: flex; align-items: center; justify-content: center; z-index: 100; backdrop-filter: blur(4px); }
  .env-modal-content { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; max-width: 600px; width: 90%; max-height: 80vh; overflow-y: auto; }
  .env-row { display: flex; padding: 6px 0; border-bottom: 1px solid var(--border); font-size: 13px; font-family: monospace; }
  .env-key { color: var(--blue); min-width: 200px; }
  .env-val { color: var(--text); word-break: break-all; }

  /* Confirm dialog */
  .confirm-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 150; backdrop-filter: blur(2px); }
  .confirm-box { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 24px; max-width: 400px; width: 90%; }
  .confirm-box h3 { margin-bottom: 12px; font-size: 16px; }
  .confirm-box p { color: var(--muted); font-size: 14px; margin-bottom: 20px; }
  .confirm-actions { display: flex; gap: 8px; justify-content: flex-end; }

  .children { margin-left: 24px; }

  /* Toast container */
  .toast-container { position: fixed; bottom: 20px; right: 20px; z-index: 200; display: flex; flex-direction: column-reverse; gap: 8px; }
  .toast { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 10px 16px; font-size: 13px; display: flex; align-items: center; gap: 8px; animation: slideIn 0.3s ease; min-width: 250px; box-shadow: 0 8px 24px rgba(0,0,0,0.4); }
  .toast-success { border-color: var(--green); }
  .toast-error { border-color: var(--red); }
  .toast-info { border-color: var(--blue); }
  .toast-icon { font-size: 16px; flex-shrink: 0; }
  .toast-dismiss { margin-left: auto; cursor: pointer; color: var(--muted); padding: 2px 4px; }
  .toast-dismiss:hover { color: var(--text); }
  @keyframes slideIn { from { transform: translateX(100%); opacity: 0; } to { transform: translateX(0); opacity: 1; } }
  @keyframes fadeOut { from { opacity: 1; } to { opacity: 0; transform: translateX(30px); } }

  /* Global action bar */
  .action-bar { position: fixed; bottom: 0; left: 0; right: 0; background: var(--surface); border-top: 1px solid var(--border); padding: 8px 24px; display: flex; align-items: center; justify-content: center; gap: 8px; z-index: 40; font-size: 13px; }
  .action-bar .spinner { width: 16px; height: 16px; }
</style>
</head>
<body>
<div id="app"></div>
<div class="toast-container" id="toasts"></div>
<script>
const API = '';
let authenticated = false;
let services = [];
let expandedService = null;
let logEntries = {};
let wsConnections = {};
let envModal = null;
let pollInterval = null;
let pendingActions = {};  // { serviceId: 'starting' | 'stopping' | 'restarting' | 'building' }
let globalAction = null;  // 'starting-all' | 'stopping-all' | null
let confirmDialog = null; // { title, message, onConfirm }

async function api(path, opts = {}) {
  const res = await fetch(API + path, { ...opts, headers: { 'Content-Type': 'application/json', ...opts.headers } });
  if (res.status === 401) { authenticated = false; render(); return null; }
  return res.json();
}

async function login(password) {
  const btn = document.querySelector('.login-box button');
  if (btn) { btn.disabled = true; btn.innerHTML = '<span class="spinner spinner-sm"></span> Logging in...'; }
  const res = await api('/api/auth/login', { method: 'POST', body: JSON.stringify({ password }) });
  if (res && res.status === 'ok') {
    authenticated = true;
    await refreshServices();
    startPolling();
  } else {
    const errEl = document.querySelector('.login-error');
    if (errEl) { errEl.textContent = 'Invalid password'; errEl.style.display = 'block'; }
    if (btn) { btn.disabled = false; btn.textContent = 'Login'; }
  }
  render();
}

async function refreshServices() {
  const data = await api('/api/services');
  if (data) {
    services = data;
    // Clear pending actions if the service status has changed
    for (const svc of flattenServices(data)) {
      const pending = pendingActions[svc.id];
      if (pending) {
        if (pending === 'starting' && (svc.status === 'running' || svc.status === 'crashed')) delete pendingActions[svc.id];
        else if (pending === 'stopping' && svc.status === 'stopped') delete pendingActions[svc.id];
        else if (pending === 'restarting' && svc.status === 'running') delete pendingActions[svc.id];
        else if (pending === 'building' && svc.status !== 'building') delete pendingActions[svc.id];
      }
    }
    // Clear global action if no pending per-service actions remain
    if (globalAction && Object.keys(pendingActions).length === 0) {
      globalAction = null;
    }
    render();
  }
}

function flattenServices(svcs) {
  const result = [];
  svcs.forEach(s => {
    result.push(s);
    if (s.children) result.push(...flattenServices(s.children));
  });
  return result;
}

function startPolling() {
  if (pollInterval) clearInterval(pollInterval);
  pollInterval = setInterval(refreshServices, 2500);
}

async function serviceAction(id, action) {
  pendingActions[id] = action === 'start' ? 'starting' : action === 'stop' ? 'stopping' : action === 'restart' ? 'restarting' : 'building';
  render();
  showToast('info', capitalise(action) + 'ing ' + id + '...');
  try {
    await api('/api/service/' + id + '/' + action, { method: 'POST' });
  } catch (e) {
    showToast('error', 'Failed to ' + action + ' ' + id);
    delete pendingActions[id];
    render();
  }
  // Let polling pick up the status change
  setTimeout(refreshServices, 800);
}

async function startAll() {
  globalAction = 'starting-all';
  render();
  showToast('info', 'Starting all services...');
  try {
    await api('/api/services/start-all', { method: 'POST' });
  } catch (e) {
    showToast('error', 'Failed to start all services');
    globalAction = null;
  }
  setTimeout(refreshServices, 1000);
}

function confirmStopAll() {
  confirmDialog = {
    title: 'Stop All Services',
    message: 'Are you sure you want to stop all running services? This will stop both native and Docker services.',
    onConfirm: async () => {
      confirmDialog = null;
      globalAction = 'stopping-all';
      render();
      showToast('info', 'Stopping all services...');
      try {
        await api('/api/services/stop-all', { method: 'POST' });
      } catch (e) {
        showToast('error', 'Failed to stop all services');
        globalAction = null;
      }
      setTimeout(refreshServices, 1000);
    }
  };
  render();
}

async function reloadConfig() {
  showToast('info', 'Reloading configuration...');
  const res = await api('/api/config/reload', { method: 'POST' });
  if (res) {
    if (res.error) {
      showToast('error', 'Config reload failed: ' + res.error);
    } else {
      showToast('success', 'Config reloaded. Added: ' + (res.added||[]).length + ', Removed: ' + (res.removed||[]).length);
      await refreshServices();
    }
  }
}

function toggleExpand(id) {
  if (expandedService === id) { disconnectLogs(id); expandedService = null; }
  else { if (expandedService) disconnectLogs(expandedService); expandedService = id; connectLogs(id); loadLogs(id); }
  render();
}

async function loadLogs(id) {
  const data = await api('/api/service/' + id + '/logs?lines=200');
  if (data) { logEntries[id] = data; render(); autoScrollLogs(); }
}

function connectLogs(id) {
  if (wsConnections[id]) return;
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(proto + '//' + location.host + '/ws/logs/' + id);
  ws.onmessage = (e) => {
    const entry = JSON.parse(e.data);
    if (!logEntries[id]) logEntries[id] = [];
    logEntries[id].push(entry);
    if (logEntries[id].length > 1000) logEntries[id] = logEntries[id].slice(-500);
    renderLogs(id);
    autoScrollLogs();
  };
  ws.onclose = () => { delete wsConnections[id]; };
  wsConnections[id] = ws;
}

function disconnectLogs(id) {
  if (wsConnections[id]) { wsConnections[id].close(); delete wsConnections[id]; }
}

function sendStdin(id, data) {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(proto + '//' + location.host + '/ws/stdin/' + id);
  ws.onopen = () => { ws.send(JSON.stringify({ data: data + '\n' })); ws.close(); };
}

async function showEnv(id) {
  const data = await api('/api/service/' + id + '/env');
  if (data && data.variables) { envModal = { id, vars: data.variables }; render(); }
}

function autoScrollLogs() {
  requestAnimationFrame(() => {
    const el = document.querySelector('.log-viewer');
    if (el) el.scrollTop = el.scrollHeight;
  });
}

let toastId = 0;
function showToast(type, msg) {
  const id = ++toastId;
  const container = document.getElementById('toasts');
  const icons = { success: '✓', error: '✕', info: 'ℹ' };
  const t = document.createElement('div');
  t.className = 'toast toast-' + type;
  t.id = 'toast-' + id;
  t.innerHTML = '<span class="toast-icon">' + (icons[type]||'') + '</span><span>' + escHtml(msg) + '</span><span class="toast-dismiss" onclick="dismissToast(' + id + ')">✕</span>';
  container.appendChild(t);
  const duration = type === 'error' ? 8000 : 4000;
  setTimeout(() => dismissToast(id), duration);
}

function dismissToast(id) {
  const t = document.getElementById('toast-' + id);
  if (t) { t.style.animation = 'fadeOut 0.3s ease forwards'; setTimeout(() => t.remove(), 300); }
}

function capitalise(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

function getServiceCounts() {
  const all = flattenServices(services);
  let running = 0, stopped = 0, errored = 0;
  all.forEach(s => {
    if (s.status === 'running') running++;
    else if (s.status === 'crashed' || s.status === 'unhealthy') errored++;
    else if (s.status === 'stopped') stopped++;
  });
  return { running, stopped, errored, total: all.length };
}

function render() {
  const app = document.getElementById('app');
  if (!authenticated) {
    app.innerHTML = '<div class="login"><div class="login-box"><h2>🔧 Foreman</h2><div class="login-error" style="display:none"></div><input type="password" id="pwd" placeholder="Enter password" onkeydown="if(event.key===\'Enter\')login(this.value)"><button onclick="login(document.getElementById(\'pwd\').value)">Login</button></div></div>';
    return;
  }

  const counts = getServiceCounts();

  // Group services
  const groups = {};
  services.forEach(s => {
    const g = s.group || 'Services';
    if (!groups[g]) groups[g] = [];
    groups[g].push(s);
  });

  const anyGlobalAction = !!globalAction;

  let html = '<div class="header"><div class="header-left"><h1>🔧 <span>Foreman</span></h1>';
  html += '<div class="header-stats">';
  if (counts.running > 0) html += '<span class="stat-pill stat-running">' + counts.running + ' running</span>';
  if (counts.stopped > 0) html += '<span class="stat-pill stat-stopped">' + counts.stopped + ' stopped</span>';
  if (counts.errored > 0) html += '<span class="stat-pill stat-error">' + counts.errored + ' error</span>';
  html += '</div></div>';
  html += '<div class="toolbar">';
  html += '<button class="btn btn-blue" onclick="reloadConfig()" title="Reload configuration">⟳ Config</button>';
  html += '<button class="btn btn-green" onclick="startAll()"' + (anyGlobalAction ? ' disabled' : '') + ' title="Start all stopped services">▶ Start All</button>';
  html += '<button class="btn btn-red" onclick="confirmStopAll()"' + (anyGlobalAction ? ' disabled' : '') + ' title="Stop all running services">■ Stop All</button>';
  html += '</div></div><div class="container">';

  for (const [group, svcs] of Object.entries(groups)) {
    html += '<div class="group-header">' + escHtml(group) + ' <span class="group-count">' + svcs.length + '</span></div>';
    svcs.forEach(s => { html += renderService(s, false); });
  }

  html += '</div>';

  // Global action bar
  if (globalAction) {
    const label = globalAction === 'starting-all' ? 'Starting all services...' : 'Stopping all services...';
    html += '<div class="action-bar"><span class="spinner"></span> ' + label + '</div>';
  }

  // Env modal
  if (envModal) {
    html += '<div class="env-modal" onclick="envModal=null;render()"><div class="env-modal-content" onclick="event.stopPropagation()">';
    html += '<h3 style="margin-bottom:12px">Environment: ' + escHtml(envModal.id) + '</h3>';
    const sortedKeys = Object.keys(envModal.vars).sort();
    for (const k of sortedKeys) {
      html += '<div class="env-row"><span class="env-key">' + escHtml(k) + '</span><span class="env-val">' + escHtml(envModal.vars[k]) + '</span></div>';
    }
    html += '<button class="btn" style="margin-top:16px" onclick="envModal=null;render()">Close</button>';
    html += '</div></div>';
  }

  // Confirm dialog
  if (confirmDialog) {
    html += '<div class="confirm-overlay" onclick="confirmDialog=null;render()">';
    html += '<div class="confirm-box" onclick="event.stopPropagation()">';
    html += '<h3>' + escHtml(confirmDialog.title) + '</h3>';
    html += '<p>' + escHtml(confirmDialog.message) + '</p>';
    html += '<div class="confirm-actions">';
    html += '<button class="btn" onclick="confirmDialog=null;render()">Cancel</button>';
    html += '<button class="btn btn-red" onclick="confirmDialog.onConfirm()">Confirm</button>';
    html += '</div></div></div>';
  }

  app.innerHTML = html;
  autoScrollLogs();
}

function renderService(s, isChild) {
  const statusClass = String(s.status).toLowerCase().replace(/\s/g, '');
  const isPending = !!pendingActions[s.id];
  const pendingLabel = pendingActions[s.id];
  const effectiveStatus = isPending ? pendingLabel : statusClass;

  let html = '<div class="service-card' + (isChild ? ' children' : '') + (isPending ? ' pending' : '') + '">';
  html += '<div class="service-row" onclick="toggleExpand(\'' + escAttr(s.id) + '\')">';

  // Status indicator
  html += '<div class="status-indicator">';
  html += '<div class="status-dot ' + effectiveStatus + '"></div>';
  html += '<span class="status-label ' + effectiveStatus + '">' + effectiveStatus + '</span>';
  html += '</div>';

  html += '<span class="service-name">' + escHtml(s.label || s.id) + '</span>';
  html += '<span class="service-type">' + escHtml(s.type) + '</span>';
  if (s.uptime && s.status === 'running') html += '<span class="service-uptime">↑ ' + escHtml(s.uptime) + '</span>';

  html += '<div class="service-actions" onclick="event.stopPropagation()">';
  if (s.has_build) {
    const buildPending = pendingActions[s.id] === 'building';
    html += '<button class="btn btn-sm" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'build\')" title="Build"' + (isPending ? ' disabled' : '') + '>' + (buildPending ? '<span class="spinner spinner-sm"></span>' : '🔨') + '</button>';
  }

  const startPending = pendingActions[s.id] === 'starting';
  const restartPending = pendingActions[s.id] === 'restarting';
  const stopPending = pendingActions[s.id] === 'stopping';

  html += '<button class="btn btn-sm btn-green" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'start\')" title="Start"' + (isPending ? ' disabled' : '') + '>' + (startPending ? '<span class="spinner spinner-sm"></span>' : '▶') + '</button>';
  html += '<button class="btn btn-sm" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'restart\')" title="Restart"' + (isPending ? ' disabled' : '') + '>' + (restartPending ? '<span class="spinner spinner-sm"></span>' : '⟳') + '</button>';
  html += '<button class="btn btn-sm btn-red" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'stop\')" title="Stop"' + (isPending ? ' disabled' : '') + '>' + (stopPending ? '<span class="spinner spinner-sm"></span>' : '■') + '</button>';
  if (s.type !== 'docker-compose') html += '<button class="btn btn-sm" onclick="showEnv(\'' + escAttr(s.id) + '\')" title="Environment">📋</button>';
  html += '</div></div>';

  if (expandedService === s.id) {
    const logs = logEntries[s.id] || [];
    html += '<div class="expanded">';
    html += '<div class="expanded-meta">';
    if (s.pid) html += '<div class="meta-item">PID: <span>' + s.pid + '</span></div>';
    if (s.restarts !== undefined) html += '<div class="meta-item">Restarts: <span>' + s.restarts + '</span></div>';
    if (s.exit_code !== undefined && s.exit_code !== 0) html += '<div class="meta-item">Exit code: <span style="color:var(--red)">' + s.exit_code + '</span></div>';
    if (s.type === 'docker-compose' && !isChild) html += '<div class="meta-item" style="color:var(--blue)">📋 Docker Compose operations log (start/stop/restart output)</div>';
    html += '</div>';
    html += '<div class="log-viewer" id="log-viewer-' + escAttr(s.id) + '">';
    logs.forEach(l => {
      const cls = l.stream === 'stderr' ? ' stderr' : '';
      html += '<div class="log-line' + cls + '"><span class="ts">' + escHtml(l.timestamp || '') + '</span>' + escHtml(l.line) + '</div>';
    });
    if (logs.length === 0) html += '<div style="color:var(--muted);text-align:center;padding:20px">' + (s.type === 'docker-compose' && !isChild ? 'No operations logged yet. Start/stop/restart to see output here in real-time.' : 'No logs yet') + '</div>';
    html += '</div>';
    if (s.type !== 'docker-compose') {
      html += '<div class="stdin-input"><input type="text" id="stdin-' + escAttr(s.id) + '" placeholder="Send input to process..." onkeydown="if(event.key===\'Enter\'){sendStdin(\'' + escAttr(s.id) + '\',this.value);this.value=\'\'}"><button class="btn btn-sm" onclick="var i=document.getElementById(\'stdin-' + escAttr(s.id) + '\');sendStdin(\'' + escAttr(s.id) + '\',i.value);i.value=\'\'">Send</button></div>';
    }
    html += '</div>';
  }

  // Render children (docker sub-services)
  if (s.children && s.children.length > 0) {
    s.children.forEach(c => { html += renderService(c, true); });
  }

  html += '</div>';
  return html;
}

// Optimized: only re-render the log viewer div instead of the whole page
function renderLogs(id) {
  const viewer = document.getElementById('log-viewer-' + id);
  if (!viewer) return;
  const logs = logEntries[id] || [];
  let html = '';
  logs.forEach(l => {
    const cls = l.stream === 'stderr' ? ' stderr' : '';
    html += '<div class="log-line' + cls + '"><span class="ts">' + escHtml(l.timestamp || '') + '</span>' + escHtml(l.line) + '</div>';
  });
  if (logs.length === 0) html = '<div style="color:var(--muted);text-align:center;padding:20px">No logs yet</div>';
  viewer.innerHTML = html;
}

function escHtml(s) { if (!s) return ''; const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
function escAttr(s) { return s.replace(/'/g, "\\'").replace(/"/g, '&quot;'); }

// Check if already authenticated
(async () => {
  const res = await fetch('/api/services');
  if (res.ok) { authenticated = true; services = await res.json(); startPolling(); }
  render();
})();
</script>
</body>
</html>
`
