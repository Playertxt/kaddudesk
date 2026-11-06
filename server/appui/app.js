// KadduDesk app home: Devices (filter by OS), Anywhere, This device, Guide.
// Identical on Windows, Linux and Android — the native app shells just host this UI.
'use strict';
const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => Array.from(el.querySelectorAll(s));
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
let toastT;
function toast(m, ms = 3000) { const t = $('#toast'); t.textContent = m; t.hidden = false; clearTimeout(toastT); toastT = setTimeout(() => (t.hidden = true), ms); }

const ICON = {
  Windows: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M3 5.5 10.5 4.4v7.1H3zM11.5 4.3 21 3v8.5h-9.5zM3 12.5h7.5v7.1L3 18.5zM11.5 12.5H21V21l-9.5-1.3z"/></svg>',
  Linux: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2c-2.3 0-3.6 2-3.6 4.6 0 1.6.4 2.6-.6 4.2-1.2 1.8-2.8 3.7-2.8 6.1 0 .8.2 1.5.6 2.1-.7.4-1.6.9-1.6 1.6 0 .9 1.6 1.2 3.4 1.3 1 .1 1.8-.3 2.4-.8.7.2 1.4.3 2.2.3s1.5-.1 2.2-.3c.6.5 1.4.9 2.4.8 1.8-.1 3.4-.4 3.4-1.3 0-.7-.9-1.2-1.6-1.6.4-.6.6-1.3.6-2.1 0-2.4-1.6-4.3-2.8-6.1-1-1.6-.6-2.6-.6-4.2C15.6 4 14.3 2 12 2zm-1.6 4.2c.5 0 .8.6.8 1.2s-.3 1-.8 1-.8-.4-.8-1 .3-1.2.8-1.2zm3.2 0c.5 0 .8.6.8 1.2s-.3 1-.8 1-.8-.4-.8-1 .3-1.2.8-1.2zM12 9.3c.9 0 2 .5 2 .9s-1.1 1.1-2 1.1-2-.7-2-1.1 1.1-.9 2-.9z"/></svg>',
  Android: '<svg viewBox="0 0 24 24" fill="currentColor"><path d="M6 9h12v8a1.5 1.5 0 0 1-1.5 1.5H15V21a1.2 1.2 0 0 1-2.4 0v-2.5h-1.2V21A1.2 1.2 0 0 1 9 21v-2.5H7.5A1.5 1.5 0 0 1 6 17zM3.8 9.2a1.2 1.2 0 0 1 2.4 0v5.6a1.2 1.2 0 0 1-2.4 0zm14 0a1.2 1.2 0 0 1 2.4 0v5.6a1.2 1.2 0 0 1-2.4 0zM6.1 8a6 6 0 0 1 3.2-4.6L8.4 1.9a.4.4 0 0 1 .7-.4l1 1.6a6.3 6.3 0 0 1 3.8 0l1-1.6a.4.4 0 0 1 .7.4l-.9 1.5A6 6 0 0 1 17.9 8zm3.6-2.4a.7.7 0 1 0 0 1.4.7.7 0 0 0 0-1.4zm4.6 0a.7.7 0 1 0 0 1.4.7.7 0 0 0 0-1.4z"/></svg>',
  hub: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><circle cx="4" cy="5" r="2"/><circle cx="20" cy="5" r="2"/><circle cx="4" cy="19" r="2"/><circle cx="20" cy="19" r="2"/><path d="M6 6l4 4M18 6l-4 4M6 18l4-4M18 18l-4-4"/></svg>',
  other: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></svg>',
  devices: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="4" width="14" height="10" rx="2"/><rect x="17" y="8" width="5" height="12" rx="1.5"/><path d="M6 18h6"/></svg>',
  anywhere: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3.2 3 14.8 0 18M12 3c-3 3.2-3 14.8 0 18"/></svg>',
  local: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 3l8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7z"/><path d="M9 12l2 2 4-4"/></svg>',
  guide: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 4h6a3 3 0 0 1 3 3v13a2.5 2.5 0 0 0-2.5-2.5H4zM20 4h-6a3 3 0 0 0-1 .2"/><path d="M20 4v13.5h-6.5"/></svg>',
};
const TABS = { devices: 'Devices', anywhere: 'Anywhere', local: 'This device', guide: 'Guide' };
const osKey = (d) => (d.kind === 'hub' ? 'hub' : ['Windows', 'Linux', 'Android'].includes(d.os) ? d.os : 'other');
const iconFor = (d) => ICON[osKey(d)] || ICON.other;

