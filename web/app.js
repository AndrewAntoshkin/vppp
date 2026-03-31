const app = document.getElementById('app');
let apiKey = localStorage.getItem('vpn_api_key') || '';
let peers = [];
let refreshInterval;

function api(method, path, body) {
  const opts = {
    method,
    headers: { 'X-API-Key': apiKey, 'Content-Type': 'application/json' },
  };
  if (body) opts.body = JSON.stringify(body);
  return fetch(path, opts).then(async (r) => {
    if (r.status === 401) {
      localStorage.removeItem('vpn_api_key');
      apiKey = '';
      render();
      throw new Error('Unauthorized');
    }
    if (!r.ok) {
      const err = await r.json().catch(() => ({ error: r.statusText }));
      throw new Error(err.error || r.statusText);
    }
    const ct = r.headers.get('content-type') || '';
    if (ct.includes('json')) return r.json();
    return r.text();
  });
}

function toast(msg, isError) {
  const el = document.createElement('div');
  el.className = 'toast' + (isError ? ' error' : '');
  el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 3000);
}

function formatBytes(n) {
  if (!n || n === '0') return '-';
  const num = parseInt(n, 10);
  if (isNaN(num)) return n;
  if (num >= 1 << 30) return (num / (1 << 30)).toFixed(1) + ' GB';
  if (num >= 1 << 20) return (num / (1 << 20)).toFixed(1) + ' MB';
  if (num >= 1 << 10) return (num / (1 << 10)).toFixed(1) + ' KB';
  return num + ' B';
}

function timeAgo(ts) {
  if (!ts || ts === '0') return '-';
  const sec = Math.floor(Date.now() / 1000) - parseInt(ts, 10);
  if (sec < 0 || isNaN(sec)) return '-';
  if (sec < 60) return sec + 's ago';
  if (sec < 3600) return Math.floor(sec / 60) + 'm ago';
  if (sec < 86400) return Math.floor(sec / 3600) + 'h ago';
  return Math.floor(sec / 86400) + 'd ago';
}

function isOnline(ts) {
  if (!ts || ts === '0') return false;
  return (Date.now() / 1000 - parseInt(ts, 10)) < 180;
}

async function loadPeers() {
  try {
    peers = await api('GET', '/api/peers');
    if (!Array.isArray(peers)) peers = [];
  } catch (e) {
    console.error('load peers:', e);
    peers = [];
  }
  renderPeers();
}

function render() {
  if (!apiKey) {
    renderLogin();
  } else {
    renderMain();
    loadPeers();
    clearInterval(refreshInterval);
    refreshInterval = setInterval(loadPeers, 5000);
  }
}

function renderLogin() {
  app.innerHTML = `
    <div class="login-form">
      <h2>VPN Panel</h2>
      <p style="color: var(--text-muted)">Enter your API key to continue</p>
      <input type="password" id="apiKeyInput" placeholder="API Key" autofocus>
      <button onclick="doLogin()">Connect</button>
    </div>
  `;
  document.getElementById('apiKeyInput').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') doLogin();
  });
}

function doLogin() {
  const key = document.getElementById('apiKeyInput').value.trim();
  if (!key) return;
  apiKey = key;
  localStorage.setItem('vpn_api_key', key);
  render();
}

function renderMain() {
  app.innerHTML = `
    <header>
      <h1><span>VPN</span> Panel</h1>
      <div class="server-info" id="serverInfo"></div>
    </header>
    <div class="actions">
      <input type="text" id="peerName" placeholder="Peer name (e.g. Andrew iPhone)" style="flex:1">
      <button onclick="addPeer()">Add Peer</button>
      <button class="secondary" onclick="logout()" style="margin-left:auto">Logout</button>
    </div>
    <div class="peers" id="peerList"></div>
  `;
  document.getElementById('peerName').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') addPeer();
  });
  loadServerInfo();
}

async function loadServerInfo() {
  try {
    const info = await api('GET', '/api/server');
    const el = document.getElementById('serverInfo');
    if (el) {
      el.innerHTML = `${info.endpoint}:${info.listen_port}<br>${info.address}`;
    }
  } catch (e) { /* ignore */ }
}

