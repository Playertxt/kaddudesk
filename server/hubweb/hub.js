// KadduDesk Hub web app: sign-in (password + 2FA), account security, workspaces,
// multi-device live view, administration. No inline scripts (strict CSP).
'use strict';
const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => Array.from(el.querySelectorAll(s));
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const osIcon = (os) => /linux/i.test(os || '') ? '🐧' : /android/i.test(os || '') ? '📱' : '🖥';
const ago = (t) => { if (!t || String(t).startsWith('0001')) return 'never'; const s = (Date.now() - new Date(t)) / 1000; return s < 90 ? 'just now' : s < 3600 ? Math.floor(s / 60) + ' min ago' : s < 86400 ? Math.floor(s / 3600) + ' h ago' : Math.floor(s / 86400) + ' d ago'; };
let toastT;
function toast(m, ms = 2800) { const t = $('#toast'); t.textContent = m; t.hidden = false; clearTimeout(toastT); toastT = setTimeout(() => (t.hidden = true), ms); }
const app = $('#app');

async function api(path, body, method) {
  const post = body !== undefined || method === 'POST';
  const r = await fetch('hub/api/' + path, {
    method: post ? 'POST' : 'GET', cache: 'no-store', credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-KD-CSRF': '1' },
    body: post ? JSON.stringify(body || {}) : undefined,
  });
  let j = {};
  try { j = await r.json(); } catch (e) {}
  if (!r.ok) { const e = new Error(j.error || 'Error ' + r.status); e.status = r.status; e.data = j; throw e; }
  return j;
}

const S = { me: null, view: 'ws', adminTab: 'users', devices: [], workspaces: [], users: [], editWs: null, poll: null, hubName: 'Hub' };
const isAdmin = () => S.me && (S.me.user.role === 'owner' || S.me.user.role === 'admin');
const native = KD.kind !== 'browser';

// ------------------------------------------------------------------ boot / routing
async function start() {
  clearInterval(S.poll);
  let st = { hasAccounts: true, name: 'Hub' };
  try { st = await api('auth/state'); } catch (e) {}
  S.hubName = st.name;
  document.title = st.name + ' · KadduDesk Hub';
  if (!st.hasAccounts) return renderNoAccounts();
  try { S.me = await api('me'); } catch (e) { return renderLogin(); }
  if (S.me.mustChange) return renderChangeTemp();
  if (S.me.needs2fa) return render2FASetup(true);
  renderMain();
}

function card(inner, wide) {
  app.innerHTML = `<section class="screen"><div class="card ${wide ? 'wide' : ''}"><img src="icon.png" class="logo" alt="">${inner}
    ${native ? '<button class="btn ghost small" id="home" style="margin-top:12px">← Back to my PCs</button>' : ''}</div></section>`;
  const h = $('#home'); if (h) h.onclick = () => KD.home();
}

function renderNoAccounts() {
  card(`<h1>${esc(S.hubName)} · Hub</h1><p class="muted">This hub has no accounts yet.</p>
    <p class="muted tiny">On the hub PC, open the <b>KadduDesk control panel → Hub → Accounts</b> and create the owner account. For security this can only be done on that PC.</p>
    <button class="btn" id="retry">I've done it</button>`);
  $('#retry').onclick = start;
}

// ------------------------------------------------------------------ sign in
function renderLogin(msg) {
  card(`<h1>${esc(S.hubName)} · Hub</h1><p class="muted">Sign in to see and control your devices.</p>
    <form class="form" id="f" autocomplete="on">
      <input id="u" name="username" autocomplete="username" placeholder="Username" required autocapitalize="off" spellcheck="false">
      <input id="p" name="password" type="password" autocomplete="current-password" placeholder="Password" required>
      <label class="check"><input id="rem" type="checkbox"> Keep me signed in on this device</label>
      <button class="btn primary" id="go">Sign in</button>
    </form><p class="err" id="err">${esc(msg || '')}</p>
    <p class="tiny muted">Protected by Argon2id passwords, two-factor authentication and lockout after repeated failures.</p>`);
  setTimeout(() => $('#u').focus(), 30);
  $('#f').onsubmit = async (e) => {
    e.preventDefault();
    $('#go').disabled = true; $('#err').textContent = '';
    try {
      const r = await api('auth/login', { username: $('#u').value, password: $('#p').value, remember: $('#rem').checked });
      if (r.mfa) return renderMFA(r.ticket);
      start();
    } catch (er) { $('#err').textContent = er.message; $('#go').disabled = false; $('#p').value = ''; }
  };
}

