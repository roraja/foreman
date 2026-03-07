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

  /* Tabs */
  .tabs { display: flex; gap: 0; background: var(--surface); border-bottom: 1px solid var(--border); padding: 0 24px; }
  .tab { padding: 10px 20px; cursor: pointer; font-size: 14px; font-weight: 500; border-bottom: 2px solid transparent; color: var(--muted); transition: all 0.15s; }
  .tab:hover { color: var(--text); }
  .tab.active { color: var(--blue); border-bottom-color: var(--blue); }
  .tab-badge { background: var(--bg); padding: 1px 7px; border-radius: 10px; font-size: 11px; margin-left: 6px; }

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
  .status-dot.idle { background: var(--muted); }
  .status-dot.success { background: var(--green); }
  .status-dot.failed { background: var(--red); box-shadow: 0 0 6px var(--red); }
  .status-dot.canceled { background: var(--orange); }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
  .status-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; font-weight: 600; }
  .status-label.running { color: var(--green); }
  .status-label.stopped { color: var(--muted); }
  .status-label.crashed { color: var(--red); }
  .status-label.unhealthy { color: var(--orange); }
  .status-label.starting, .status-label.building, .status-label.restarting { color: var(--yellow); }
  .status-label.stopping { color: var(--yellow); }
  .status-label.idle { color: var(--muted); }
  .status-label.success { color: var(--green); }
  .status-label.failed { color: var(--red); }
  .status-label.canceled { color: var(--orange); }

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

  /* Command-specific */
  .cmd-search { display: flex; gap: 8px; margin-bottom: 16px; }
  .cmd-search input, .cmd-search select { padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg); color: var(--text); font-size: 13px; outline: none; }
  .cmd-search input { flex: 1; }
  .cmd-search input:focus, .cmd-search select:focus { border-color: var(--blue); }
  .cmd-card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; margin-bottom: 6px; overflow: hidden; transition: border-color 0.2s; }
  .cmd-card:hover { border-color: #484f58; }
  .cmd-row { display: flex; align-items: center; padding: 10px 16px; cursor: pointer; gap: 12px; }
  .cmd-row:hover { background: rgba(255,255,255,0.02); }
  .cmd-desc { color: var(--muted); font-size: 12px; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .cmd-tags { display: flex; gap: 4px; flex-wrap: wrap; }
  .cmd-tag { color: var(--purple); font-size: 10px; background: rgba(188,140,255,0.1); padding: 2px 7px; border-radius: 4px; }
  .cmd-duration { color: var(--muted); font-size: 12px; min-width: 60px; text-align: right; font-family: 'SF Mono', monospace; }

  /* History */
  .history-entry { display: flex; align-items: center; gap: 10px; padding: 6px 0; border-bottom: 1px solid var(--border); font-size: 12px; }
  .history-entry .ts { color: var(--muted); font-family: monospace; }
</style>
</head>
<body>
<div id="app"></div>
<div class="toast-container" id="toasts"></div>
<script>
const API = '';
let authenticated = false;
let activeTab = 'services';
let services = [];
let expandedService = null;
let logEntries = {};
let wsConnections = {};
let envModal = null;
let pollInterval = null;
let pendingActions = {};
let globalAction = null;
let confirmDialog = null;

// Commands state
let commands = [];
let cmdSearch = '';
let cmdGroupFilter = '';
let cmdTagFilter = '';
let expandedCommand = null;
let cmdLogEntries = {};
let cmdWsConnections = {};
let cmdHistory = []; // { id, cmdId, label, status, startedAt, duration }

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
    await refreshCommands();
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
    for (const svc of flattenServices(data)) {
      const pending = pendingActions[svc.id];
      if (pending) {
        if (pending === 'starting' && (svc.status === 'running' || svc.status === 'crashed')) delete pendingActions[svc.id];
        else if (pending === 'stopping' && svc.status === 'stopped') delete pendingActions[svc.id];
        else if (pending === 'restarting' && svc.status === 'running') delete pendingActions[svc.id];
        else if (pending === 'building' && svc.status !== 'building') delete pendingActions[svc.id];
      }
    }
    if (globalAction && Object.keys(pendingActions).length === 0) globalAction = null;
    render();
  }
}