function renderPeers() {
  const container = document.getElementById('peerList');
  if (!container) return;

  if (peers.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <h3>No peers yet</h3>
        <p>Add your first peer to get started</p>
      </div>
    `;
    return;
  }

  container.innerHTML = peers.map((p) => {
    const online = isOnline(p.latest_handshake);
    const endpoint = p.endpoint && p.endpoint !== '(none)' ? p.endpoint : '-';
    return `
      <div class="peer-card">
        <div class="peer-header">
          <span class="peer-name">
            <span class="status-dot ${online ? 'online' : 'offline'}"></span>
            ${esc(p.name)}
          </span>
          <div class="peer-actions">
            <button class="secondary" onclick="showConfig(${p.id})">Config</button>
            <button class="secondary" onclick="showQR(${p.id})">QR</button>
            <button class="danger" onclick="removePeer(${p.id}, '${esc(p.name)}')">Remove</button>
          </div>
        </div>
        <div class="peer-details">
          <div class="peer-detail">
            <label>IP Address</label>
            <div class="value">${p.allowed_ips}</div>
          </div>
          <div class="peer-detail">
            <label>Endpoint</label>
            <div class="value">${endpoint}</div>
          </div>
          <div class="peer-detail">
            <label>Handshake</label>
            <div class="value">${timeAgo(p.latest_handshake)}</div>
          </div>
          <div class="peer-detail">
            <label>Transfer</label>
            <div class="value">${formatBytes(p.transfer_rx)} / ${formatBytes(p.transfer_tx)}</div>
          </div>
        </div>
      </div>
    `;
  }).join('');
}

async function addPeer() {
  const input = document.getElementById('peerName');
  const name = input.value.trim();
  if (!name) return input.focus();

  try {
    const result = await api('POST', '/api/peers', { name });
    input.value = '';
    toast(`Peer "${name}" added`);
    showNewPeerModal(result);
    await loadPeers();
  } catch (e) {
    toast(e.message, true);
  }
}

function showNewPeerModal(result) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.onclick = (e) => { if (e.target === overlay) overlay.remove(); };
  overlay.innerHTML = `
    <div class="modal">
      <h3>Peer Created: ${esc(result.peer.name)}</h3>
      <p style="color:var(--text-muted);font-size:0.85rem;margin-bottom:0.5rem">
        Save this config or scan the QR code with the WireGuard app.
        The private key will not be shown again.
      </p>
      ${result.qr ? `
        <div class="qr-container">
          <img src="data:image/png;base64,${result.qr}" alt="QR Code">
        </div>
      ` : ''}
      <pre>${esc(result.config)}</pre>
      <div class="modal-actions">
        <button class="secondary" onclick="copyText(this, \`${btoa(result.config)}\`)">Copy Config</button>
        <button onclick="this.closest('.modal-overlay').remove()">Close</button>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
}

async function showConfig(id) {
  try {
    const config = await api('GET', `/api/peers/${id}/config`);
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.onclick = (e) => { if (e.target === overlay) overlay.remove(); };
    overlay.innerHTML = `
      <div class="modal">
        <h3>Client Config</h3>
        <pre>${esc(config)}</pre>
        <div class="modal-actions">
          <button class="secondary" onclick="copyText(this, \`${btoa(config)}\`)">Copy</button>
          <button onclick="this.closest('.modal-overlay').remove()">Close</button>
        </div>
      </div>
    `;
    document.body.appendChild(overlay);
  } catch (e) {
    toast(e.message, true);
  }
}

async function showQR(id) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.onclick = (e) => { if (e.target === overlay) overlay.remove(); };
  overlay.innerHTML = `
    <div class="modal">
      <h3>QR Code</h3>
      <div class="qr-container">
        <img src="/api/peers/${id}/qr?api_key=${encodeURIComponent(apiKey)}" alt="QR Code">
      </div>
      <div class="modal-actions">
        <button onclick="this.closest('.modal-overlay').remove()">Close</button>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
}

async function removePeer(id, name) {
  if (!confirm(`Remove peer "${name}"? This cannot be undone.`)) return;
  try {
    await api('DELETE', `/api/peers/${id}`);
    toast(`Peer "${name}" removed`);
    await loadPeers();
  } catch (e) {
    toast(e.message, true);
  }
}

function copyText(btn, b64) {
  const text = atob(b64);
  navigator.clipboard.writeText(text).then(() => {
    btn.textContent = 'Copied!';
    setTimeout(() => btn.textContent = 'Copy', 1500);
  });
}

function logout() {
  localStorage.removeItem('vpn_api_key');
  apiKey = '';
  clearInterval(refreshInterval);
  render();
}

function esc(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

render();