function renderMFA(ticket) {
  card(`<h1>Two-factor check</h1><p class="muted">Enter the 6-digit code from your authenticator app.</p>
    <form class="form" id="f" autocomplete="off">
      <input id="c" inputmode="numeric" autocomplete="one-time-code" placeholder="123 456" maxlength="11" style="font-size:22px;letter-spacing:.3em;text-align:center">
      <button class="btn primary" id="go">Verify</button>
      <button class="btn ghost small" type="button" id="rc">Use a recovery code instead</button>
    </form><p class="err" id="err"></p>`);
  setTimeout(() => $('#c').focus(), 30);
  $('#rc').onclick = () => { $('#c').placeholder = 'xxxxx-xxxxx'; $('#c').inputMode = 'text'; $('#c').style.letterSpacing = '.08em'; $('#c').focus(); };
  $('#f').onsubmit = async (e) => {
    e.preventDefault();
    try {
      const r = await api('auth/mfa', { ticket, code: $('#c').value });
      if (r.recoveryLeft !== undefined && r.recoveryLeft <= 3 && r.recoveryLeft >= 0) toast(`Only ${r.recoveryLeft} recovery codes left. Make new ones in Account.`, 6000);
      start();
    } catch (er) {
      if (er.data && er.data.restart) return renderLogin(er.message);
      $('#err').textContent = er.message; $('#c').value = ''; $('#c').focus();
    }
  };
}

// ------------------------------------------------------------------ mandatory setup
function renderChangeTemp() {
  card(`<h1>Choose your password</h1><p class="muted">Your account was created with a temporary password. Pick your own to continue.</p>
    <form class="form" id="f">
      <input id="cur" type="password" autocomplete="current-password" placeholder="Temporary password" required>
      <input id="n1" type="password" autocomplete="new-password" placeholder="New password (10+ characters)" required>
      <input id="n2" type="password" autocomplete="new-password" placeholder="Repeat new password" required>
      <button class="btn primary">Save</button>
    </form><p class="err" id="err"></p>${logoutLink()}`);
  wireLogout();
  $('#f').onsubmit = async (e) => {
    e.preventDefault();
    if ($('#n1').value !== $('#n2').value) { $('#err').textContent = "The new passwords don't match"; return; }
    try { await api('me/password', { current: $('#cur').value, new: $('#n1').value }); toast('Password changed'); start(); }
    catch (er) { $('#err').textContent = er.message; }
  };
}

async function render2FASetup(mandatory) {
  let b;
  try { b = await api('me/2fa/begin', {}); } catch (e) { toast(e.message); return mandatory ? null : renderMain(); }
  card(`<h1>Turn on two-factor sign-in</h1>
    <p class="muted">${mandatory ? 'This hub requires 2FA. ' : ''}Scan with an authenticator app (Google Authenticator, Microsoft Authenticator, Authy, 1Password…), then enter the code it shows.</p>
    <img class="qr" src="${b.qr}" alt="QR code">
    <p class="tiny muted">Can't scan? Enter this key:<br><span class="mono">${esc(b.secret.match(/.{1,4}/g).join(' '))}</span></p>
    ${native && /Android/i.test(navigator.userAgent) ? `<p><a class="link" href="${esc(b.uri)}">Open in authenticator app on this phone</a></p>` : ''}
    <form class="form" id="f" autocomplete="off">
      <input id="c" inputmode="numeric" autocomplete="one-time-code" placeholder="6-digit code" maxlength="7" style="font-size:20px;letter-spacing:.3em;text-align:center">
      <button class="btn primary">Turn on 2FA</button>
      ${mandatory ? '' : '<button class="btn ghost" type="button" id="cancel">Cancel</button>'}
    </form><p class="err" id="err"></p>${mandatory ? logoutLink() : ''}`, true);
  wireLogout();
  const c = $('#cancel'); if (c) c.onclick = renderMain;
  $('#f').onsubmit = async (e) => {
    e.preventDefault();
    try { const r = await api('me/2fa/enable', { code: $('#c').value }); renderRecovery(r.recovery); }
    catch (er) { $('#err').textContent = er.message; }
  };
}

