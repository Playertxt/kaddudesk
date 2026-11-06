/* KadduDesk web client — WebCodecs H.264 over secure WebSocket, low-latency path. */
'use strict';
const $ = (s) => document.querySelector(s);
const $$ = (s) => Array.from(document.querySelectorAll(s));
const isTouch = matchMedia('(pointer: coarse)').matches || 'ontouchstart' in window;
if (isTouch) document.documentElement.classList.add('touch');

// ------------------------------------------------------------------ settings
const DEFAULTS = {
  preset: 'auto', fps: 60, bitrate: 20, maxHeight: isTouch ? 1080 : 0, fit: 'fit',
  mode: isTouch ? 'trackpad' : 'mouse', speed: 1.2, natural: true, relmouse: false, haptics: true,
  localCursor: true, audio: true, stats: false, clipAuto: true, monitor: 0,
};
const PRESETS = {
  auto: { fps: 60, bitrate: 20 }, speed: { fps: 60, bitrate: 8, maxHeight: 720 },
  balanced: { fps: 60, bitrate: 20 }, quality: { fps: 60, bitrate: 45, maxHeight: 0 },
};
let S = Object.assign({}, DEFAULTS);
try { Object.assign(S, JSON.parse(localStorage.kd_settings || '{}')); } catch (e) {}
const save = () => { if (embedMode === 'thumb') return; try { localStorage.kd_settings = JSON.stringify(S); } catch (e) {} };

// When opened through a hub the app lives under /d/<id>/ — all URLs are relative to BASE,
// and the saved login is kept per device.
const BASE = location.pathname.replace(/[^/]*$/, '');
const VIA_HUB = BASE !== '/';
const TOK = VIA_HUB ? 'kd_token:' + BASE : 'kd_token';
let token = localStorage[TOK] || sessionStorage[TOK] || '';
// Embedded in the hub's multi-device view: 'thumb' (small, cheap, silent) or 'main' (full control)
const EMBED = new URLSearchParams(location.search).get('embed');
let embedMode = EMBED;
if (EMBED) document.documentElement.classList.add('embed', EMBED === 'thumb' ? 'thumb' : 'main');

// ------------------------------------------------------------------ helpers
let toastT;
function toast(msg, ms = 2600) {
  const t = $('#toast');
  t.textContent = msg; t.hidden = false;
  clearTimeout(toastT); toastT = setTimeout(() => (t.hidden = true), ms);
}
function confirmBox(msg) {
  return new Promise((res) => {
    const d = $('#confirmDlg');
    $('#confirmMsg').textContent = msg;
    $('#cYes').onclick = () => { d.close(); res(true); };
    $('#cNo').onclick = () => { d.close(); res(false); };
    d.showModal();
  });
}
const fmtBytes = (n) => n < 1024 ? n + ' B' : n < 1048576 ? (n / 1024).toFixed(1) + ' KB' : n < 1073741824 ? (n / 1048576).toFixed(1) + ' MB' : (n / 1073741824).toFixed(2) + ' GB';
const authHdr = () => ({ Authorization: 'Bearer ' + token });

// ------------------------------------------------------------------ boot
async function boot() {
  if (!window.VideoDecoder || !window.isSecureContext) { $('#unsupported').hidden = false; return; }
  if (KD.kind !== 'browser' || VIA_HUB) $('#loginHome').hidden = false;
  if (VIA_HUB) $('#loginHome').textContent = '← Back to all devices';
  $('#loginHome').onclick = () => goHome();
  try {
    const info = await (await fetch(BASE + 'api/info', { cache: 'no-store' })).json();
    $('#loginTitle').textContent = info.name;
    $('#pcName').textContent = info.name;
    $('#fpShow').textContent = info.fp.match(/.{4}/g).slice(0, 4).join(' ');
    document.title = info.name + ' · KadduDesk';
  } catch (e) {}
  const r = await fetch(BASE + 'api/check', { headers: token ? authHdr() : {} }).catch(() => null);
  if (r && r.ok) return startSession();
  if (token) { token = ''; delete localStorage[TOK]; }
  showLogin();
}

async function showLogin(err) {
  $('#session').hidden = true;
  $('#login').hidden = false;
  $('#loginErr').textContent = err || '';
  if (!$('#devname').value) $('#devname').value = localStorage.kd_devname || (KD.deviceName ? await KD.deviceName() : '') || '';
  setTimeout(() => $('#pin').focus(), 50);
}

