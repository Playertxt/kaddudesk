// KadduDesk Windows client (Electron). Hosts the shared launcher and the server's web client,
// pins each PC's self-signed certificate (trust on first use), and provides native helpers.
const { app, BrowserWindow, ipcMain, session, clipboard, Menu, powerSaveBlocker, dialog } = require('electron');
const path = require('path');
const fs = require('fs');
const tls = require('tls');
const https = require('https');
const dgram = require('dgram');
const crypto = require('crypto');
const os = require('os');
const net = require('net');

// low-latency rendering
app.commandLine.appendSwitch('force_high_performance_gpu');
app.commandLine.appendSwitch('disable-renderer-backgrounding');
app.commandLine.appendSwitch('disable-background-timer-throttling');
app.commandLine.appendSwitch('disable-backgrounding-occluded-windows');
app.commandLine.appendSwitch('enable-gpu-rasterization');
app.commandLine.appendSwitch('enable-zero-copy');

const LAUNCHER = path.join(__dirname, '..', 'launcher', 'index.html');
const storeFile = () => path.join(app.getPath('userData'), 'kaddudesk.json');
let store = { hosts: [], pins: {} };
function loadStore() { try { store = Object.assign(store, JSON.parse(fs.readFileSync(storeFile(), 'utf8'))); } catch (e) {} }
function saveStore() { try { fs.writeFileSync(storeFile(), JSON.stringify(store, null, 2)); } catch (e) {} }

const fpOfDer = (der) => crypto.createHash('sha256').update(der).digest('hex').toUpperCase();
const fpOfPem = (pem) => fpOfDer(Buffer.from(pem.replace(/-----[^-]+-----/g, '').replace(/\s+/g, ''), 'base64'));

let win = null;
let awakeId = null;

if (!app.requestSingleInstanceLock()) app.quit();
app.on('second-instance', () => { if (win) { if (win.isMinimized()) win.restore(); win.focus(); } });

function trustedUrl(u) {
  try {
    const url = new URL(u);
    if (url.protocol === 'file:') return true;
    return url.protocol === 'https:' && !!store.pins[url.hostname.replace(/^\[|\]$/g, '')];
  } catch (e) { return false; }
}

function createWindow() {
  win = new BrowserWindow({
    width: 1280, height: 800, minWidth: 480, minHeight: 360,
    backgroundColor: '#0f1115', title: 'KadduDesk', autoHideMenuBar: true,
    icon: path.join(__dirname, '..', '..', 'build', 'icon.ico'),
    webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, sandbox: true, backgroundThrottling: false, spellcheck: false },
  });
  Menu.setApplicationMenu(null);
  win.loadFile(LAUNCHER);
  win.webContents.on('will-navigate', (e, u) => { if (!trustedUrl(u)) e.preventDefault(); });
  win.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
  win.webContents.on('page-title-updated', (e, t) => win.setTitle(t));
  win.webContents.on('did-fail-load', (e, code, desc, url) => {
    if (url.startsWith('https:')) dialog.showMessageBox(win, { type: 'error', title: 'KadduDesk', message: 'Could not open the PC: ' + desc }).then(() => win.loadFile(LAUNCHER));
  });
  // allow fullscreen + keyboard lock + clipboard for pinned PCs
  session.defaultSession.setPermissionRequestHandler((wc, perm, cb, details) => cb(trustedUrl(details.requestingUrl || wc.getURL())));
  session.defaultSession.setPermissionCheckHandler((wc, perm, origin) => trustedUrl(origin));
  session.defaultSession.on('will-download', (e, item) => {
    item.setSaveDialogOptions({ defaultPath: path.join(app.getPath('downloads'), item.getFilename()) });
  });
}

app.whenReady().then(() => {
  loadStore();
  // certificate pinning: accept a PC's self-signed cert only if it matches the trusted fingerprint
  session.defaultSession.setCertificateVerifyProc((req, cb) => {
    const pin = store.pins[req.hostname.replace(/^\[|\]$/g, '')];
    if (!pin) return cb(-3);
    try { cb(fpOfPem(req.certificate.data) === pin ? 0 : -2); } catch (e) { cb(-2); }
  });
  createWindow();
});
app.on('window-all-closed', () => app.quit());

// ------------------------------------------------------------------ IPC (only from trusted pages)
function handle(ch, fn) {
  ipcMain.handle(ch, (e, ...args) => {
    if (!trustedUrl(e.senderFrame.url)) throw new Error('untrusted sender');
    return fn(...args);
  });
}