function renderRecovery(codes) {
  card(`<h1>Save your recovery codes</h1>
    <p class="muted">If you lose your phone, each code lets you sign in once. Store them somewhere safe (a password manager or on paper). They won't be shown again.</p>
    <div class="codes">${codes.map(esc).join('<br>')}</div>
    <div class="row gap" style="margin-top:12px;justify-content:center"><button class="btn" id="copy">Copy</button><button class="btn" id="dl">Download</button></div>
    <label class="check" style="justify-content:center"><input type="checkbox" id="ok"> I saved my recovery codes</label>
    <button class="btn primary" id="cont" disabled>Continue</button>`, true);
  const text = 'KadduDesk recovery codes (' + S.hubName + ')\n\n' + codes.join('\n') + '\n';
  $('#copy').onclick = async () => { try { await navigator.clipboard.writeText(text); toast('Copied'); } catch (e) { if (KD.setClipboard) { KD.setClipboard(text); toast('Copied'); } } };
  $('#dl').onclick = () => { const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([text], { type: 'text/plain' })); a.download = 'kaddudesk-recovery-codes.txt'; a.click(); };
  $('#ok').onchange = (e) => ($('#cont').disabled = !e.target.checked);
  $('#cont').onclick = start;
}

const logoutLink = () => '<button class="btn ghost small" id="lo" style="margin-top:10px">Sign out</button>';
function wireLogout() { const b = $('#lo'); if (b) b.onclick = logout; }
async function logout() { try { await api('auth/logout', {}); } catch (e) {} S.me = null; start(); }

// ------------------------------------------------------------------ main shell
function renderMain() {
  const u = S.me.user;
  app.innerHTML = `<div class="wrap">
    <div class="top"><img src="icon.png" alt="">
      <div><h1>${esc(S.me.hub)} · Hub</h1><div class="sub">Signed in as <b>${esc(u.username)}</b> <span class="badge">${esc(u.role)}</span> ${u.totp ? '<span class="badge ok">2FA on</span>' : '<span class="badge warn">2FA off</span>'}</div></div>
      <div class="sp"></div>
      ${native ? '<button class="btn small" id="home">My PCs</button>' : ''}
      <button class="btn small ghost" id="lo">Sign out</button>
    </div>
    <div class="nav">
      <button data-v="ws">Workspaces</button><button data-v="devices">All devices</button>
      <button data-v="account">Account &amp; security</button>${isAdmin() ? '<button data-v="admin">Administration</button>' : ''}
    </div>
    <div id="content"></div></div>`;
  wireLogout();
  const h = $('#home'); if (h) h.onclick = () => KD.home();
  $$('.nav button').forEach((b) => b.onclick = () => { S.view = b.dataset.v; S.editWs = null; renderView(); });
  renderView();
  S.poll = setInterval(() => { if (!document.hidden && !$('#multi') && (S.view === 'ws' || S.view === 'devices') && !S.editWs) refreshLists(); }, 4000);
}

function renderView() {
  $$('.nav button').forEach((b) => b.classList.toggle('on', b.dataset.v === S.view));
  const c = $('#content');
  c.innerHTML = '<div class="empty">Loading…</div>';
  ({ ws: viewWorkspaces, devices: viewDevices, account: viewAccount, admin: viewAdmin }[S.view] || viewWorkspaces)(c);
}

async function refreshLists() {
  try {
    const [d, w] = await Promise.all([api('devices'), api('workspaces')]);
    S.devices = d.devices; S.workspaces = w.workspaces;
    if (S.view === 'ws') drawWorkspaces($('#content')); else if (S.view === 'devices') drawDevices($('#content'));
  } catch (e) { if (e.status === 401) start(); }
}

// ------------------------------------------------------------------ workspaces
async function viewWorkspaces(c) { await refreshLists(); drawWorkspaces(c); }