async function refreshCommands() {
  let url = '/api/commands';
  const params = [];
  if (cmdSearch) params.push('q=' + encodeURIComponent(cmdSearch));
  if (cmdGroupFilter) params.push('group=' + encodeURIComponent(cmdGroupFilter));
  if (cmdTagFilter) params.push('tag=' + encodeURIComponent(cmdTagFilter));
  if (params.length) url += '?' + params.join('&');
  const data = await api(url);
  if (data) { commands = data || []; render(); }
}

function flattenServices(svcs) {
  const result = [];
  svcs.forEach(s => { result.push(s); if (s.children) result.push(...flattenServices(s.children)); });
  return result;
}

function startPolling() {
  if (pollInterval) clearInterval(pollInterval);
  pollInterval = setInterval(() => {
    refreshServices();
    if (activeTab === 'commands') refreshCommands();
  }, 2500);
}

function switchTab(tab) {
  activeTab = tab;
  if (tab === 'commands') refreshCommands();
  render();
}

async function serviceAction(id, action) {
  pendingActions[id] = action === 'start' ? 'starting' : action === 'stop' ? 'stopping' : action === 'restart' ? 'restarting' : 'building';
  render();
  showToast('info', capitalise(action) + 'ing ' + id + '...');
  try { await api('/api/service/' + id + '/' + action, { method: 'POST' }); }
  catch (e) { showToast('error', 'Failed to ' + action + ' ' + id); delete pendingActions[id]; render(); }
  setTimeout(refreshServices, 800);
}

async function startAll() {
  globalAction = 'starting-all'; render();
  showToast('info', 'Starting all services...');
  try { await api('/api/services/start-all', { method: 'POST' }); }
  catch (e) { showToast('error', 'Failed to start all services'); globalAction = null; }
  setTimeout(refreshServices, 1000);
}

function confirmStopAll() {
  confirmDialog = {
    title: 'Stop All Services',
    message: 'Are you sure you want to stop all running services?',
    onConfirm: async () => {
      confirmDialog = null; globalAction = 'stopping-all'; render();
      showToast('info', 'Stopping all services...');
      try { await api('/api/services/stop-all', { method: 'POST' }); }
      catch (e) { showToast('error', 'Failed to stop all services'); globalAction = null; }
      setTimeout(refreshServices, 1000);
    }
  };
  render();
}