$('#loginForm').addEventListener('submit', async (e) => {
  e.preventDefault();
  const btn = $('#loginBtn');
  btn.disabled = true; btn.textContent = 'Waiting for PC…';
  $('#loginErr').textContent = '';
  try {
    const remember = $('#remember').checked;
    localStorage.kd_devname = $('#devname').value;
    const r = await fetch(BASE + 'api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ pin: $('#pin').value, remember, device: $('#devname').value }) });
    const j = await r.json();
    if (!r.ok) throw new Error(j.error || 'Login failed');
    token = j.token;
    if (remember) localStorage[TOK] = token; else sessionStorage[TOK] = token;
    $('#pin').value = '';
    startSession();
  } catch (err) {
    $('#loginErr').textContent = err.message;
  } finally {
    btn.disabled = false; btn.textContent = 'Connect';
  }
});

// ------------------------------------------------------------------ session / websocket
let ws = null, wantOpen = false, retry = 0, hello = null, serverCfg = null;
const canvas = $('#screen');
const ctx2d = canvas.getContext('2d', { alpha: false, desynchronized: true });
let vidW = 0, vidH = 0;

function startSession() {
  $('#login').hidden = true;
  $('#session').hidden = false;
  wantOpen = true;
  if (KD.keepAwake) KD.keepAwake(true);
  initUI();
  connect();
}

function status(msg) {
  const s = $('#statusMsg');
  if (!msg) { s.hidden = true; return; }
  s.hidden = false; s.querySelector('span').textContent = msg;
}

function connect() {
  status(retry ? 'Reconnecting…' : 'Connecting…');
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(`${proto}//${location.host}${BASE}ws?token=${encodeURIComponent(token)}`);
  ws.binaryType = 'arraybuffer';
  ws.onopen = () => { retry = 0; };
  ws.onmessage = (ev) => {
    if (typeof ev.data === 'string') onCtl(JSON.parse(ev.data));
    else onBinary(ev.data);
  };
  ws.onclose = async () => {
    ws = null;
    resetDecoder();
    if (!wantOpen) return;
    const r = await fetch(BASE + 'api/check', { headers: authHdr() }).catch(() => null);
    if (r && r.status === 401) { wantOpen = false; token = ''; delete localStorage[TOK]; return showLogin('Session expired — enter the PIN again.'); }
    retry++;
    status('Connection lost — reconnecting…');
    setTimeout(() => wantOpen && connect(), Math.min(5000, 300 * retry));
  };
}

function send(o) { if (ws && ws.readyState === 1) ws.send(JSON.stringify(o)); }

function sendCfg() {
  if (embedMode === 'thumb') { send({ t: 'cfg', cfg: { monitor: S.monitor | 0, fps: 10, bitrate: 800, maxHeight: 360 } }); return; }
  const p = PRESETS[S.preset] || {};
  const cfg = {
    monitor: S.monitor | 0,
    fps: S.preset === 'custom' ? S.fps : p.fps,
    bitrate: Math.round((S.preset === 'custom' ? S.bitrate : S.preset === 'auto' ? auto.br : p.bitrate) * 1000),
    maxHeight: S.preset === 'custom' || S.preset === 'auto' || S.preset === 'balanced' ? S.maxHeight | 0 : (p.maxHeight ?? S.maxHeight) | 0,
  };
  send({ t: 'cfg', cfg });
}

function onCtl(m) {
  switch (m.t) {
    case 'hello':
      hello = m;
      $('#pcName').textContent = m.name;
      $('#pcInfo').textContent = `v${m.version} · ${m.service ? 'service mode' : m.admin ? 'admin mode' : 'standard mode'}`;
      onDesk(m.desk, m.service);
      buildMonitors(m.monitors);
      setupRemoteOS(m.os);
      saveHostMeta(m);
      if (!m.features.control) toast('View-only: the PC owner disabled remote control');
      sendCfg();
      if (S.audio && m.features.audio && embedMode !== 'thumb') startAudio();
      if (S.clipAuto) pushLocalClipboard();
      break;
    case 'cfg': serverCfg = m.cfg; break;
    case 'cur': curShape = m.s; if (S.mode === 'mouse' && !S.relmouse) canvas.style.cursor = m.s === 'none' ? 'none' : m.s; break;
    case 'cp': onRemoteCursor(m.x, m.y); break;
    case 'clip': onRemoteClip(m.s); break;
    case 'pong': rtt = performance.now() - m.ts; rttHist.push(rtt); if (rttHist.length > 20) rttHist.shift(); break;
    case 'st': srvStats = m; updateStats(); break;
    case 'congestion': auto.congestion(); break;
    case 'notice': toast(m.s, 4000); break;
    case 'desk': onDesk(m.s, m.service); break;
  }
}

// Controlling an Android phone: add its system buttons and use direct touch on phones.
function setupRemoteOS(os) {
  if (os !== 'Android' || $('#qaBack')) return;
  if (isTouch && S.mode === 'trackpad') S.mode = 'touch'; // not saved: only for this session
  const q = $('.quick');
  for (const [id, label, ic, a] of [['qaBack', 'Back', '◀', 'back'], ['qaHome', 'Home', '●', 'home'], ['qaRecents', 'Recents', '■', 'recents'], ['qaNotif', 'Notifications', '🔔', 'notifications']]) {
    const b = document.createElement('button');
    b.className = 'qa'; b.id = id; b.innerHTML = `<i>${ic}</i><span>${label}</span>`;
    b.onclick = () => { send({ t: 'power', a }); vib(10); };
    q.appendChild(b);
  }
  const cad = $('#qaCad'); if (cad) cad.hidden = true;
}

// lock screen / UAC prompt: visible only in Service mode
function onDesk(name, service) {
  const secure = name && name !== 'Default';
  if (secure && !service) status('The PC is showing a lock screen or UAC prompt. Turn on Service mode in the KadduDesk control panel on the PC to see and control it.');
  else if (firstFrame) status(null);
  if (secure && service) toast(name === 'Winlogon' ? 'Lock / sign-in screen' : 'Secure desktop');
}

// ------------------------------------------------------------------ video decode
let decoder = null, codecStr = '', needKey = true, lastSeq = 0;
const recvTimes = new Map();
let frames = 0, bytes = 0, decodeMsAcc = 0, decodeN = 0, dropped = 0;
let srvStats = null, rtt = 0; const rttHist = [];

function spsCodec(u8) {
  // find SPS NAL (type 7) in Annex-B and build avc1.PPCCLL
  for (let i = 0; i + 4 < u8.length; i++) {
    if (u8[i] === 0 && u8[i + 1] === 0 && u8[i + 2] === 1) {
      const t = u8[i + 3] & 0x1f;
      if (t === 7) {
        const h = (b) => b.toString(16).padStart(2, '0');
        return 'avc1.' + h(u8[i + 4]) + h(u8[i + 5]) + h(u8[i + 6]);
      }
    }
  }
  return null;
}

function resetDecoder() {
  if (decoder) { try { decoder.close(); } catch (e) {} }
  decoder = null; codecStr = ''; needKey = true; recvTimes.clear();
}

function makeDecoder(codec) {
  const d = new VideoDecoder({
    output: (frame) => {
      const rt = recvTimes.get(frame.timestamp);
      if (rt !== undefined) { decodeMsAcc += performance.now() - rt; decodeN++; recvTimes.delete(frame.timestamp); }
      if (frame.displayWidth !== vidW || frame.displayHeight !== vidH) {
        vidW = canvas.width = frame.displayWidth; vidH = canvas.height = frame.displayHeight; layout();
      }
      ctx2d.drawImage(frame, 0, 0);
      frame.close();
      frames++;
      if (!firstFrame) { firstFrame = true; status(null); }
    },
    error: (e) => { console.warn('decoder error', e); resetDecoder(); send({ t: 'kf' }); },
  });
  d.configure({ codec, optimizeForLatency: true, hardwareAcceleration: 'prefer-hardware' });
  return d;
}
let firstFrame = false;

function onBinary(buf) {
  const dv = new DataView(buf);
  const type = dv.getUint8(0);
  if (type === 1) {
    const key = dv.getUint8(1) & 1;
    const ts = Number(dv.getBigUint64(2));
    const seq = dv.getUint32(10);
    const data = new Uint8Array(buf, 14);
    bytes += buf.byteLength;
    auto.sample(ts);
    if (lastSeq && seq !== lastSeq + 1 && !key) needKey = true; // gap (server dropped for us)
    lastSeq = seq;
    if (key) {
      const c = spsCodec(data);
      if (c && (c !== codecStr || !decoder || decoder.state === 'closed')) {
        resetDecoder(); codecStr = c; decoder = makeDecoder(c);
      }
      needKey = false;
    }
    if (needKey || !decoder || decoder.state !== 'configured') { dropped++; return; }
    if (decoder.decodeQueueSize > 4) { // decoder can't keep up: resync instead of lagging
      dropped++; needKey = true; send({ t: 'kf' }); return;
    }
    recvTimes.set(ts, performance.now());
    if (recvTimes.size > 120) recvTimes.clear();
    try {
      decoder.decode(new EncodedVideoChunk({ type: key ? 'key' : 'delta', timestamp: ts, data }));
    } catch (e) { resetDecoder(); send({ t: 'kf' }); }
  } else if (type === 2) {
    if (audioOut) audioOut.push(new Uint8Array(buf, 9), Number(dv.getBigUint64(1)));
  }
}

// ------------------------------------------------------------------ adaptive bitrate (Auto preset)
const auto = {
  br: 20, minOff: Infinity, samples: [], lastChange: 0, goodSince: 0, offT: 0,
  sample(ts) {
    const off = performance.timeOrigin + performance.now() - ts / 1000; // one-way delay + clock offset
    const now = performance.now();
    if (off < this.minOff || now - this.offT > 15000) { this.minOff = off; this.offT = now; }
    this.samples.push(off - this.minOff);
    if (this.samples.length > 60) this.samples.shift();
  },
  queueDelay() { const a = [...this.samples].sort((x, y) => x - y); return a.length ? a[Math.floor(a.length * 0.5)] : 0; },
  congestion() { if (S.preset === 'auto') this.adjust(0.6, true); },
  tick() {
    if (S.preset !== 'auto' || !ws || embedMode === 'thumb') return;
    const q = this.queueDelay(), now = performance.now();
    if (q > 45) { this.goodSince = 0; this.adjust(0.7); }
    else if (q < 12) { if (!this.goodSince) this.goodSince = now; if (now - this.goodSince > 8000) { this.goodSince = now; this.adjust(1.2); } }
    else this.goodSince = 0;
  },
  adjust(f, force) {
    const now = performance.now();
    if (!force && now - this.lastChange < 4000) return;
    const nb = Math.max(1.5, Math.min(40, Math.round(this.br * f * 10) / 10));
    if (Math.abs(nb - this.br) < 0.5) return;
    this.br = nb; this.lastChange = now; this.samples = []; this.minOff = Infinity;
    sendCfg();
  },
};

// ------------------------------------------------------------------ audio
let audioOut = null;
class AudioOut {
  constructor() {
    this.ctx = new (window.AudioContext || window.webkitAudioContext)({ latencyHint: 'interactive', sampleRate: 48000 });
    this.next = 0;
    this.dec = new AudioDecoder({ output: (a) => this.play(a), error: (e) => console.warn('audio', e) });
    this.dec.configure({ codec: 'opus', sampleRate: 48000, numberOfChannels: 2 });
  }
  push(data, ts) {
    if (this.dec.state !== 'configured') return;
    if (this.ctx.state === 'suspended') this.ctx.resume();
    try { this.dec.decode(new EncodedAudioChunk({ type: 'key', timestamp: ts, data })); } catch (e) {}
  }
  play(a) {
    const n = a.numberOfFrames, ch = a.numberOfChannels;
    const ab = this.ctx.createBuffer(ch, n, a.sampleRate);
    if (a.format && a.format.endsWith('-planar')) {
      for (let c = 0; c < ch; c++) { const t = new Float32Array(n); a.copyTo(t, { planeIndex: c, format: 'f32-planar' }); ab.copyToChannel(t, c); }
    } else {
      const t = new Float32Array(n * ch); a.copyTo(t, { planeIndex: 0, format: 'f32' });
      for (let c = 0; c < ch; c++) { const p = new Float32Array(n); for (let i = 0; i < n; i++) p[i] = t[i * ch + c]; ab.copyToChannel(p, c); }
    }
    a.close();
    const src = this.ctx.createBufferSource();
    src.buffer = ab; src.connect(this.ctx.destination);
    const now = this.ctx.currentTime;
    if (this.next < now + 0.005 || this.next > now + 0.2) this.next = now + 0.03; // re-anchor on gaps/drift
    src.start(this.next);
    this.next += ab.duration;
  }
  close() { try { this.dec.close(); } catch (e) {} try { this.ctx.close(); } catch (e) {} }
}
function startAudio() {
  if (!window.AudioDecoder) { toast('Sound not supported on this device'); return; }
  if (!audioOut) { try { audioOut = new AudioOut(); } catch (e) { toast('Audio failed: ' + e.message); return; } }
  send({ t: 'audio', on: true });
}
function stopAudio() { send({ t: 'audio', on: false }); if (audioOut) { audioOut.close(); audioOut = null; } }

// ------------------------------------------------------------------ layout / zoom
const stage = $('#stage');
let zoom = 1, panX = 0, panY = 0, dispW = 0, dispH = 0;
function layout() {
  const vv = window.visualViewport;
  const vw = stage.clientWidth, vh = vv ? Math.min(stage.clientHeight, vv.height) - ($('#keybar').hidden ? 0 : $('#keybar').offsetHeight) : stage.clientHeight;
  const w = vidW || 1920, h = vidH || 1080;
  let base = Math.min(vw / w, vh / h);
  if (S.fit === 'fill') base = Math.max(vw / w, vh / h);
  if (S.fit === '1:1') base = 1 / (window.devicePixelRatio || 1);
  const s = base * zoom;
  dispW = w * s; dispH = h * s;
  if (dispW <= vw) panX = (vw - dispW) / 2; else panX = Math.min(0, Math.max(vw - dispW, panX));
  if (dispH <= vh) panY = (vh - dispH) / 2; else panY = Math.min(0, Math.max(vh - dispH, panY));
  canvas.style.width = dispW + 'px';
  canvas.style.height = dispH + 'px';
  canvas.style.transform = `translate3d(${panX}px, ${panY}px, 0)`;
  stage.classList.toggle('pixel', s >= 1.5);
  placeCursor();
}
window.addEventListener('resize', layout);
if (window.visualViewport) visualViewport.addEventListener('resize', () => { layout(); followCursor(); });

function zoomAt(cx, cy, factor) {
  const nz = Math.max(1, Math.min(6, zoom * factor));
  const rx = (cx - panX) / dispW, ry = (cy - panY) / dispH;
  zoom = nz;
  const w = vidW || 1920, h = vidH || 1080;
  const vw = stage.clientWidth, vh = stage.clientHeight;
  let base = Math.min(vw / w, vh / h);
  if (S.fit === 'fill') base = Math.max(vw / w, vh / h);
  if (S.fit === '1:1') base = 1 / (window.devicePixelRatio || 1);
  panX = cx - rx * w * base * zoom; panY = cy - ry * h * base * zoom;
  layout();
}

function toRemote(clientX, clientY) {
  const r = canvas.getBoundingClientRect();
  return [(clientX - r.left) / r.width, (clientY - r.top) / r.height];
}

// ------------------------------------------------------------------ cursor
let curX = 0.5, curY = 0.5, curShape = 'default', touching = false;
const vcur = $('#vcursor');
function placeCursor() {
  const show = S.localCursor && S.mode !== 'mouse' && curShape !== 'none';
  vcur.hidden = !show;
  if (!show) return;
  const r = canvas.getBoundingClientRect(), sr = stage.getBoundingClientRect();
  vcur.style.transform = `translate3d(${r.left - sr.left + curX * r.width - 3}px, ${r.top - sr.top + curY * r.height - 2}px, 0)`;
}
function onRemoteCursor(x, y) {
  if (S.mode === 'mouse' || touching) return; // local cursor is authoritative while we move it
  if (performance.now() - lastLocalMove < 300) return;
  curX = x; curY = y; placeCursor();
}
let lastLocalMove = 0;
function moveCursorTo(x, y) {
  curX = Math.max(0, Math.min(1, x)); curY = Math.max(0, Math.min(1, y));
  lastLocalMove = performance.now();
  send({ t: 'mm', x: curX, y: curY });
  placeCursor();
}
function followCursor() {
  if (zoom <= 1 || S.mode !== 'trackpad') return;
  const vw = stage.clientWidth, vh = window.visualViewport ? visualViewport.height : stage.clientHeight;
  const px = panX + curX * dispW, py = panY + curY * dispH, m = 60;
  if (px < m) panX += m - px; else if (px > vw - m) panX -= px - (vw - m);
  if (py < m) panY += m - py; else if (py > vh - m) panY -= py - (vh - m);
  layout();
}

// ------------------------------------------------------------------ mouse input (desktop)
const btnMap = { 0: 0, 1: 1, 2: 2, 3: 3, 4: 4 };
let wheelAccX = 0, wheelAccY = 0;
canvas.addEventListener('pointermove', (e) => {
  if (e.pointerType === 'touch') return;
  if (document.pointerLockElement === canvas) { send({ t: 'mr', dx: e.movementX, dy: e.movementY }); return; }
  const [x, y] = toRemote(e.clientX, e.clientY);
  curX = x; curY = y;
  send({ t: 'mm', x, y });
});
canvas.addEventListener('pointerdown', (e) => {
  if (e.pointerType === 'touch') return;
  e.preventDefault();
  canvas.focus();
  if (S.relmouse && document.pointerLockElement !== canvas) { canvas.requestPointerLock && canvas.requestPointerLock(); return; }
  canvas.setPointerCapture(e.pointerId);
  if (document.pointerLockElement !== canvas) { const [x, y] = toRemote(e.clientX, e.clientY); send({ t: 'mm', x, y }); }
  send({ t: 'mb', b: btnMap[e.button] ?? 0, d: true });
});
canvas.addEventListener('pointerup', (e) => {
  if (e.pointerType === 'touch') return;
  send({ t: 'mb', b: btnMap[e.button] ?? 0, d: false });
});
canvas.addEventListener('contextmenu', (e) => e.preventDefault());
canvas.addEventListener('wheel', (e) => {
  e.preventDefault();
  const k = e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? 800 : 1.2;
  wheelAccX += e.deltaX * k; wheelAccY += e.deltaY * k;
  const dx = Math.trunc(wheelAccX), dy = Math.trunc(wheelAccY);
  if (dx || dy) { wheelAccX -= dx; wheelAccY -= dy; send({ t: 'wh', dx, dy }); }
}, { passive: false });
canvas.addEventListener('auxclick', (e) => e.preventDefault());
canvas.tabIndex = 0;

// ------------------------------------------------------------------ keyboard (physical)
const down = new Set();
function isTypingTarget(t) { return t && t !== $('#ime') && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT'); }
window.addEventListener('keydown', (e) => {
  if ($('#session').hidden || isTypingTarget(e.target)) return;
  if (e.ctrlKey && e.altKey && e.shiftKey && e.code === 'KeyM') { e.preventDefault(); toggleDrawer(); return; }
  if (!$('#drawer').hidden && e.code === 'Escape') { e.preventDefault(); toggleDrawer(false); return; }
  if (e.target === $('#ime') && (e.key === 'Unidentified' || e.keyCode === 229)) return; // soft keyboard → handled via input events
  if (!e.code) return;
  e.preventDefault();
  if ((e.ctrlKey || e.metaKey) && e.code === 'KeyV' && S.clipAuto) pushLocalClipboard();
  down.add(e.code);
  send({ t: 'k', c: e.code, d: true });
}, true);
window.addEventListener('keyup', (e) => {
  if ($('#session').hidden || isTypingTarget(e.target) || !e.code) return;
  if (e.target === $('#ime') && (e.key === 'Unidentified' || e.keyCode === 229)) return;
  e.preventDefault();
  down.delete(e.code);
  send({ t: 'k', c: e.code, d: false });
}, true);
window.addEventListener('blur', () => { down.clear(); send({ t: 'release' }); });
window.addEventListener('focus', () => { if (S.clipAuto) pushLocalClipboard(); });

function tapKey(code) {
  const mods = [...stickyMods];
  mods.forEach((m) => send({ t: 'k', c: m, d: true }));
  send({ t: 'k', c: code, d: true }); send({ t: 'k', c: code, d: false });
  mods.reverse().forEach((m) => send({ t: 'k', c: m, d: false }));
  clearSticky();
}

// ------------------------------------------------------------------ soft keyboard (mobile)
const ime = $('#ime');
const SENT = '​​'; // sentinel so Backspace always has something to delete
let imeLast = SENT;
function resetIme() { ime.value = SENT; imeLast = SENT; ime.setSelectionRange(SENT.length, SENT.length); }
function showKeyboard(on) {
  $('#keybar').hidden = !on;
  $('#qaKeyboard').classList.toggle('on', on);
  if (on) { resetIme(); ime.focus({ preventScroll: true }); if (KD.showKeyboard) KD.showKeyboard(true); }
  else { ime.blur(); clearSticky(); if (KD.showKeyboard) KD.showKeyboard(false); }
  setTimeout(layout, 300);
}
const charCode = (ch) => {
  if (/^[a-z]$/i.test(ch)) return 'Key' + ch.toUpperCase();
  if (/^[0-9]$/.test(ch)) return 'Digit' + ch;
  return { ' ': 'Space', '-': 'Minus', '=': 'Equal', '[': 'BracketLeft', ']': 'BracketRight', ';': 'Semicolon',
    "'": 'Quote', ',': 'Comma', '.': 'Period', '/': 'Slash', '\\': 'Backslash', '`': 'Backquote' }[ch];
};
function sendTyped(text) {
  if (!text) return;
  if (stickyMods.size && [...stickyMods].some((m) => m !== 'ShiftLeft')) { // Ctrl+C etc. from soft keyboard
    for (const ch of text) { const c = charCode(ch.toLowerCase()); if (c) tapKey(c); }
    return;
  }
  clearSticky();
  send({ t: 'txt', s: text });
}
ime.addEventListener('input', () => {
  const v = ime.value;
  let i = 0;
  while (i < v.length && i < imeLast.length && v[i] === imeLast[i]) i++;
  const removed = imeLast.length - i, added = v.slice(i);
  for (let k = 0; k < removed; k++) tapKey('Backspace');
  if (added) sendTyped(added.replace(/​/g, ''));
  imeLast = v;
  if (!v.startsWith(SENT) || v.length > 40) setTimeout(resetIme, 0);
});
ime.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') { e.preventDefault(); tapKey('Enter'); }
});
ime.addEventListener('compositionend', () => setTimeout(() => { if (ime.value.length > 20) resetIme(); }, 0));