function drawWorkspaces(c) {
  if (S.editWs) return;
  const ws = S.workspaces;
  c.innerHTML = `${isAdmin() ? `<div class="panel"><form class="inline" id="nw"><input id="nwn" placeholder="New workspace name, e.g. Home, Office, Gaming" maxlength="60" required><button class="btn primary">Create workspace</button></form></div>` : ''}
    <div class="grid" id="wg"></div>`;
  const g = $('#wg');
  if (!ws.length) g.innerHTML = `<div class="empty">${isAdmin() ? 'No workspaces yet. Create one, add devices to it, and open it to see all of them live at once.' : 'You haven\'t been added to a workspace yet. Ask the hub owner.'}</div>`;
  for (const w of ws) {
    const on = w.devices.filter((d) => d.online).length;
    const el = document.createElement('div');
    el.className = 'tile';
    el.innerHTML = `<div class="row1"><div class="ic">🗂</div><div class="meta"><b>${esc(w.name)}</b>
      <div class="tiny muted">${on} of ${w.devices.length} device(s) online · ${w.members.length} member(s)</div>
      <div class="tiny muted">${w.devices.slice(0, 4).map((d) => esc(d.name)).join(', ')}${w.devices.length > 4 ? '…' : ''}</div></div></div>
      <div class="acts"><button class="btn primary" data-a="open" ${on ? '' : 'disabled'}>Open (${on} live)</button>
      ${isAdmin() ? '<button class="btn" data-a="edit">Edit</button>' : ''}</div>`;
    el.querySelector('[data-a=open]').onclick = () => openMulti(w);
    const e = el.querySelector('[data-a=edit]'); if (e) e.onclick = () => editWorkspace(w);
    g.appendChild(el);
  }
  const f = $('#nw');
  if (f) f.onsubmit = async (e) => {
    e.preventDefault();
    try { const r = await api('admin/workspaces', { name: $('#nwn').value }); await refreshLists(); const w = S.workspaces.find((x) => x.id === r.id); if (w) editWorkspace(w); }
    catch (er) { toast(er.message); }
  };
}

async function editWorkspace(w) {
  S.editWs = w.id;
  let users = [];
  try { users = (await api('admin/users')).users; } catch (e) {}
  const all = (await api('devices')).devices;
  const inWs = new Set(w.devices.map((d) => d.id));
  const perm = Object.fromEntries(w.members.map((m) => [m.userId, m.perm]));
  const c = $('#content');
  c.innerHTML = `<div class="panel"><h2>Edit workspace</h2>
    <div class="form"><input id="wn" value="${esc(w.name)}" maxlength="60"></div>
    <h2 style="margin-top:16px">Devices</h2>
    <div>${all.map((d) => `<label class="chk"><input type="checkbox" data-dev="${esc(d.id)}" ${inWs.has(d.id) ? 'checked' : ''}> ${osIcon(d.os)} <b>${esc(d.name)}</b> <span class="idt">${esc(d.pretty)}</span> ${d.online ? '<span class="badge ok">online</span>' : '<span class="badge">offline</span>'}</label>`).join('') || '<div class="muted">No devices have joined the hub yet.</div>'}</div>
    <h2 style="margin-top:16px">Who can use it</h2>
    <p class="tiny muted">Owners and admins can always use every workspace.</p>
    <table>${users.filter((u) => u.role === 'member').map((u) => `<tr><td><b>${esc(u.username)}</b></td><td style="text-align:right"><select data-mem="${esc(u.id)}" style="width:auto">
      <option value="">No access</option><option value="view" ${perm[u.id] === 'view' ? 'selected' : ''}>View only</option><option value="control" ${perm[u.id] === 'control' ? 'selected' : ''}>Full control</option></select></td></tr>`).join('') || '<tr><td class="muted">No member accounts yet. Create them under Administration → Users.</td></tr>'}</table>
    <div class="row gap" style="margin-top:14px"><button class="btn primary" id="save">Save</button><button class="btn" id="cancel">Cancel</button><div class="sp"></div><button class="btn" id="del" style="color:var(--danger)">Delete workspace</button></div></div>`;
  $('#cancel').onclick = () => { S.editWs = null; renderView(); };
  $('#save').onclick = async () => {
    const devices = $$('[data-dev]').filter((x) => x.checked).map((x) => x.dataset.dev);
    const members = $$('[data-mem]').filter((x) => x.value).map((x) => ({ userId: x.dataset.mem, perm: x.value }));
    try { await api('admin/workspaces', { id: w.id, name: $('#wn').value, devices, members }); toast('Saved'); S.editWs = null; renderView(); }
    catch (er) { toast(er.message); }
  };
  $('#del').onclick = async () => {
    if (!confirm(`Delete workspace "${w.name}"? Devices stay on the hub.`)) return;
    await api('admin/workspaces', { id: w.id, delete: true }); S.editWs = null; renderView();
  };
}

