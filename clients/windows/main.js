// KadduDesk desktop app (Windows + Linux). A thin native window around the bundled
// KadduDesk core: the core serves the app UI on a private loopback port and does all
// networking (discovery, certificate pinning, hubs, Anywhere, sessions) and sharing.
const { app, BrowserWindow, ipcMain, clipboard, Menu, powerSaveBlocker, shell, dialog } = require('electron');
const path = require('path');
const fs = require('fs');
const os = require('os');
const { spawn } = require('child_process');

app.commandLine.appendSwitch('force_high_performance_gpu');
app.commandLine.appendSwitch('disable-renderer-backgrounding');
app.commandLine.appendSwitch('disable-background-timer-throttling');
app.commandLine.appendSwitch('disable-backgrounding-occluded-windows');
app.commandLine.appendSwitch('enable-gpu-rasterization');
app.commandLine.appendSwitch('enable-zero-copy');

let win = null, core = null, origin = '', homeURL = '', awakeId = null, quitting = false;

if (!app.requestSingleInstanceLock()) app.quit();
app.on('second-instance', () => { if (win) { if (win.isMinimized()) win.restore(); win.show(); win.focus(); } });

function corePath() {
  const exe = process.platform === 'win32' ? 'KadduDeskServer.exe' : 'kaddudesk-server';
  const dev = path.join(__dirname, '..', '..', 'build', process.platform === 'win32' ? 'core-win' : 'core-linux', exe);
  return [process.env.KD_CORE, path.join(process.resourcesPath || '', 'core', exe), dev].find((p) => p && fs.existsSync(p));
}

function startCore() {
  return new Promise((resolve, reject) => {
    const p = corePath();
    if (!p) return reject(new Error('The KadduDesk core is missing from this installation. Please reinstall.'));
    core = spawn(p, ['-app', '-parentwatch', '-apppath', process.execPath], { stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
    let buf = '', err = '';
    core.stdout.on('data', (d) => {
      buf += d;
      const m = buf.match(/KDAPP (\S+)/);
      if (m && !origin) { const u = new URL(m[1]); origin = u.origin; homeURL = origin + '/app/'; resolve(m[1]); }
    });
    core.stderr.on('data', (d) => { err += d; if (err.length > 8000) err = err.slice(-8000); });
    core.on('exit', (code) => {
      if (!origin) reject(new Error('KadduDesk core stopped (' + code + ').\n' + err.slice(-600)));
      else if (!quitting) dialog.showMessageBox({ type: 'error', title: 'KadduDesk', message: 'The KadduDesk core stopped unexpectedly.', detail: err.slice(-600) }).then(() => app.quit());
    });
  });
}

const ours = (u) => { try { return origin && new URL(u).origin === origin; } catch (e) { return false; } };

function createWindow() {
  win = new BrowserWindow({
    width: 1280, height: 820, minWidth: 420, minHeight: 360, backgroundColor: '#0f1115', title: 'KadduDesk', autoHideMenuBar: true, show: false,
    icon: path.join(__dirname, '..', '..', 'build', process.platform === 'win32' ? 'icon.ico' : 'icon.png'),
    webPreferences: { preload: path.join(__dirname, 'preload.js'), contextIsolation: true, sandbox: true, nodeIntegrationInSubFrames: true, backgroundThrottling: false, spellcheck: false },
  });
  Menu.setApplicationMenu(null);
  win.loadURL('data:text/html;charset=utf-8,' + encodeURIComponent('<body style="background:#0f1115;color:#9aa1ad;font:15px system-ui;display:grid;place-items:center;height:95vh;margin:0">Starting KadduDesk…</body>'));
  win.once('ready-to-show', () => win.show());
  win.webContents.on('will-navigate', (e, u) => { if (!ours(u)) { e.preventDefault(); if (/^https:\/\//.test(u)) shell.openExternal(u); } });
  win.webContents.setWindowOpenHandler(({ url }) => { if (/^https:\/\//.test(url)) shell.openExternal(url); return { action: 'deny' }; });
  win.webContents.on('page-title-updated', (e, t) => win.setTitle(t));
  win.webContents.session.setPermissionRequestHandler((wc, perm, cb, d) => cb(ours(d.requestingUrl || wc.getURL())));
  win.webContents.session.setPermissionCheckHandler((wc, perm, o) => ours(o));
  win.webContents.session.on('will-download', (e, item) => item.setSaveDialogOptions({ defaultPath: path.join(app.getPath('downloads'), item.getFilename()) }));
}

app.whenReady().then(async () => {
  createWindow();
  try {
    const url = await startCore();
    win.loadURL(url);
  } catch (e) {
    await dialog.showMessageBox(win, { type: 'error', title: 'KadduDesk', message: 'KadduDesk could not start', detail: String(e.message || e) });
    app.quit();
  }
});
app.on('window-all-closed', () => app.quit());
app.on('before-quit', () => { quitting = true; if (core) { try { core.stdin.end(); core.kill(); } catch (e) {} } });

// ------------------------------------------------------------------ native helpers for the UI
function handle(ch, fn) {
  ipcMain.handle(ch, (e, ...args) => {
    if (!ours(e.senderFrame.url)) throw new Error('untrusted sender');
    return fn(...args);
  });
}
handle('home', () => { win.setFullScreen(false); win.loadURL(homeURL); });
handle('openExternal', (u) => { if (/^https:\/\//.test(String(u))) shell.openExternal(String(u)); });
handle('getClipboard', () => clipboard.readText());
handle('setClipboard', (s) => clipboard.writeText(String(s)));
handle('deviceName', () => os.hostname());
handle('fullscreen', (on) => win.setFullScreen(!!on));
handle('download', (u) => { if (ours(u)) win.webContents.downloadURL(u); });
handle('keepAwake', (on) => {
  if (on && awakeId === null) awakeId = powerSaveBlocker.start('prevent-display-sleep');
  if (!on && awakeId !== null) { powerSaveBlocker.stop(awakeId); awakeId = null; }
});