const stickyMods = new Set();
function clearSticky() { stickyMods.clear(); $$('#keybar [data-mod]').forEach((b) => b.classList.remove('on')); }
$$('#keybar button').forEach((b) => {
  b.addEventListener('pointerdown', (e) => e.preventDefault()); // keep IME focused
  b.addEventListener('click', () => {
    if (S.haptics && KD.vibrate) KD.vibrate(8);
    if (b.dataset.mod) {
      const m = b.dataset.mod;
      if (stickyMods.has(m)) stickyMods.delete(m); else stickyMods.add(m);
      b.classList.toggle('on', stickyMods.has(m));
      if (m === 'MetaLeft' && stickyMods.size === 1) { /* allow Win alone on double tap */ }
    } else if (b.dataset.key) tapKey(b.dataset.key);
  });
});
$('#kbHide').onclick = () => showKeyboard(false);
$('#keybar [data-mod="MetaLeft"]').addEventListener('dblclick', () => { clearSticky(); tapKey('MetaLeft'); });

// ------------------------------------------------------------------ touch gestures
const T = new Map();
let g = null, lastTap = { t: 0, x: 0, y: 0 }, longT = null;
const vib = (ms) => S.haptics && KD.vibrate && KD.vibrate(ms);
function click(b = 0) { send({ t: 'mb', b, d: true }); send({ t: 'mb', b, d: false }); vib(10); }
function centroid() { let x = 0, y = 0; for (const p of T.values()) { x += p.x; y += p.y; } return [x / T.size, y / T.size]; }
function spread() { const a = [...T.values()]; return a.length < 2 ? 0 : Math.hypot(a[0].x - a[1].x, a[0].y - a[1].y); }