// ------------------------------------------------------------------ all devices
async function viewDevices(c) { await refreshLists(); drawDevices(c); }

function drawDevices(c) {
  c.innerHTML = `<div class="panel" style="padding:10px 12px"><input id="q" placeholder="Search by name or 9-digit ID…" value="${esc(S.q || '')}"></div><div class="grid" id="dg"></div>`;
  $('#q').oninput = (e) => { S.q = e.target.value; drawDeviceGrid(); };
  drawDeviceGrid();
}

function drawDeviceGrid() {
  const q = (S.q || '').toLowerCase().replace(/\s/g, '');
  const list = S.devices.filter((d) => !q || d.name.toLowerCase().includes(q) || d.id.includes(q));
  const g = $('#dg'); if (!g) return;
  g.innerHTML = list.length ? '' : `<div class="empty">${q ? 'No match.' : 'No devices yet. On each PC: control panel → Join a hub.'}</div>`;
  for (const d of list) {
    const el = document.createElement('div');
    el.className = 'tile' + (d.online ? ' click' : ' off');
    el.innerHTML = `<div class="row1"><div class="ic">${osIcon(d.os)}<i class="${d.online ? 'on' : ''}"></i></div>
      <div class="meta"><b>${esc(d.name)}${d.self ? '<span class="badge">hub</span>' : ''}</b><div class="idt">${esc(d.pretty)}</div>
      <div class="tiny muted">${esc(d.os || '')} · ${d.perm === 'view' ? 'view only' : 'full control'}${d.online ? '' : ' · offline ' + ago(d.lastSeen)}</div></div></div>
      <div class="acts">${d.online ? '<button class="btn primary" data-a="go">Connect</button>' : ''}
      ${!d.online && d.wake && d.perm === 'control' ? '<button class="btn" data-a="wake">⏰ Wake</button>' : ''}
      ${isAdmin() && !d.self && !d.online ? '<button class="btn" data-a="rm">Remove</button>' : ''}</div>`;
    const go = () => { location.href = 'd/' + d.id + '/'; };
    if (d.online) el.onclick = (e) => { if (!e.target.closest('[data-a=wake],[data-a=rm]')) go(); };
    const w = el.querySelector('[data-a=wake]');
    if (w) w.onclick = async (e) => { e.stopPropagation(); try { await api('wake?id=' + d.id, {}); toast('Wake-on-LAN sent from the hub'); } catch (er) { toast(er.message); } };
    const rm = el.querySelector('[data-a=rm]');
    if (rm) rm.onclick = async (e) => { e.stopPropagation(); if (confirm('Remove ' + d.name + ' from the hub?')) { await api('admin/forget?id=' + d.id, {}); refreshLists(); } };
    g.appendChild(el);
  }
}

// ------------------------------------------------------------------ multi-device live view
let multi = null;
function openMulti(w) {
  clearInterval(S.poll);
  const devs = w.devices.filter((d) => d.online);
  const el = document.createElement('div');
  el.id = 'multi';
  el.innerHTML = `<div class="bar"><button class="btn small" id="mBack">← Back</button><b>${esc(w.name)}</b>
    <span class="tiny muted" id="mInfo">${devs.length} live · click a screen to control it</span><div class="sp"></div>
    <button class="btn small" id="mWall" hidden>▦ All screens</button><button class="btn small" id="mFull">⛶ Fullscreen</button></div>
    <div id="wall"></div>`;
  document.body.appendChild(el);
  const wall = $('#wall');
  multi = { w, focus: null, frames: {} };
  for (const d of devs) {
    const cell = document.createElement('div');
    cell.className = 'cell';
    cell.dataset.id = d.id;
    cell.innerHTML = `<iframe src="d/${d.id}/?embed=thumb" allow="fullscreen; clipboard-read; clipboard-write; autoplay; keyboard-map" title="${esc(d.name)}"></iframe>
      <div class="cover" title="Control ${esc(d.name)}"></div><div class="label">${osIcon(d.os)} ${esc(d.name)}<span class="idt">${esc(d.pretty)}</span>${d.perm === 'view' ? ' <span class="badge">view</span>' : ''}</div>`;
    cell.querySelector('.cover').onclick = () => focusDevice(d.id);
    multi.frames[d.id] = cell.querySelector('iframe');
    wall.appendChild(cell);
  }
  $('#mBack').onclick = closeMulti;
  $('#mWall').onclick = () => focusDevice(null);
  $('#mFull').onclick = () => { if (!document.fullscreenElement) el.requestFullscreen().catch(() => {}); else document.exitFullscreen(); };
}