async function reloadConfig() {
  showToast('info', 'Reloading configuration...');
  const res = await api('/api/config/reload', { method: 'POST' });
  if (res) {
    if (res.error) showToast('error', 'Config reload failed: ' + res.error);
    else { showToast('success', 'Config reloaded. Added: ' + (res.added||[]).length + ', Removed: ' + (res.removed||[]).length); await refreshServices(); await refreshCommands(); }
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
    renderServiceLogs(id); autoScrollLogs();
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

/* ---------- Command functions ---------- */

function toggleCmdExpand(id) {
  if (expandedCommand === id) { disconnectCmdLogs(id); expandedCommand = null; }
  else { if (expandedCommand) disconnectCmdLogs(expandedCommand); expandedCommand = id; connectCmdLogs(id); loadCmdLogs(id); }
  render();
}

async function loadCmdLogs(id) {
  const data = await api('/api/command/' + id + '/logs?lines=200');
  if (data) { cmdLogEntries[id] = data; render(); autoScrollLogs(); }
}

function connectCmdLogs(id) {
  if (cmdWsConnections[id]) return;
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(proto + '//' + location.host + '/ws/command/' + id);
  ws.onmessage = (e) => {
    const entry = JSON.parse(e.data);
    if (!cmdLogEntries[id]) cmdLogEntries[id] = [];
    cmdLogEntries[id].push(entry);
    if (cmdLogEntries[id].length > 1000) cmdLogEntries[id] = cmdLogEntries[id].slice(-500);
    renderCmdLogs(id); autoScrollLogs();
  };
  ws.onclose = () => { delete cmdWsConnections[id]; };
  cmdWsConnections[id] = ws;
}

function disconnectCmdLogs(id) {
  if (cmdWsConnections[id]) { cmdWsConnections[id].close(); delete cmdWsConnections[id]; }
}

async function runCommand(id, label) {
  const cmd = commands.find(c => c.id === id);
  if (cmd && cmd.confirm) {
    confirmDialog = {
      title: 'Run ' + (label || id) + '?',
      message: 'This command has confirm enabled. Are you sure you want to run it?',
      onConfirm: () => { confirmDialog = null; doRunCommand(id, label); }
    };
    render();
    return;
  }
  doRunCommand(id, label);
}

async function doRunCommand(id, label) {
  showToast('info', 'Running ' + (label || id) + '...');
  cmdHistory.unshift({ id: Date.now(), cmdId: id, label: label || id, status: 'running', startedAt: new Date().toLocaleTimeString(), duration: '' });
  if (cmdHistory.length > 50) cmdHistory = cmdHistory.slice(0, 50);
  render();
  try {
    await api('/api/command/' + id + '/run', { method: 'POST' });
    // Poll for completion
    pollCmdStatus(id, cmdHistory[0]);
  } catch (e) {
    showToast('error', 'Failed to run ' + id);
    cmdHistory[0].status = 'failed';
    render();
  }
}

function pollCmdStatus(id, historyEntry) {
  const check = async () => {
    const info = await api('/api/command/' + id + '/status');
    if (!info) return;
    const cmd = commands.find(c => c.id === id);
    if (cmd) { cmd.status = info.status; cmd.exit_code = info.exit_code; cmd.duration = info.duration; }
    if (info.status === 'running') { setTimeout(check, 1000); }
    else {
      historyEntry.status = info.status;
      historyEntry.duration = info.duration || '';
      if (info.status === 'success') showToast('success', (historyEntry.label) + ' completed');
      else if (info.status === 'failed') showToast('error', (historyEntry.label) + ' failed');
      else if (info.status === 'canceled') showToast('info', (historyEntry.label) + ' canceled');
    }
    render();
  };
  setTimeout(check, 500);
}

async function cancelCommand(id) {
  showToast('info', 'Canceling ' + id + '...');
  await api('/api/command/' + id + '/cancel', { method: 'POST' });
  setTimeout(() => refreshCommands(), 500);
}

let cmdSearchTimer = null;
function onCmdSearchInput(val) { cmdSearch = val; clearTimeout(cmdSearchTimer); cmdSearchTimer = setTimeout(refreshCommands, 300); }
function onCmdGroupFilter(val) { cmdGroupFilter = val; refreshCommands(); }
function onCmdTagFilter(val) { cmdTagFilter = val; refreshCommands(); }

function getCommandGroups() {
  const groups = new Set();
  commands.forEach(c => { if (c.group) groups.add(c.group); });
  return Array.from(groups).sort();
}

function getCommandTags() {
  const tags = new Set();
  commands.forEach(c => { (c.tags || []).forEach(t => tags.add(t)); });
  return Array.from(tags).sort();
}

/* ---------- Rendering ---------- */

function autoScrollLogs() {
  requestAnimationFrame(() => { const el = document.querySelector('.log-viewer'); if (el) el.scrollTop = el.scrollHeight; });
}

let toastId = 0;
function showToast(type, msg) {
  const id = ++toastId;
  const container = document.getElementById('toasts');
  const icons = { success: '✓', error: '✕', info: 'ℹ' };
  const t = document.createElement('div');
  t.className = 'toast toast-' + type; t.id = 'toast-' + id;
  t.innerHTML = '<span class="toast-icon">' + (icons[type]||'') + '</span><span>' + escHtml(msg) + '</span><span class="toast-dismiss" onclick="dismissToast(' + id + ')">✕</span>';
  container.appendChild(t);
  setTimeout(() => dismissToast(id), type === 'error' ? 8000 : 4000);
}
function dismissToast(id) { const t = document.getElementById('toast-' + id); if (t) { t.style.animation = 'fadeOut 0.3s ease forwards'; setTimeout(() => t.remove(), 300); } }

function capitalise(s) { return s.charAt(0).toUpperCase() + s.slice(1); }
function getServiceCounts() {
  const all = flattenServices(services);
  let running = 0, stopped = 0, errored = 0;
  all.forEach(s => { if (s.status === 'running') running++; else if (s.status === 'crashed' || s.status === 'unhealthy') errored++; else if (s.status === 'stopped') stopped++; });
  return { running, stopped, errored, total: all.length };
}

function render() {
  const app = document.getElementById('app');
  if (!authenticated) {
    app.innerHTML = '<div class="login"><div class="login-box"><h2>🔧 Foreman</h2><div class="login-error" style="display:none"></div><input type="password" id="pwd" placeholder="Enter password" onkeydown="if(event.key===\'Enter\')login(this.value)"><button onclick="login(document.getElementById(\'pwd\').value)">Login</button></div></div>';
    return;
  }
  const counts = getServiceCounts();
  let html = '<div class="header"><div class="header-left"><h1>🔧 <span>Foreman</span></h1>';
  html += '<div class="header-stats">';
  if (counts.running > 0) html += '<span class="stat-pill stat-running">' + counts.running + ' running</span>';
  if (counts.stopped > 0) html += '<span class="stat-pill stat-stopped">' + counts.stopped + ' stopped</span>';
  if (counts.errored > 0) html += '<span class="stat-pill stat-error">' + counts.errored + ' error</span>';
  html += '</div></div>';
  html += '<div class="toolbar">';
  html += '<button class="btn btn-blue" onclick="reloadConfig()" title="Reload configuration">⟳ Config</button>';
  if (activeTab === 'services') {
    const anyGlobalAction = !!globalAction;
    html += '<button class="btn btn-green" onclick="startAll()"' + (anyGlobalAction ? ' disabled' : '') + '>▶ Start All</button>';
    html += '<button class="btn btn-red" onclick="confirmStopAll()"' + (anyGlobalAction ? ' disabled' : '') + '>■ Stop All</button>';
  }
  html += '</div></div>';

  // Tabs
  html += '<div class="tabs">';
  html += '<div class="tab' + (activeTab === 'services' ? ' active' : '') + '" onclick="switchTab(\'services\')">Services<span class="tab-badge">' + counts.total + '</span></div>';
  html += '<div class="tab' + (activeTab === 'commands' ? ' active' : '') + '" onclick="switchTab(\'commands\')">Commands<span class="tab-badge">' + commands.length + '</span></div>';
  html += '<div class="tab' + (activeTab === 'history' ? ' active' : '') + '" onclick="switchTab(\'history\')">History<span class="tab-badge">' + cmdHistory.length + '</span></div>';
  html += '</div>';

  html += '<div class="container">';
  if (activeTab === 'services') html += renderServicesTab();
  else if (activeTab === 'commands') html += renderCommandsTab();
  else if (activeTab === 'history') html += renderHistoryTab();
  html += '</div>';

  // Action bar
  if (globalAction) {
    const label = globalAction === 'starting-all' ? 'Starting all services...' : 'Stopping all services...';
    html += '<div class="action-bar"><span class="spinner"></span> ' + label + '</div>';
  }

  // Env modal
  if (envModal) {
    html += '<div class="env-modal" onclick="envModal=null;render()"><div class="env-modal-content" onclick="event.stopPropagation()">';
    html += '<h3 style="margin-bottom:12px">Environment: ' + escHtml(envModal.id) + '</h3>';
    for (const k of Object.keys(envModal.vars).sort()) {
      html += '<div class="env-row"><span class="env-key">' + escHtml(k) + '</span><span class="env-val">' + escHtml(envModal.vars[k]) + '</span></div>';
    }
    html += '<button class="btn" style="margin-top:16px" onclick="envModal=null;render()">Close</button></div></div>';
  }

  // Confirm dialog
  if (confirmDialog) {
    html += '<div class="confirm-overlay" onclick="confirmDialog=null;render()"><div class="confirm-box" onclick="event.stopPropagation()">';
    html += '<h3>' + escHtml(confirmDialog.title) + '</h3><p>' + escHtml(confirmDialog.message) + '</p>';
    html += '<div class="confirm-actions"><button class="btn" onclick="confirmDialog=null;render()">Cancel</button>';
    html += '<button class="btn btn-red" onclick="confirmDialog.onConfirm()">Confirm</button></div></div></div>';
  }

  // Preserve focus state across re-render
  const focused = document.activeElement;
  let focusId = null, focusStart = null, focusEnd = null, focusVal = null;
  if (focused && focused.id && (focused.tagName === 'INPUT' || focused.tagName === 'TEXTAREA' || focused.tagName === 'SELECT')) {
    focusId = focused.id;
    focusStart = focused.selectionStart;
    focusEnd = focused.selectionEnd;
    focusVal = focused.value;
  }

  app.innerHTML = html;

  // Restore focus after re-render
  if (focusId) {
    const el = document.getElementById(focusId);
    if (el) {
      el.focus();
      if (focusVal !== null) el.value = focusVal;
      if (focusStart !== null && el.setSelectionRange) {
        try { el.setSelectionRange(focusStart, focusEnd); } catch(e) {}
      }
    }
  }

  autoScrollLogs();
}

function renderServicesTab() {
  const groups = {};
  services.forEach(s => { const g = s.group || 'Services'; if (!groups[g]) groups[g] = []; groups[g].push(s); });
  let html = '';
  for (const [group, svcs] of Object.entries(groups)) {
    html += '<div class="group-header">' + escHtml(group) + ' <span class="group-count">' + svcs.length + '</span></div>';
    svcs.forEach(s => { html += renderService(s, false); });
  }
  if (services.length === 0) html += '<div style="text-align:center;color:var(--muted);padding:40px">No services configured</div>';
  return html;
}

function renderService(s, isChild) {
  const statusClass = String(s.status).toLowerCase().replace(/\s/g, '');
  const isPending = !!pendingActions[s.id];
  const pendingLabel = pendingActions[s.id];
  const effectiveStatus = isPending ? pendingLabel : statusClass;

  let html = '<div class="service-card' + (isChild ? ' children' : '') + (isPending ? ' pending' : '') + '">';
  html += '<div class="service-row" onclick="toggleExpand(\'' + escAttr(s.id) + '\')">';
  html += '<div class="status-indicator"><div class="status-dot ' + effectiveStatus + '"></div><span class="status-label ' + effectiveStatus + '">' + effectiveStatus + '</span></div>';
  html += '<span class="service-name">' + escHtml(s.label || s.id) + '</span>';
  html += '<span class="service-type">' + escHtml(s.type) + '</span>';
  if (s.uptime && s.status === 'running') html += '<span class="service-uptime">↑ ' + escHtml(s.uptime) + '</span>';

  html += '<div class="service-actions" onclick="event.stopPropagation()">';
  if (s.url) html += '<button class="btn btn-sm btn-blue" onclick="window.open(\'' + escAttr(s.url) + '\',\'_blank\')" title="Open">🔗</button>';
  if (s.has_build) {
    const bp = pendingActions[s.id] === 'building';
    html += '<button class="btn btn-sm" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'build\')"' + (isPending ? ' disabled' : '') + '>' + (bp ? '<span class="spinner spinner-sm"></span>' : '🔨') + '</button>';
  }
  html += '<button class="btn btn-sm btn-green" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'start\')"' + (isPending ? ' disabled' : '') + '>' + (pendingActions[s.id]==='starting' ? '<span class="spinner spinner-sm"></span>' : '▶') + '</button>';
  html += '<button class="btn btn-sm" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'restart\')"' + (isPending ? ' disabled' : '') + '>' + (pendingActions[s.id]==='restarting' ? '<span class="spinner spinner-sm"></span>' : '⟳') + '</button>';
  html += '<button class="btn btn-sm btn-red" onclick="serviceAction(\'' + escAttr(s.id) + '\',\'stop\')"' + (isPending ? ' disabled' : '') + '>' + (pendingActions[s.id]==='stopping' ? '<span class="spinner spinner-sm"></span>' : '■') + '</button>';
  if (s.type !== 'docker-compose') html += '<button class="btn btn-sm" onclick="showEnv(\'' + escAttr(s.id) + '\')" title="Environment">📋</button>';
  html += '</div></div>';

  if (expandedService === s.id) {
    const logs = logEntries[s.id] || [];
    html += '<div class="expanded"><div class="expanded-meta">';
    if (s.pid) html += '<div class="meta-item">PID: <span>' + s.pid + '</span></div>';
    if (s.restarts !== undefined) html += '<div class="meta-item">Restarts: <span>' + s.restarts + '</span></div>';
    if (s.exit_code !== undefined && s.exit_code !== 0) html += '<div class="meta-item">Exit code: <span style="color:var(--red)">' + s.exit_code + '</span></div>';
    html += '</div>';
    html += '<div class="log-viewer" id="log-viewer-' + escAttr(s.id) + '">';
    logs.forEach(l => { html += '<div class="log-line' + (l.stream === 'stderr' ? ' stderr' : '') + '"><span class="ts">' + escHtml(l.timestamp||'') + '</span>' + escHtml(l.line) + '</div>'; });
    if (logs.length === 0) html += '<div style="color:var(--muted);text-align:center;padding:20px">No logs yet</div>';
    html += '</div>';
    if (s.type !== 'docker-compose') {
      html += '<div class="stdin-input"><input type="text" id="stdin-' + escAttr(s.id) + '" placeholder="Send input to process..." onkeydown="if(event.key===\'Enter\'){sendStdin(\'' + escAttr(s.id) + '\',this.value);this.value=\'\'}"><button class="btn btn-sm" onclick="var i=document.getElementById(\'stdin-' + escAttr(s.id) + '\');sendStdin(\'' + escAttr(s.id) + '\',i.value);i.value=\'\'">Send</button></div>';
    }
    html += '</div>';
  }

  if (s.children && s.children.length > 0) s.children.forEach(c => { html += renderService(c, true); });
  html += '</div>';
  return html;
}

function renderCommandsTab() {
  const groups = getCommandGroups();
  const tags = getCommandTags();

  let html = '<div class="cmd-search">';
  html += '<input type="text" id="cmd-search" placeholder="🔍 Search commands..." value="' + escAttr(cmdSearch) + '" oninput="onCmdSearchInput(this.value)">';
  html += '<select onchange="onCmdGroupFilter(this.value)"><option value="">All Groups</option>';
  groups.forEach(g => { html += '<option value="' + escAttr(g) + '"' + (cmdGroupFilter === g ? ' selected' : '') + '>' + escHtml(g) + '</option>'; });
  html += '</select>';
  html += '<select onchange="onCmdTagFilter(this.value)"><option value="">All Tags</option>';
  tags.forEach(t => { html += '<option value="' + escAttr(t) + '"' + (cmdTagFilter === t ? ' selected' : '') + '>' + escHtml(t) + '</option>'; });
  html += '</select></div>';

  // Group commands
  const cmdGroups = {};
  commands.forEach(c => { const g = c.group || 'Other'; if (!cmdGroups[g]) cmdGroups[g] = []; cmdGroups[g].push(c); });

  for (const [group, cmds] of Object.entries(cmdGroups)) {
    html += '<div class="group-header">' + escHtml(group) + ' <span class="group-count">' + cmds.length + '</span></div>';
    cmds.forEach(c => { html += renderCommand(c); });
  }

  if (commands.length === 0) html += '<div style="text-align:center;color:var(--muted);padding:40px">No commands found</div>';
  return html;
}

function renderCommand(c) {
  const st = c.status || 'idle';
  let html = '<div class="cmd-card">';
  html += '<div class="cmd-row" onclick="toggleCmdExpand(\'' + escAttr(c.id) + '\')">';

  // Status
  html += '<div class="status-indicator"><div class="status-dot ' + st + '"></div><span class="status-label ' + st + '">' + st + '</span></div>';

  // Name
  html += '<span class="service-name">' + escHtml(c.label || c.id) + '</span>';

  // Description
  if (c.description) html += '<span class="cmd-desc">' + escHtml(c.description) + '</span>';

  // Tags
  if (c.tags && c.tags.length) {
    html += '<div class="cmd-tags">';
    c.tags.forEach(t => { html += '<span class="cmd-tag">' + escHtml(t) + '</span>'; });
    html += '</div>';
  }

  // Duration
  if (c.duration) html += '<span class="cmd-duration">' + escHtml(c.duration) + '</span>';

  // Actions
  html += '<div class="service-actions" onclick="event.stopPropagation()">';
  if (st === 'running') {
    html += '<button class="btn btn-sm btn-red" onclick="cancelCommand(\'' + escAttr(c.id) + '\')" title="Cancel">✕ Cancel</button>';
  } else {
    html += '<button class="btn btn-sm btn-green" onclick="runCommand(\'' + escAttr(c.id) + '\',\'' + escAttr(c.label || c.id) + '\')" title="Run">';
    if (c.confirm) html += '⚠ ';
    html += '▶ Run</button>';
  }
  html += '</div></div>';

  // Expanded: logs
  if (expandedCommand === c.id) {
    const logs = cmdLogEntries[c.id] || [];
    html += '<div class="expanded"><div class="expanded-meta">';
    if (c.exit_code !== undefined && c.exit_code !== null) html += '<div class="meta-item">Exit code: <span' + (c.exit_code !== 0 ? ' style="color:var(--red)"' : '') + '>' + c.exit_code + '</span></div>';
    if (c.duration) html += '<div class="meta-item">Duration: <span>' + escHtml(c.duration) + '</span></div>';
    if (c.depends_on && c.depends_on.length) html += '<div class="meta-item">Depends on: <span>' + escHtml(c.depends_on.join(', ')) + '</span></div>';
    if (c.parallel && c.parallel.length) html += '<div class="meta-item">Parallel: <span>' + escHtml(c.parallel.join(', ')) + '</span></div>';
    html += '</div>';
    html += '<div class="log-viewer" id="log-viewer-' + escAttr(c.id) + '">';
    logs.forEach(l => { html += '<div class="log-line' + (l.stream === 'stderr' ? ' stderr' : '') + '"><span class="ts">' + escHtml(l.timestamp||'') + '</span>' + escHtml(l.line) + '</div>'; });
    if (logs.length === 0) html += '<div style="color:var(--muted);text-align:center;padding:20px">No output yet — run the command to see logs</div>';
    html += '</div></div>';
  }

  html += '</div>';
  return html;
}

function renderHistoryTab() {
  let html = '';

  // In-memory recent runs
  if (cmdHistory.length > 0) {
    html += '<div class="group-header">Recent Runs (this session) <span class="group-count">' + cmdHistory.length + '</span></div>';
    cmdHistory.forEach(h => {
      const st = h.status || 'idle';
      html += '<div class="cmd-card"><div class="cmd-row">';
      html += '<div class="status-indicator"><div class="status-dot ' + st + '"></div><span class="status-label ' + st + '">' + st + '</span></div>';
      html += '<span class="service-name">' + escHtml(h.label) + '</span>';
      html += '<span class="cmd-desc" style="color:var(--muted)">' + escHtml(h.startedAt) + '</span>';
      if (h.duration) html += '<span class="cmd-duration">' + escHtml(h.duration) + '</span>';
      html += '</div></div>';
    });
  }

  // Persisted log browser
  html += '<div class="group-header" style="margin-top:24px">Persisted Logs</div>';
  html += '<div class="cmd-search"><input type="text" placeholder="Service or command name..." id="log-name-input" value="' + escAttr(historyLogName) + '" onkeydown="if(event.key===\'Enter\')loadLogRuns(this.value)"><button class="btn btn-sm btn-blue" onclick="loadLogRuns(document.getElementById(\'log-name-input\').value)">Load</button></div>';

  // Quick buttons for all known services and commands
  const allNames = [];
  services.forEach(s => { if (s.type !== 'docker-compose') allNames.push(s.id); });
  commands.forEach(c => allNames.push(c.id));
  if (allNames.length > 0) {
    html += '<div style="display:flex;gap:4px;flex-wrap:wrap;margin-bottom:12px">';
    allNames.forEach(n => {
      const active = historyLogName === n ? ' btn-blue' : '';
      html += '<button class="btn btn-sm' + active + '" onclick="loadLogRuns(\'' + escAttr(n) + '\')">' + escHtml(n) + '</button>';
    });
    html += '</div>';
  }

  // Show runs
  if (historyRuns && historyRuns.length > 0) {
    html += '<div class="group-header">' + escHtml(historyLogName) + ' <span class="group-count">' + historyRuns.length + ' runs</span></div>';
    historyRuns.forEach((run, idx) => {
      const isExpanded = historyExpandedRun === run.run_number;
      html += '<div class="cmd-card"><div class="cmd-row" onclick="toggleHistoryRun(' + run.run_number + ',\'' + escAttr(historyLogName) + '\')" style="cursor:pointer">';
      html += '<span class="service-name">Run #' + run.run_number + '</span>';
      html += '<span class="cmd-desc" style="color:var(--muted)">' + escHtml(run.timestamp) + '</span>';
      const totalSize = (run.stdout_size || 0) + (run.stderr_size || 0);
      html += '<span class="cmd-duration">' + formatBytes(totalSize) + '</span>';
      html += '</div>';
      if (isExpanded) {
        html += '<div class="expanded">';
        html += '<div style="display:flex;gap:8px;margin-bottom:8px">';
        if (run.stdout_file) html += '<button class="btn btn-sm' + (historyActiveStream === 'stdout' ? ' btn-blue' : '') + '" onclick="event.stopPropagation();loadHistoryFile(\'' + escAttr(run.stdout_file) + '\',\'stdout\')">stdout (' + formatBytes(run.stdout_size) + ')</button>';
        if (run.stderr_file) html += '<button class="btn btn-sm' + (historyActiveStream === 'stderr' ? ' btn-red' : '') + '" onclick="event.stopPropagation();loadHistoryFile(\'' + escAttr(run.stderr_file) + '\',\'stderr\')">stderr (' + formatBytes(run.stderr_size) + ')</button>';
        html += '</div>';
        html += '<div class="log-viewer">';
        if (historyLogContent && historyLogContent.length > 0) {
          const cls = historyActiveStream === 'stderr' ? ' stderr' : '';
          historyLogContent.forEach(l => { html += '<div class="log-line' + cls + '">' + escHtml(l) + '</div>'; });
        } else {
          html += '<div style="color:var(--muted);text-align:center;padding:20px">Select stdout or stderr to view</div>';
        }
        html += '</div></div>';
      }
      html += '</div>';
    });
  } else if (historyLogName && historyRuns !== null) {
    html += '<div style="text-align:center;color:var(--muted);padding:20px">No persisted logs found for "' + escHtml(historyLogName) + '"</div>';
  } else if (!historyLogName) {
    html += '<div style="text-align:center;color:var(--muted);padding:20px">Enter a service or command name above, or click a button to browse its log history.</div>';
  }

  return html;
}

let historyLogName = '';
let historyRuns = null;
let historyExpandedRun = null;
let historyActiveStream = '';
let historyLogContent = null;

async function loadLogRuns(name) {
  historyLogName = name;
  historyExpandedRun = null;
  historyLogContent = null;
  historyActiveStream = '';
  if (!name) { historyRuns = null; render(); return; }
  const data = await api('/api/logs/' + encodeURIComponent(name));
  historyRuns = data || [];
  render();
}

function toggleHistoryRun(runNumber, name) {
  if (historyExpandedRun === runNumber) { historyExpandedRun = null; historyLogContent = null; historyActiveStream = ''; }
  else { historyExpandedRun = runNumber; historyLogContent = null; historyActiveStream = ''; }
  render();
}

async function loadHistoryFile(filePath, stream) {
  historyActiveStream = stream;
  historyLogContent = null;
  render();
  const data = await api('/api/logs/' + encodeURIComponent(historyLogName) + '/read?file=' + encodeURIComponent(filePath) + '&lines=500');
  if (data && data.lines) { historyLogContent = data.lines; }
  else { historyLogContent = ['(empty)']; }
  render();
  autoScrollLogs();
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024*1024) return (bytes/1024).toFixed(1) + ' KB';
  return (bytes/(1024*1024)).toFixed(1) + ' MB';
}

// Optimized log renders
function renderServiceLogs(id) {
  const viewer = document.getElementById('log-viewer-' + id);
  if (!viewer) return;
  const logs = logEntries[id] || [];
  let html = '';
  logs.forEach(l => { html += '<div class="log-line' + (l.stream === 'stderr' ? ' stderr' : '') + '"><span class="ts">' + escHtml(l.timestamp||'') + '</span>' + escHtml(l.line) + '</div>'; });
  if (logs.length === 0) html = '<div style="color:var(--muted);text-align:center;padding:20px">No logs yet</div>';
  viewer.innerHTML = html;
}

function renderCmdLogs(id) {
  const viewer = document.getElementById('log-viewer-' + id);
  if (!viewer) return;
  const logs = cmdLogEntries[id] || [];
  let html = '';
  logs.forEach(l => { html += '<div class="log-line' + (l.stream === 'stderr' ? ' stderr' : '') + '"><span class="ts">' + escHtml(l.timestamp||'') + '</span>' + escHtml(l.line) + '</div>'; });
  if (logs.length === 0) html = '<div style="color:var(--muted);text-align:center;padding:20px">No output yet</div>';
  viewer.innerHTML = html;
}

function escHtml(s) { if (!s) return ''; const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
function escAttr(s) { return String(s).replace(/'/g, "\\'").replace(/"/g, '&quot;'); }

// Check auth on load
(async () => {
  const res = await fetch('/api/services');
  if (res.ok) {
    authenticated = true;
    services = await res.json();
    await refreshCommands();
    startPolling();
  }
  render();
})();
</script>
</body>
</html>
` + ""