async function api(p, body) {
  const r = await fetch('/app/api/' + p, { method: body ? 'POST' : 'GET', headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined, cache: 'no-store' });
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || 'Error ' + r.status);
  return j;
}

const S = { tab: localStorage.kd_tab || 'devices', filter: localStorage.kd_filter || 'all', q: '', devices: [], state: null, poll: null };

// ------------------------------------------------------------------ shell
function setTab(t) {
  S.tab = t; localStorage.kd_tab = t;
  $$('.tab').forEach((b) => b.classList.toggle('on', b.dataset.tab === t));
  clearInterval(S.poll);
  const v = $('#view');
  v.className = '';
  if (t === 'devices') return viewDevices(v);
  if (t === 'anywhere') return viewAnywhere(v);
  if (t === 'local') return viewLocal(v);
  return viewGuide(v);
}

async function boot() {
  $$('.tab').forEach((b) => { b.innerHTML = ICON[b.dataset.tab] + '<span>' + TABS[b.dataset.tab] + '</span>'; b.onclick = () => setTab(b.dataset.tab); });
  try { S.state = await api('state'); } catch (e) { S.state = { platform: '?', name: '' }; }
  $('#meName').textContent = (S.state.name || '') + ' · ' + S.state.platform;
  const want = new URLSearchParams(location.hash.slice(1)).get('tab');
  setTab(want && TABS[want] ? want : S.tab);
}

// ------------------------------------------------------------------ devices
async function viewDevices(v) {
  v.innerHTML = `<h1 class="pg">Devices</h1><p class="pgsub">Pick the device you want to control. Devices on your Wi-Fi appear by themselves; turn on <b>Anywhere</b> to reach them from any network.</p>
    <div class="chips" id="chips"></div>
    <div class="toolbar"><input id="q" placeholder="Search by name or ID…" value="${esc(S.q)}"><button class="btn primary" id="add">＋ Add device</button></div>
    <div id="lists"><div class="empty">Looking for devices…</div></div>`;
  $('#q').oninput = (e) => { S.q = e.target.value; drawDevices(); };
  $('#add').onclick = addDialog;
  await refreshDevices();
  S.poll = setInterval(refreshDevices, 3500);
}

async function refreshDevices() {
  try { S.devices = (await api('devices')).devices || []; } catch (e) { return; }
  drawDevices();
}