function focusDevice(id) {
  if (!multi) return;
  multi.focus = id;
  $('#wall').classList.toggle('focus', !!id);
  $('#mWall').hidden = !id;
  for (const cell of $$('#wall .cell')) {
    const main = cell.dataset.id === id;
    cell.classList.toggle('main', main);
    const f = multi.frames[cell.dataset.id];
    try { f.contentWindow.postMessage({ kd: 'mode', mode: main ? 'main' : 'thumb' }, location.origin); } catch (e) {}
    if (main) setTimeout(() => f.focus(), 50);
  }
  const d = multi.w.devices.find((x) => x.id === id);
  $('#mInfo').textContent = d ? `Controlling ${d.name} · others stay live` : `${Object.keys(multi.frames).length} live · click a screen to control it`;
}

function closeMulti() {
  if (document.fullscreenElement) document.exitFullscreen();
  const el = $('#multi'); if (el) el.remove();
  multi = null;
  renderMain();
}

window.addEventListener('message', (e) => {
  if (e.origin !== location.origin || !multi || !e.data || !e.data.kd) return;
  const id = Object.keys(multi.frames).find((k) => multi.frames[k].contentWindow === e.source);
  if (!id) return;
  if (e.data.kd === 'ready') e.source.postMessage({ kd: 'mode', mode: multi.focus === id ? 'main' : 'thumb' }, location.origin);
  if (e.data.kd === 'closed' && multi.focus === id) focusDevice(null);
});

// ------------------------------------------------------------------ account & security
async function viewAccount(c) {
  const u = S.me.user;
  let sessions = [];
  try { sessions = (await api('me/sessions')).sessions || []; } catch (e) {}
  c.innerHTML = `
    <div class="panel"><h2>Password</h2>
      <form class="form" id="pf"><input id="cur" type="password" autocomplete="current-password" placeholder="Current password" required>
      <input id="n1" type="password" autocomplete="new-password" placeholder="New password (10+ characters, mixed)" required>
      <input id="n2" type="password" autocomplete="new-password" placeholder="Repeat new password" required>
      <button class="btn primary">Change password</button><span class="tiny muted">Changing it signs you out everywhere else.</span></form></div>
    <div class="panel"><h2>Two-factor authentication</h2>
      ${u.totp ? `<p>✅ On. Recovery codes left: <b>${u.recoveryLeft}</b></p>
        <div class="inline"><input id="rp" type="password" placeholder="Password to make new recovery codes"><button class="btn" id="rg">New recovery codes</button></div>
        ${S.me.security.require2fa ? '<p class="tiny muted">2FA is required on this hub and can\'t be turned off.</p>' : `<div class="inline" style="margin-top:10px"><input id="dp" type="password" placeholder="Password"><input id="dc" inputmode="numeric" placeholder="Current code"><button class="btn" id="d2" style="color:var(--danger)">Turn off 2FA</button></div>`}`
      : `<p>⚠ Off. Anyone with your password could sign in.</p><button class="btn primary" id="e2">Turn on 2FA</button>`}</div>
    <div class="panel"><h2>Signed-in sessions</h2><table><tr><th>Device</th><th>IP</th><th>Last active</th><th></th></tr>
      ${sessions.map((s) => `<tr><td>${esc(shortUA(s.ua))}${s.current ? ' <span class="badge ok">this one</span>' : ''}${s.remember ? ' <span class="badge">kept</span>' : ''}</td><td class="mono">${esc(s.ip)}</td><td>${ago(s.lastSeen)}</td><td style="text-align:right">${s.current ? '' : `<button class="btn small" data-rv="${esc(s.id)}">Sign out</button>`}</td></tr>`).join('')}</table>
      <button class="btn" id="ra" style="margin-top:10px">Sign out all other sessions</button></div>`;
  $('#pf').onsubmit = async (e) => {
    e.preventDefault();
    if ($('#n1').value !== $('#n2').value) return toast("The new passwords don't match");
    try { await api('me/password', { current: $('#cur').value, new: $('#n1').value }); toast('Password changed'); renderView(); } catch (er) { toast(er.message); }
  };
  const e2 = $('#e2'); if (e2) e2.onclick = () => render2FASetup(false);
  const rg = $('#rg'); if (rg) rg.onclick = async () => { try { const r = await api('me/2fa/recovery', { password: $('#rp').value }); renderRecovery(r.recovery); } catch (er) { toast(er.message); } };
  const d2 = $('#d2'); if (d2) d2.onclick = async () => { if (!confirm('Turn off 2FA? Your account will be protected by the password only.')) return; try { await api('me/2fa/disable', { password: $('#dp').value, code: $('#dc').value }); S.me = await api('me'); renderMain(); S.view = 'account'; renderView(); } catch (er) { toast(er.message); } };
  $$('[data-rv]').forEach((b) => b.onclick = async () => { await api('me/sessions', { id: b.dataset.rv }); renderView(); });
  $('#ra').onclick = async () => { await api('me/sessions', { all: true }); toast('Other sessions signed out'); renderView(); };
}