stage.addEventListener('touchstart', (e) => {
  if (e.target.closest('#bubble')) return;
  e.preventDefault();
  touching = true;
  const now = performance.now();
  for (const t of e.changedTouches) T.set(t.identifier, { x: t.clientX, y: t.clientY, sx: t.clientX, sy: t.clientY });
  if (!g) g = { start: now, moved: false, max: 0, drag: false, two: null, dragPending: false };
  g.max = Math.max(g.max, T.size);
  clearTimeout(longT);
  if (T.size === 1) {
    const p = [...T.values()][0];
    if (S.mode === 'trackpad' && now - lastTap.t < 300 && Math.hypot(p.x - lastTap.x, p.y - lastTap.y) < 50) g.dragPending = true;
    longT = setTimeout(() => {
      if (!g || g.moved || T.size !== 1) return;
      if (S.mode === 'touch') { const [x, y] = toRemote(p.x, p.y); moveCursorTo(x, y); click(2); g.consumed = true; vib(25); }
      else { send({ t: 'mb', b: 0, d: true }); g.drag = true; vib(25); } // trackpad: long-press starts a drag
    }, 480);
  } else if (T.size === 2) {
    const [cx, cy] = centroid();
    g.two = { mode: null, d0: spread(), d: spread(), cx, cy, cx0: cx, cy0: cy, acc: 0 };
  }
}, { passive: false });