function drawDevices() {
  const chips = $('#chips'); if (!chips) return;
  const count = (f) => S.devices.filter((d) => f === 'all' || (f === 'hub' ? d.kind === 'hub' : d.kind !== 'hub' && d.os === f)).length;
  const F = [['all', 'All', ICON.devices], ['Windows', 'Windows', ICON.Windows], ['Linux', 'Linux', ICON.Linux], ['Android', 'Android', ICON.Android], ['hub', 'Hubs', ICON.hub]];
  chips.innerHTML = F.map(([k, l, ic]) => `<button class="chip ${S.filter === k ? 'on' : ''}" data-f="${k}">${ic}${l} <span class="n">${count(k)}</span></button>`).join('');
  $$('.chip', chips).forEach((c) => c.onclick = () => { S.filter = c.dataset.f; localStorage.kd_filter = S.filter; drawDevices(); });
  const q = S.q.toLowerCase().replace(/\s/g, '');
  const list = S.devices.filter((d) => (S.filter === 'all' || (S.filter === 'hub' ? d.kind === 'hub' : d.kind !== 'hub' && d.os === S.filter))
    && (!q || (d.name || '').toLowerCase().replace(/\s/g, '').includes(q) || (d.id || '').includes(q) || (d.addr || '').includes(q)));
  const sec = { saved: 'My devices', nearby: 'Nearby on this network', anywhere: 'Anywhere (your account, any network)' };
  const L = $('#lists');
  let html = '';
  for (const k of ['saved', 'nearby', 'anywhere']) {
    const items = list.filter((d) => d.section === k);
    if (!items.length && k !== 'saved') continue;
    html += `<h2 class="sec">${sec[k]}</h2><div class="grid">`;
    if (!items.length) html += `<div class="empty">${S.devices.length ? 'Nothing here for this filter.' : 'No saved devices yet. Pick one below, or tap <b>＋ Add device</b>.'}</div>`;
    for (const d of items) {
      const route = d.route === 'mesh' ? '<span class="tag net">Internet</span>' : '<span class="tag wifi">Wi-Fi</span>';
      const sub = d.kind === 'hub' ? 'Hub · all your devices, workspaces' : (d.pretty ? 'ID ' + esc(d.pretty) : esc(d.addr));
      html += `<div class="dev ${d.online || k !== 'saved' ? '' : 'off'}" data-key="${esc(d.key)}">
        <div class="top"><div class="osic ${osKey(d)}">${iconFor(d)}<i class="${d.online ? 'on' : ''}"></i></div>
        <div class="meta"><b>${esc(d.name || d.addr)}</b><div class="sub">${route}${esc(d.kind === 'hub' ? 'Hub' : (d.os || 'Device'))} · ${sub}</div></div></div>
        <div class="acts"><button class="btn primary" data-a="go">${d.kind === 'hub' ? 'Open hub' : 'Connect'}</button>
        ${d.saved && d.wake && !d.online ? '<button class="btn" data-a="wake">⏰ Wake</button>' : ''}
        ${d.saved ? '<button class="btn more" data-a="more">⋯</button>' : ''}</div></div>`;
    }
    html += '</div>';
  }
  if (!list.length && !S.devices.length) html += `<p class="pgsub" style="margin-top:18px">Nothing found yet. Make sure KadduDesk is running on the other device with <b>sharing on</b>, and that both are on the same Wi-Fi — or use the <b>Anywhere</b> tab.</p>`;
  L.innerHTML = html;
  $$('.dev', L).forEach((el) => {
    const d = S.devices.find((x) => x.key === el.dataset.key);
    el.querySelector('[data-a=go]').onclick = () => openDevice(d);
    const w = el.querySelector('[data-a=wake]'); if (w) w.onclick = async () => { try { await api('wake', { key: d.key }); toast('Wake-on-LAN sent to ' + d.name); } catch (e) { toast(e.message); } };
    const m = el.querySelector('[data-a=more]'); if (m) m.onclick = (e) => moreMenu(e, d);
  });
}

function moreMenu(ev, d) {
  closeMenu();
  const m = document.createElement('div'); m.className = 'menu'; m.id = 'menu';
  m.innerHTML = '<button data-a="rename">Rename</button><button data-a="remove" class="danger">Remove</button>';
  document.body.appendChild(m);
  const r = ev.target.getBoundingClientRect();
  m.style.left = Math.min(r.left, innerWidth - 190) + 'px'; m.style.top = Math.min(r.bottom + 4, innerHeight - 100) + 'px';
  m.querySelector('[data-a=rename]').onclick = async () => { closeMenu(); const n = prompt('Name for this device:', d.name); if (n) { await api('rename', { key: d.key, name: n }); refreshDevices(); } };
  m.querySelector('[data-a=remove]').onclick = async () => { closeMenu(); if (confirm('Remove ' + d.name + ' from this list?')) { await api('remove', { key: d.key }); refreshDevices(); } };
  setTimeout(() => document.addEventListener('click', closeMenu, { once: true }), 0);
}
function closeMenu() { const m = $('#menu'); if (m) m.remove(); }

function go(kind, key) { location.href = kind === 'hub' ? '/app/hubui/' + key + '/' : '/app/s/' + key + '/'; }

async function openDevice(d) {
  if (d.saved) return go(d.kind, d.key);
  await connectTo(d.route, d.addr, d.name);
}

// probe → confirm fingerprint (first time) → save → open
async function connectTo(route, addr, label, autoTrust) {
  toast('Connecting to ' + (label || addr) + '…', 9000);
  let p;
  try { p = await api('probe', { route, addr }); } catch (e) { toast(e.message, 6000); return; }
  $('#toast').hidden = true;
  if (!autoTrust && p.known !== p.fp && !(await trustDialog(p))) return;
  const t = await api('trust', { route: p.route, addr: p.addr, fp: p.fp, info: p.info });
  go(t.kind, t.key);
}