function shortUA(ua) {
  if (!ua) return 'Unknown';
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Windows/.test(ua) ? 'Windows' : /Mac OS/.test(ua) ? 'macOS' : /Linux/.test(ua) ? 'Linux' : '';
  const br = /Electron/.test(ua) ? 'KadduDesk app' : /; wv\)/.test(ua) ? 'KadduDesk app' : /Edg\//.test(ua) ? 'Edge' : /Chrome\//.test(ua) ? 'Chrome' : /Firefox\//.test(ua) ? 'Firefox' : /Safari\//.test(ua) ? 'Safari' : 'Browser';
  return br + (os ? ' on ' + os : '');
}

// ------------------------------------------------------------------ administration
async function viewAdmin(c) {
  c.innerHTML = `<div class="nav" id="at"><button data-t="users">Users</button><button data-t="security">Security policy</button><button data-t="audit">Audit log</button></div><div id="ac"></div>`;
  $$('#at button').forEach((b) => { b.classList.toggle('on', b.dataset.t === S.adminTab); b.onclick = () => { S.adminTab = b.dataset.t; viewAdmin(c); }; });
  const ac = $('#ac');
  if (S.adminTab === 'users') return adminUsers(ac);
  if (S.adminTab === 'security') return adminSecurity(ac);
  return adminAudit(ac);
}

function genPassword() {
  const a = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%&*?';
  const b = new Uint32Array(16); crypto.getRandomValues(b);
  return Array.from(b, (x) => a[x % a.length]).join('');
}

async function adminUsers(c) {
  const { users } = await api('admin/users');
  const me = S.me.user;
  c.innerHTML = `<div class="panel"><h2>Add a person</h2>
    <form class="inline" id="cu"><input id="cun" placeholder="Username" autocapitalize="off" required>
      <input id="cup" placeholder="Temporary password" required><button class="btn" type="button" id="gen">Generate</button>
      <select id="cur" style="max-width:150px"><option value="member">Member</option>${me.role === 'owner' ? '<option value="admin">Admin</option>' : ''}</select>
      <button class="btn primary">Create</button></form>
    <p class="tiny muted">They must choose their own password${S.me.security.require2fa ? ' and set up 2FA' : ''} the first time they sign in. Give them access by adding them to a workspace.</p></div>
    <div class="panel"><h2>People</h2><table><tr><th>User</th><th>Role</th><th>Security</th><th>Last sign-in</th><th></th></tr>
    ${users.map((u) => `<tr><td><b>${esc(u.username)}</b>${u.id === me.id ? ' <span class="badge">you</span>' : ''}</td>
      <td>${esc(u.role)}</td>
      <td>${u.totp ? '<span class="badge ok">2FA</span>' : '<span class="badge warn">no 2FA</span>'}${u.disabled ? ' <span class="badge bad">disabled</span>' : ''}${u.locked ? ' <span class="badge bad">locked</span>' : ''}${u.mustChange ? ' <span class="badge">temp password</span>' : ''}</td>
      <td>${ago(u.lastLogin)}</td>
      <td style="text-align:right">${u.role === 'owner' || u.id === me.id ? '' : `<select data-act="${esc(u.id)}" style="width:auto"><option value="">Actions…</option>
        ${me.role === 'owner' ? `<option value="role">${u.role === 'admin' ? 'Make member' : 'Make admin'}</option>` : ''}
        <option value="pw">Reset password</option>${u.totp ? '<option value="2fa">Reset 2FA</option>' : ''}${u.locked ? '<option value="unlock">Unlock</option>' : ''}
        <option value="dis">${u.disabled ? 'Enable' : 'Disable'}</option><option value="del">Delete</option></select>`}</td></tr>`).join('')}</table></div>`;
  $('#gen').onclick = () => { $('#cup').value = genPassword(); };
  $('#cu').onsubmit = async (e) => {
    e.preventDefault();
    try { await api('admin/users', { username: $('#cun').value, password: $('#cup').value, role: $('#cur').value }); toast('Created. Share the temporary password privately.', 5000); adminUsers(c); }
    catch (er) { toast(er.message); }
  };
  $$('[data-act]').forEach((s) => s.onchange = async () => {
    const id = s.dataset.act, u = users.find((x) => x.id === id), a = s.value; s.value = '';
    try {
      if (a === 'role') await api('admin/users/update', { id, role: u.role === 'admin' ? 'member' : 'admin' });
      if (a === 'pw') { const p = genPassword(); if (!confirm(`Reset ${u.username}'s password to:\n\n${p}\n\nThey must change it at next sign-in.`)) return; await api('admin/users/update', { id, password: p }); }
      if (a === '2fa' && confirm(`Reset 2FA for ${u.username}? They'll set it up again at next sign-in.`)) await api('admin/users/update', { id, reset2fa: true });
      if (a === 'unlock') await api('admin/users/update', { id, unlock: true });
      if (a === 'dis') await api('admin/users/update', { id, disabled: !u.disabled });
      if (a === 'del' && confirm(`Delete ${u.username}? This can't be undone.`)) await api('admin/users/update', { id, delete: true });
      adminUsers(c);
    } catch (er) { toast(er.message); }
  });
}