stage.addEventListener('touchmove', (e) => {
  if (!g) return;
  e.preventDefault();
  const prev = new Map([...T].map(([k, v]) => [k, { x: v.x, y: v.y }]));
  for (const t of e.changedTouches) { const p = T.get(t.identifier); if (p) { p.x = t.clientX; p.y = t.clientY; } }
  if (T.size === 1) {
    const [id, p] = [...T][0];
    const q = prev.get(id) || p;
    const dx = p.x - q.x, dy = p.y - q.y;
    if (!g.moved && Math.hypot(p.x - p.sx, p.y - p.sy) > 7) { g.moved = true; clearTimeout(longT); }
    if (!g.moved) return;
    if (S.mode === 'trackpad') {
      if (g.dragPending && !g.drag) { send({ t: 'mb', b: 0, d: true }); g.drag = true; }
      // pointer follows the finger over the visible (possibly zoomed) image, with acceleration
      const v = Math.hypot(dx, dy);
      const accel = S.speed * (0.6 + Math.min(1.8, v / 14)); // slow = precise, fast flick = far
      moveCursorTo(curX + (dx * accel) / dispW, curY + (dy * accel) / dispH);
      followCursor();
    } else {
      if (!g.drag) { const [sx, sy] = toRemote(p.sx, p.sy); moveCursorTo(sx, sy); send({ t: 'mb', b: 0, d: true }); g.drag = true; }
      const [x, y] = toRemote(p.x, p.y); moveCursorTo(x, y);
    }
  } else if (T.size >= 2 && g.two) {
    const tw = g.two;
    const [cx, cy] = centroid(), d = spread();
    if (!tw.mode) {
      if (Math.abs(d - tw.d0) > 28) tw.mode = 'pinch';
      else if (Math.hypot(cx - tw.cx0, cy - tw.cy0) > 10) tw.mode = 'scroll';
    }
    if (tw.mode === 'pinch') {
      zoomAt(cx, cy, d / (tw.d || d));
      panX += cx - tw.cx; panY += cy - tw.cy; layout();
    } else if (tw.mode === 'scroll') {
      const dir = S.natural ? -1 : 1;
      tw.acc += (cy - tw.cy) * 3.2 * dir;
      const sx = (cx - tw.cx) * 3.2 * dir;
      const n = Math.trunc(tw.acc);
      if (n || Math.abs(sx) >= 1) { tw.acc -= n; send({ t: 'wh', dx: Math.trunc(sx), dy: n }); }
    }
    tw.cx = cx; tw.cy = cy; tw.d = d;
    g.moved = true;
  }
}, { passive: false });

