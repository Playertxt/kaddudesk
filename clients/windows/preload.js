const { contextBridge, ipcRenderer } = require('electron');
const inv = (ch) => (...a) => ipcRenderer.invoke(ch, ...a);
contextBridge.exposeInMainWorld('kdElectron', {
  discover: inv('discover'), probe: inv('probe'), trust: inv('trust'),
  getHosts: inv('getHosts'), saveHosts: inv('saveHosts'), open: inv('open'), home: inv('home'),
  wake: inv('wake'), getClipboard: inv('getClipboard'), setClipboard: inv('setClipboard'),
  download: inv('download'), keepAwake: inv('keepAwake'), fullscreen: inv('fullscreen'),
  deviceName: inv('deviceName'), vibrate: () => {},
});