async function adminSecurity(c) {
  const s = await api('admin/security');
  const owner = S.me.user.role === 'owner';
  c.innerHTML = `<div class="panel"><h2>Security policy</h2>
    <label class="chk"><input type="checkbox" id="r2" ${s.require2fa ? 'checked' : ''} ${owner ? '' : 'disabled'}> Require two-factor authentication for everyone (recommended)</label>
    <div class="inline" style="margin:8px 0"><span style="min-width:260px">Sign out after inactivity (minutes)</span><input id="idle" type="number" min="5" max="1440" value="${s.idleMinutes}" ${owner ? '' : 'disabled'}></div>
    <div class="inline" style="margin:8px 0"><span style="min-width:260px">"Keep me signed in" lasts (days)</span><input id="rd" type="number" min="1" max="90" value="${s.rememberDays}" ${owner ? '' : 'disabled'}></div>
    ${owner ? '<button class="btn primary" id="sv">Save</button>' : '<p class="tiny muted">Only the owner can change these.</p>'}
    <p class="tiny muted" style="margin-top:12px">Always on: Argon2id password hashing · lockout after 5 failed attempts (per account and per network) · HttpOnly, Secure, SameSite=Strict session cookies · CSRF protection · TLS with certificate pinning in the apps · full audit log.</p></div>`;
  const sv = $('#sv');
  if (sv) sv.onclick = async () => {
    try { await api('admin/security', { require2fa: $('#r2').checked, idleMinutes: +$('#idle').value, rememberDays: +$('#rd').value }); toast('Saved'); S.me = await api('me'); }
    catch (er) { toast(er.message); }
  };
}

async function adminAudit(c) {
  const { entries } = await api('admin/audit');
  const bad = (e) => /failed|blocked/.test(e.event);
  c.innerHTML = `<div class="panel"><h2>Audit log (newest first)</h2><table><tr><th>When</th><th>Event</th><th>User</th><th>From</th><th>Details</th></tr>
    ${entries.map((e) => `<tr><td class="tiny">${esc(new Date(e.t).toLocaleString())}</td><td>${bad(e) ? '<span class="badge bad">' + esc(e.event) + '</span>' : esc(e.event)}</td><td>${esc(e.user || '')}</td><td class="mono tiny">${esc(e.ip || '')}</td><td class="tiny">${esc(e.detail || '')}</td></tr>`).join('') || '<tr><td class="muted">Nothing yet.</td></tr>'}</table></div>`;
}

window.kdBack = () => { if (multi) { if (multi.focus) focusDevice(null); else closeMulti(); return true; } return false; };
start();