function touchEnd(e) {
  if (!g) return;
  e.preventDefault();
  for (const t of e.changedTouches) T.delete(t.identifier);
  if (T.size > 0) { if (T.size === 1 && g.two) g.two = null; return; }
  clearTimeout(longT);
  const now = performance.now(), dur = now - g.start;
  if (g.drag) send({ t: 'mb', b: 0, d: false });
  else if (!g.moved && !g.consumed && dur < 300) {
    if (g.max === 1) {
      const p = e.changedTouches[0];
      if (S.mode === 'touch') { const [x, y] = toRemote(p.clientX, p.clientY); moveCursorTo(x, y); }
      click(0);
      lastTap = { t: now, x: p.clientX, y: p.clientY };
    } else if (g.max === 2) click(2);
    else if (g.max >= 3) showKeyboard($('#keybar').hidden);
  }
  g = null; touching = false;
}
stage.addEventListener('touchend', touchEnd, { passive: false });
stage.addEventListener('touchcancel', touchEnd, { passive: false });

// ------------------------------------------------------------------ clipboard sync
let lastClip = null;
async function pushLocalClipboard() {
  if (!hello || !hello.features.clipboard || !KD.getClipboard) return;
  const s = await KD.getClipboard();
  if (s && s !== lastClip) { lastClip = s; send({ t: 'clip', s }); $('#clipText').value = s; }
}
function onRemoteClip(s) {
  lastClip = s;
  $('#clipText').value = s;
  if (S.clipAuto && KD.setClipboard) KD.setClipboard(s);
}

// ------------------------------------------------------------------ files
let curPath = '';
async function loadFiles(p = curPath) {
  const list = $('#fileList');
  list.innerHTML = '<div class="empty">Loading…</div>';
  try {
    const r = await fetch(BASE + 'api/files?path=' + encodeURIComponent(p), { headers: authHdr() });
    const j = await r.json();
    if (!r.ok) throw new Error(j.error);
    curPath = j.path;
    $('#uploadDir').textContent = 'Files you send are saved to ' + j.uploadDir;
    const crumbs = $('#crumbs'); crumbs.innerHTML = '';
    const mk = (label, path) => { const b = document.createElement('button'); b.textContent = label; b.onclick = () => loadFiles(path); crumbs.appendChild(b); };
    mk('🏠 This PC', '');
    if (j.path) { let acc = ''; j.path.split('\\').filter(Boolean).forEach((seg, i) => { acc += seg + '\\'; mk(seg, acc); }); }
    list.innerHTML = '';
    if (j.parent !== '' || j.path) {
      const up = document.createElement('div'); up.className = 'f'; up.innerHTML = '<span>⬆</span><span class="n">..</span>';
      up.onclick = () => loadFiles(j.parent); list.appendChild(up);
    }
    if (!j.entries.length) list.insertAdjacentHTML('beforeend', '<div class="empty">Empty folder</div>');
    for (const f of j.entries) {
      const el = document.createElement('div'); el.className = 'f';
      el.innerHTML = `<span>${f.dir ? '📁' : '📄'}</span><span class="n"></span><span class="s">${f.dir ? '' : fmtBytes(f.size)}</span>`;
      el.querySelector('.n').textContent = f.name;
      el.onclick = () => f.dir ? loadFiles(f.path) : download(f);
      list.appendChild(el);
    }
  } catch (e) { list.innerHTML = ''; list.insertAdjacentHTML('beforeend', '<div class="empty"></div>'); list.firstChild.textContent = e.message; }
}
function download(f) {
  const url = `${location.origin}${BASE}api/download?path=${encodeURIComponent(f.path)}&token=${encodeURIComponent(token)}`;
  if (KD.download) { KD.download(url, f.name); toast('Downloading ' + f.name + '…'); return; }
  const a = document.createElement('a'); a.href = url; a.download = f.name; document.body.appendChild(a); a.click(); a.remove();
}
function upload(files) {
  for (const file of files) {
    const row = document.createElement('div'); row.className = 'u';
    row.innerHTML = '<div class="name"></div><div class="bar"><i></i></div>';
    row.querySelector('.name').textContent = `${file.name} · ${fmtBytes(file.size)}`;
    $('#uploads').prepend(row);
    const xhr = new XMLHttpRequest();
    xhr.open('POST', BASE + 'api/upload?name=' + encodeURIComponent(file.name));
    xhr.setRequestHeader('Authorization', 'Bearer ' + token);
    xhr.upload.onprogress = (e) => { if (e.lengthComputable) row.querySelector('i').style.width = (100 * e.loaded / e.total) + '%'; };
    xhr.onload = () => {
      let j = {}; try { j = JSON.parse(xhr.responseText); } catch (e) {}
      if (xhr.status === 200) { row.querySelector('.name').textContent = '✅ ' + file.name + ' → ' + j.path; toast('Sent ' + file.name); }
      else row.querySelector('.name').textContent = '❌ ' + file.name + ': ' + (j.error || xhr.status);
    };
    xhr.onerror = () => { row.querySelector('.name').textContent = '❌ ' + file.name + ': network error'; };
    xhr.send(file);
  }
}
$('#uploadBtn').onclick = () => $('#uploadInput').click();
$('#uploadInput').onchange = (e) => { upload(e.target.files); e.target.value = ''; };
$('#filesRefresh').onclick = () => loadFiles();
let dragDepth = 0;
window.addEventListener('dragenter', (e) => { if ($('#session').hidden) return; e.preventDefault(); dragDepth++; $('#dropHint').hidden = false; });
window.addEventListener('dragover', (e) => e.preventDefault());
window.addEventListener('dragleave', () => { if (--dragDepth <= 0) { dragDepth = 0; $('#dropHint').hidden = true; } });
window.addEventListener('drop', (e) => { e.preventDefault(); dragDepth = 0; $('#dropHint').hidden = true; if (e.dataTransfer.files.length) upload(e.dataTransfer.files); });

// ------------------------------------------------------------------ host metadata (for Wake-on-LAN in launcher)
async function saveHostMeta(m) {
  if (!KD.getHosts || VIA_HUB) return; // via a hub the hub keeps the device list
  try {
    const list = await KD.getHosts();
    const port = location.port || '443';
    let h = list.find((x) => x.host === location.hostname && String(x.port) === port);
    if (!h) { h = { host: location.hostname, port: +port }; list.push(h); }
    h.name = m.name; h.macs = m.macs; h.fp = m.fp; h.last = Date.now();
    await KD.saveHosts(list);
    const mac = (m.macs || [])[0];
    $('#wolInfo').textContent = mac ? `Wake-on-LAN saved for this PC (${mac.mac}). Use "Wake" in the app's PC list when it's asleep.` : '';
  } catch (e) {}
}