function trustDialog(p) {
  return new Promise((res) => {
    const pretty = p.fp.match(/.{4}/g).slice(0, 8).join(' ');
    const name = (p.info && p.info.name) || p.addr;
    const m = document.createElement('div'); m.className = 'modal';
    m.innerHTML = `<div class="card"><h1 style="font-size:19px;margin:0 0 6px">${p.known ? '⚠ This device changed' : 'Trust ' + esc(name) + '?'}</h1>
      ${p.known ? `<div class="warnbox">Its identity is different from last time. If you didn't reinstall KadduDesk on it, someone may be intercepting the connection — cancel.</div>` : ''}
      <p class="muted">First connection. Check that this matches <b>Certificate fingerprint</b> in KadduDesk → <b>This device</b> on ${esc(name)}:</p>
      <div class="fpbox">${pretty}</div>
      <div class="row gap end"><button class="btn" id="no">Cancel</button><button class="btn primary" id="yes">Trust &amp; connect</button></div></div>`;
    document.body.appendChild(m);
    $('#yes', m).onclick = () => { m.remove(); res(true); };
    $('#no', m).onclick = () => { m.remove(); res(false); };
  });
}

function addDialog() {
  const m = document.createElement('div'); m.className = 'modal';
  m.innerHTML = `<div class="card"><h1 style="font-size:19px;margin:0 0 8px">Add a device</h1>
    <p class="muted tiny">Enter the address shown in KadduDesk → <b>This device</b> on the other device.</p>
    <form id="f" style="display:grid;gap:10px;margin-top:10px">
      <input id="a" placeholder="e.g. 192.168.1.20  or  100.101.102.103" required autocapitalize="off" spellcheck="false">
      <select id="k"><option value="47800">A device (port 47800)</option><option value="47820">A hub (port 47820)</option></select>
      <label class="check"><input type="checkbox" id="mesh"> It's on my <b>Anywhere</b> network (100.x address)</label>
      <div class="row gap end"><button class="btn" type="button" id="no">Cancel</button><button class="btn primary">Connect</button></div>
    </form></div>`;
  document.body.appendChild(m);
  $('#a', m).focus();
  $('#a', m).oninput = (e) => { $('#mesh', m).checked = /^100\./.test(e.target.value.trim()); };
  $('#no', m).onclick = () => m.remove();
  $('#f', m).onsubmit = (e) => {
    e.preventDefault();
    let a = $('#a', m).value.trim();
    if (!/:\d+$/.test(a)) a += ':' + $('#k', m).value;
    const route = $('#mesh', m).checked ? 'mesh' : 'lan';
    m.remove();
    connectTo(route, a);
  };
}

// ------------------------------------------------------------------ anywhere (mesh)
async function viewAnywhere(v) {
  v.innerHTML = `<h1 class="pg">Connect from anywhere</h1>
    <p class="pgsub">Reach your devices from any network in the world — other Wi-Fi, mobile data, behind any router — with no router setup. Connections are end-to-end encrypted and go directly between your devices whenever possible.</p>
    <div class="panel"><div class="big-toggle"><div><h3>Anywhere</h3><div class="muted tiny" id="mmode"></div></div><label class="switch"><input type="checkbox" id="mon"><span></span></label></div>
      <div id="mstate"></div></div>
    <div class="panel"><h3>How it works</h3><ol class="steps">
      <li>Turn on <b>Anywhere</b> on every device you use — the PC you want to control <b>and</b> the phone or laptop you control it from.</li>
      <li>Tap <b>Sign in</b> and use the <b>same account</b> (Google, Microsoft, GitHub or Apple) on every device. This is free.</li>
      <li>Your devices then show up under <b>Devices → Anywhere</b>, wherever they are.</li></ol>
      <p class="muted tiny" style="margin-top:10px">KadduDesk uses the Tailscale network for this (WireGuard encryption, free for personal use). Sign-in happens once per device in your browser's account page; after that everything stays inside KadduDesk.</p></div>`;
  $('#mon').onchange = async (e) => {
    try { await api('mesh/enable', { on: e.target.checked }); } catch (er) { toast(er.message); }
    setTimeout(refreshMesh, 600);
  };
  await refreshMesh();
  S.poll = setInterval(refreshMesh, 2500);
}

async function refreshMesh() {
  let st;
  try { [st, S.state] = await Promise.all([api('mesh/status'), api('state')]); } catch (e) { return; }
  if (!$('#mstate')) return;
  $('#mon').checked = !!st.enabled;
  const mode = (S.state && S.state.meshMode) || '';
  $('#mmode').textContent = st.enabled ? (mode === 'host' ? 'Using this PC\'s sharing service (one sign-in for the whole PC)' : 'On for this device') : 'Off';
  let html = '';
  if (!st.enabled) html = `<div class="state"><span class="dot"></span>Off — only devices on the same Wi-Fi can connect.</div>`;
  else if (st.state === 'Running') {
    html = `<div class="state"><span class="dot ok"></span><div>Connected${st.user ? ' as <b>' + esc(st.user) + '</b>' : ''}${st.ips && st.ips[0] ? ' · this device: <b>' + esc(st.ips[0]) + '</b>' : ''}</div></div>
      <div class="row gap"><button class="btn" id="mdev">Show my devices</button><button class="btn ghost" id="mout">Sign out</button></div>`;
  } else if (st.state === 'NeedsLogin' || st.state === 'NoState' || st.state === 'Stopped') {
    html = `<div class="state"><span class="dot warn"></span>Sign in once to connect this device.</div><button class="btn primary" id="min">Sign in</button>`;
  } else html = `<div class="state"><span class="dot warn"></span>${esc(st.state || 'Starting')}…${st.error ? ' ' + esc(st.error) : ''}</div>`;
  $('#mstate').innerHTML = html;
  const mi = $('#min'); if (mi) mi.onclick = signIn;
  const mo = $('#mout'); if (mo) mo.onclick = async () => { if (confirm('Sign this device out of Anywhere?')) { await api('mesh/logout', {}); refreshMesh(); } };
  const md = $('#mdev'); if (md) md.onclick = () => { S.filter = 'all'; setTab('devices'); };
}

async function signIn() {
  const b = $('#min'); if (b) { b.disabled = true; b.textContent = 'Opening sign-in…'; }
  try {
    const r = await api('mesh/login', {});
    if (r.url) { openExternal(r.url); toast('Finish signing in on the page that opened, then come back here.', 7000); }
  } catch (e) { toast(e.message, 6000); }
  if (b) { b.disabled = false; b.textContent = 'Sign in'; }
}

function openExternal(u) {
  if (KD.openExternal) KD.openExternal(u); else window.open(u, '_blank');
}

// ------------------------------------------------------------------ this device
function viewLocal(v) {
  v.className = 'flush';
  v.innerHTML = '<iframe src="/app/local/" title="This device"></iframe>';
}
window.addEventListener('message', async (e) => {
  if (e.origin !== location.origin || !e.data) return;
  if (e.data.kd === 'openLocalHub') await connectTo('lan', '127.0.0.1:' + e.data.port, 'this hub', true);
});

// ------------------------------------------------------------------ guide
function viewGuide(v) {
  const plat = (S.state && S.state.platform) || '';
  v.innerHTML = `<h1 class="pg">Guide</h1><p class="pgsub">Step-by-step help for everything KadduDesk does. You're on <b>${esc(plat)}</b> — the steps below match this app.</p><div class="guide" id="g"></div>`;
  $('#g').innerHTML = (window.KD_GUIDE || []).filter((x) => !x.only || x.only.includes(plat)).map((x, i) => `<details ${i === 0 ? 'open' : ''}><summary><span class="ico">${x.icon}</span>${esc(x.title)}<span class="chev">›</span></summary><div class="body">${x.html}</div></details>`).join('');
}

window.kdBack = () => {
  const m = $('.modal'); if (m) { m.remove(); return true; }
  if (S.tab !== 'devices') { setTab('devices'); return true; }
  return false;
};
boot();
