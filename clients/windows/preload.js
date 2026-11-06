const { contextBridge, ipcRenderer } = require('electron');
const inv = (ch) => (...a) => ipcRenderer.invoke(ch, ...a);
contextBridge.exposeInMainWorld('kdElectron', {
  home: inv('home'), openExternal: inv('openExternal'),
  getClipboard: inv('getClipboard'), setClipboard: inv('setClipboard'),
  download: inv('download'), keepAwake: inv('keepAwake'), fullscreen: inv('fullscreen'),
  deviceName: inv('deviceName'), vibrate: () => {},
});