// ------------------------------------------------------------------ stats
let statT = 0;
function updateStats() {
  const now = performance.now(), dt = (now - statT) / 1000 || 1; statT = now;
  const fps = frames / dt, mbps = (bytes * 8) / dt / 1e6, dec = decodeN ? decodeMsAcc / decodeN : 0;
  frames = 0; bytes = 0; decodeMsAcc = 0; decodeN = 0;
  const q = auto.queueDelay();
  const r = rttHist.length ? [...rttHist].sort((a, b) => a - b)[Math.floor(rttHist.length / 2)] : 0;
  const est = r / 2 + q + dec + (srvStats ? 1000 / Math.max(srvStats.fps, 1) / 2 : 8);
  const dot = $('#topTab .dot');
  dot.className = 'dot' + (est > 90 ? ' bad' : est > 45 ? ' warn' : '');
  if (!S.stats) { $('#stats').hidden = true; return; }
  const cls = (v, a, b) => (v < a ? 'good' : v < b ? 'warn' : 'bad');
  const st = $('#stats');
  st.hidden = false;
  st.innerHTML =
    `<span class="${cls(est, 45, 90)}">latency ≈ ${est.toFixed(0)} ms</span>\n` +
    `ping ${r.toFixed(0)} ms · net queue ${q.toFixed(0)} ms\n` +
    `decode ${dec.toFixed(1)} ms · ${fps.toFixed(0)} fps\n` +
    `${mbps.toFixed(1)} Mbps · ${vidW}×${vidH}\n` +
    (srvStats ? `${srvStats.enc}${srvStats.zc ? ' (zero-copy)' : ''} · srv ${srvStats.fps} fps\n` : '') +
    (S.preset === 'auto' ? `auto bitrate ${auto.br} Mbps\n` : '') +
    `dropped ${dropped} · ${codecStr}`;
}
setInterval(() => { send({ t: 'ping', ts: performance.now() }); auto.tick(); }, 1000);

// ------------------------------------------------------------------ UI wiring
let uiReady = false;
function initUI() {
  if (uiReady) { layout(); return; }
  uiReady = true;
  auto.br = Math.min(S.bitrate || 20, 30);
  // bubble (touch): draggable; tap opens drawer
  const bub = $('#bubble');
  const bpos = JSON.parse(localStorage.kd_bubble || 'null') || { x: 1, y: 0.25 };
  const placeBubble = () => { bub.style.left = (bpos.x * (innerWidth - 56) + 4) + 'px'; bub.style.top = (bpos.y * (innerHeight - 56) + 4) + 'px'; };
  placeBubble(); window.addEventListener('resize', placeBubble);
  let bd = null;
  bub.addEventListener('touchstart', (e) => { e.stopPropagation(); const t = e.touches[0]; bd = { x: t.clientX, y: t.clientY, moved: false, t: performance.now() }; }, { passive: true });
  bub.addEventListener('touchmove', (e) => {
    e.preventDefault(); e.stopPropagation();
    const t = e.touches[0];
    if (Math.hypot(t.clientX - bd.x, t.clientY - bd.y) > 8) bd.moved = true;
    if (bd.moved) { bpos.x = Math.max(0, Math.min(1, (t.clientX - 28) / (innerWidth - 56))); bpos.y = Math.max(0, Math.min(1, (t.clientY - 28) / (innerHeight - 56))); placeBubble(); }
  }, { passive: false });
  let lastBubTap = 0;
  bub.addEventListener('touchend', (e) => {
    e.preventDefault(); e.stopPropagation();
    if (bd && bd.moved) { bpos.x = bpos.x < 0.5 ? 0 : 1; placeBubble(); localStorage.kd_bubble = JSON.stringify(bpos); return; }
    const now = performance.now();
    if (now - lastBubTap < 320) { zoom = 1; layout(); toggleDrawer(false); lastBubTap = 0; return; }
    lastBubTap = now;
    setTimeout(() => { if (lastBubTap === now) toggleDrawer(); }, 330);
  });
  bub.addEventListener('click', (e) => { if (!isTouch) toggleDrawer(); });
  $('#topTab').onclick = () => toggleDrawer();
  document.addEventListener('mousemove', (e) => { $('#topTab').classList.toggle('peek', e.clientY < 6); });
  $('#scrim').onclick = () => toggleDrawer(false);
  $('#drawerClose').onclick = () => toggleDrawer(false);
  $$('.tabs button').forEach((b) => b.onclick = () => {
    $$('.tabs button').forEach((x) => x.classList.toggle('on', x === b));
    $$('.pane').forEach((p) => (p.hidden = p.dataset.pane !== b.dataset.tab));
    if (b.dataset.tab === 'files') loadFiles();
  });

  // quick actions
  $('#qaKeyboard').onclick = () => { toggleDrawer(false); showKeyboard($('#keybar').hidden); };
  $('#qaAudio').onclick = () => { S.audio = !S.audio; save(); S.audio ? startAudio() : stopAudio(); syncUI(); };
  $('#qaFull').onclick = toggleFullscreen;
  $('#qaStats').onclick = () => { S.stats = !S.stats; save(); syncUI(); updateStats(); };
  $('#qaCad').onclick = () => send({ t: 'combo', k: ['ControlLeft', 'AltLeft', 'Delete'] });
  $('#qaDisconnect').onclick = disconnect;

  // display
  $$('#presets button').forEach((b) => b.onclick = () => {
    S.preset = b.dataset.preset;
    const p = PRESETS[S.preset];
    if (p && p.maxHeight !== undefined) S.maxHeight = p.maxHeight;
    if (S.preset === 'auto') auto.br = 20;
    save(); syncUI(); sendCfg();
  });
  $('#fps').oninput = (e) => { S.fps = +e.target.value; syncUI(); };
  $('#br').oninput = (e) => { S.bitrate = +e.target.value; syncUI(); };
  $('#fps').onchange = $('#br').onchange = () => { save(); sendCfg(); };
  $('#res').onchange = (e) => { S.maxHeight = +e.target.value; if (S.preset === 'speed' || S.preset === 'quality') S.preset = 'custom'; save(); syncUI(); sendCfg(); };
  $$('#fit button').forEach((b) => b.onclick = () => { S.fit = b.dataset.fit; zoom = 1; save(); syncUI(); layout(); });

  // input
  $$('#tmode button').forEach((b) => b.onclick = () => { S.mode = b.dataset.mode; save(); syncUI(); placeCursor(); });
  $('#speed').oninput = (e) => { S.speed = +e.target.value; save(); syncUI(); };
  $('#natural').onchange = (e) => { S.natural = e.target.checked; save(); };
  $('#relmouse').onchange = (e) => { S.relmouse = e.target.checked; save(); if (!S.relmouse && document.exitPointerLock) document.exitPointerLock(); else toast('Click the screen to capture the mouse. Press Esc to release.'); };
  $('#haptics').onchange = (e) => { S.haptics = e.target.checked; save(); };
  $('#localCursor').onchange = (e) => { S.localCursor = e.target.checked; save(); placeCursor(); };

  // shortcuts
  $$('[data-combo]').forEach((b) => b.onclick = () => { send({ t: 'combo', k: b.dataset.combo.split(',') }); vib(10); });
  $('#typeBtn').onclick = () => { const v = $('#typeText').value; if (v) { send({ t: 'txt', s: v }); $('#typeText').value = ''; toast('Typed on PC'); } };

  // clipboard
  $('#clipAuto').onchange = (e) => { S.clipAuto = e.target.checked; save(); };
  $('#clipSend').onclick = () => { const s = $('#clipText').value; lastClip = s; send({ t: 'clip', s }); toast('Clipboard sent to PC'); };
  $('#clipCopy').onclick = async () => { if (KD.setClipboard) await KD.setClipboard($('#clipText').value); toast('Copied to this device'); };

  // power
  $$('[data-power]').forEach((b) => b.onclick = async () => {
    const a = b.dataset.power;
    const danger = { restart: 'Restart the PC?', shutdown: 'Shut down the PC? You will need Wake-on-LAN or physical access to turn it back on.', signout: 'Sign out of Windows? Open apps will close.', sleep: 'Put the PC to sleep? You can wake it with Wake-on-LAN.' };
    if (danger[a] && !(await confirmBox(danger[a]))) return;
    send({ t: 'power', a }); toast('Sent: ' + b.textContent.trim());
  });

  document.addEventListener('visibilitychange', () => { if (!document.hidden) { needKey = true; send({ t: 'kf' }); } });
  document.addEventListener('pointerlockchange', () => { if (!document.pointerLockElement && S.relmouse) toast('Mouse released'); });
  window.kdBack = () => { // Android back button
    if (!$('#drawer').hidden) { toggleDrawer(false); return true; }
    if (!$('#keybar').hidden) { showKeyboard(false); return true; }
    toggleDrawer(true); return true;
  };
  syncUI(); layout();
}