handle('getHosts', () => store.hosts);
handle('saveHosts', (list) => { if (Array.isArray(list)) { store.hosts = list.slice(0, 200); saveStore(); } });
handle('trust', (host, fp) => { if (typeof host === 'string' && /^[0-9A-F]{64}$/.test(fp)) { store.pins[host] = fp; saveStore(); } });
handle('open', (u) => { if (trustedUrl(u)) win.loadURL(u); });
handle('home', () => { win.setFullScreen(false); win.loadFile(LAUNCHER); });
handle('getClipboard', () => clipboard.readText());
handle('setClipboard', (s) => clipboard.writeText(String(s)));
handle('deviceName', () => os.hostname());
handle('fullscreen', (on) => win.setFullScreen(!!on));
handle('download', (u) => { if (trustedUrl(u)) win.webContents.downloadURL(u); });
handle('keepAwake', (on) => {
  if (on && awakeId === null) awakeId = powerSaveBlocker.start('prevent-display-sleep');
  if (!on && awakeId !== null) { powerSaveBlocker.stop(awakeId); awakeId = null; }
});

handle('probe', (host, port) => new Promise((resolve) => {
  port = +port || 47800;
  const sock = tls.connect({ host, port, rejectUnauthorized: false, servername: net.isIP(host) ? undefined : host, timeout: 4000 });
  const fail = (err) => { sock.destroy(); resolve({ ok: false, error: err }); };
  sock.on('timeout', () => fail('timed out'));
  sock.on('error', (e) => fail(e.code || e.message));
  sock.on('secureConnect', () => {
    const cert = sock.getPeerCertificate();
    const fp = fpOfDer(cert.raw);
    sock.end();
    const req = https.get({ host, port, path: '/api/info', rejectUnauthorized: false, timeout: 4000,
      checkServerIdentity: () => undefined }, (res) => {
      let body = '';
      res.on('data', (d) => (body += d));
      res.on('end', () => {
        const c = res.socket && res.socket.getPeerCertificate && res.socket.getPeerCertificate();
        let info = null;
        try { info = JSON.parse(body); } catch (e) {}
        if (c && c.raw && fpOfDer(c.raw) !== fp) return resolve({ ok: false, error: 'certificate mismatch' });
        resolve({ ok: true, fp, info });
      });
    });
    req.on('error', () => resolve({ ok: true, fp, info: null }));
    req.on('timeout', () => { req.destroy(); resolve({ ok: true, fp, info: null }); });
  });
}));

function broadcasts() {
  const out = new Set(['255.255.255.255']);
  for (const list of Object.values(os.networkInterfaces())) {
    for (const a of list || []) {
      if (a.family !== 'IPv4' || a.internal) continue;
      const ip = a.address.split('.').map(Number), mask = a.netmask.split('.').map(Number);
      out.add(ip.map((b, i) => (b | (~mask[i] & 255))).join('.'));
    }
  }
  return [...out];
}

handle('discover', () => new Promise((resolve) => {
  const found = new Map();
  const s = dgram.createSocket('udp4');
  s.on('message', (msg, rinfo) => {
    try {
      const j = JSON.parse(msg.toString());
      if (j.app === 'kaddudesk') found.set(rinfo.address + ':' + j.port, { host: rinfo.address, port: j.port, name: j.name, fp: j.fp, macs: j.macs });
    } catch (e) {}
  });
  s.on('error', () => {});
  s.bind(0, () => {
    s.setBroadcast(true);
    const ping = Buffer.from('KADDUDESK?');
    const shoot = () => broadcasts().forEach((b) => s.send(ping, 47810, b, () => {}));
    shoot(); setTimeout(shoot, 400);
  });
  setTimeout(() => { try { s.close(); } catch (e) {} resolve([...found.values()]); }, 1300);
}));

handle('wake', (mac, bcast) => new Promise((resolve) => {
  const hex = String(mac).replace(/[^0-9a-f]/gi, '');
  if (hex.length !== 12) return resolve(false);
  const m = Buffer.from(hex, 'hex');
  const pkt = Buffer.alloc(102, 0xff);
  for (let i = 0; i < 16; i++) m.copy(pkt, 6 + i * 6);
  const s = dgram.createSocket('udp4');
  s.bind(0, () => {
    s.setBroadcast(true);
    const targets = new Set([...broadcasts(), bcast].filter(Boolean));
    let n = 0;
    for (const t of targets) for (const port of [9, 7]) { n++; s.send(pkt, port, t, () => { if (--n === 0) { s.close(); resolve(true); } }); }
  });
}));