function syncUI() {
  $$('#presets button').forEach((b) => b.classList.toggle('on', b.dataset.preset === S.preset));
  $('#custom').hidden = S.preset !== 'custom' && S.preset !== 'auto';
  $('#fps').parentElement.hidden = $('#br').parentElement.hidden = S.preset !== 'custom';
  $('#fps').value = S.fps; $('#fpsVal').textContent = S.fps + ' fps';
  $('#br').value = S.bitrate; $('#brVal').textContent = S.bitrate + ' Mbps';
  $('#res').value = String(S.maxHeight | 0);
  $$('#fit button').forEach((b) => b.classList.toggle('on', b.dataset.fit === S.fit));
  $$('#tmode button').forEach((b) => b.classList.toggle('on', b.dataset.mode === S.mode));
  $('#modeHelp').innerHTML = S.mode === 'trackpad'
    ? 'Drag one finger to move the pointer · tap = click · two-finger tap = right-click · two-finger drag = scroll · double-tap & drag (or long-press) = drag · pinch = zoom · three-finger tap = keyboard'
    : 'Tap where you want to click · drag = click & drag · long-press = right-click · two-finger drag = scroll · pinch = zoom · three-finger tap = keyboard';
  $('#tmode').parentElement.querySelector('h3').hidden = $('#tmode').hidden = !isTouch;
  $('#speed').value = S.speed; $('#spdVal').textContent = S.speed.toFixed(1) + '×';
  $('#natural').checked = S.natural; $('#relmouse').checked = S.relmouse; $('#haptics').checked = S.haptics;
  $('#localCursor').checked = S.localCursor; $('#clipAuto').checked = S.clipAuto;
  $('#qaAudio').classList.toggle('on', S.audio); $('#qaStats').classList.toggle('on', S.stats);
  $('#qaFull').classList.toggle('on', !!document.fullscreenElement);
  $$('#monitors button').forEach((b) => b.classList.toggle('on', +b.dataset.mon === (S.monitor | 0)));
}

function buildMonitors(mons) {
  const box = $('#monitors'); box.innerHTML = '';
  if ((S.monitor | 0) >= mons.length) S.monitor = 0;
  mons.forEach((m, i) => {
    const b = document.createElement('button');
    b.dataset.mon = i;
    b.textContent = `${m.primary ? '★ ' : ''}Screen ${i + 1} · ${m.w}×${m.h}`;
    b.onclick = () => { S.monitor = i; save(); syncUI(); sendCfg(); toast('Switching to screen ' + (i + 1)); };
    box.appendChild(b);
  });
  if (mons.length < 2) box.parentElement.querySelector('h3').textContent = 'Screen';
  syncUI();
}

function toggleDrawer(force) {
  const d = $('#drawer'), open = force === undefined ? d.hidden : force;
  d.hidden = !open; $('#scrim').hidden = !open;
  $('#bubble').style.visibility = open ? 'hidden' : '';
  if (open) { syncUI(); if (document.pointerLockElement) document.exitPointerLock(); }
  else canvas.focus({ preventScroll: true });
}

async function toggleFullscreen() {
  try {
    if (KD.fullscreen && KD.kind === 'android') { S._fs = !S._fs; KD.fullscreen(S._fs); }
    else if (!document.fullscreenElement) {
      await document.documentElement.requestFullscreen({ navigationUI: 'hide' });
      if (navigator.keyboard && navigator.keyboard.lock) navigator.keyboard.lock().catch(() => {}); // capture Win, Alt+Tab, Esc
    } else await document.exitFullscreen();
  } catch (e) { toast('Fullscreen not available'); }
  setTimeout(() => { syncUI(); layout(); }, 200);
}
document.addEventListener('fullscreenchange', () => { syncUI(); layout(); });

function setEmbedMode(m) {
  if (!EMBED || (m !== 'thumb' && m !== 'main') || m === embedMode) return;
  embedMode = m;
  document.documentElement.classList.toggle('thumb', m === 'thumb');
  document.documentElement.classList.toggle('main', m === 'main');
  sendCfg();
  if (m === 'thumb') { stopAudio(); toggleDrawer(false); send({ t: 'release' }); }
  else { if (S.audio && hello && hello.features.audio) startAudio(); canvas.focus({ preventScroll: true }); }
  layout();
}
if (EMBED) {
  window.addEventListener('message', (e) => { if (e.origin === location.origin && e.data && e.data.kd === 'mode') setEmbedMode(e.data.mode); });
  try { parent.postMessage({ kd: 'ready' }, location.origin); } catch (e) {}
}

function goHome() {
  if (EMBED) { try { parent.postMessage({ kd: 'closed' }, location.origin); } catch (e) {} return; }
  if (VIA_HUB) { location.href = '../../'; return; } // back to the hub's device list
  if (KD.home) return KD.home();
  location.reload();
}

async function disconnect() {
  wantOpen = false;
  send({ t: 'release' });
  if (ws) ws.close();
  stopAudio();
  if (KD.keepAwake) KD.keepAwake(false);
  if (VIA_HUB || KD.home) return goHome();
  $('#session').hidden = true;
  showLogin('Disconnected.');
}

boot();
