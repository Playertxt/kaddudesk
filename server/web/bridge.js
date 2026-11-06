// KadduDesk native bridge: one async API over Electron (preload), Android (JS interface) or a plain browser.
(function () {
  const cbs = {};
  let seq = 0;
  // Android calls back into this with results of async work
  window.__kdcb = function (id, json) {
    const f = cbs[id];
    delete cbs[id];
    if (f) { try { f(JSON.parse(json)); } catch (e) { f(null); } }
  };

  let api;
  if (window.kdElectron) {
    api = Object.assign({ kind: 'electron' }, window.kdElectron);
  } else if (window.KDAndroid) {
    const A = window.KDAndroid;
    const call = (m, ...args) => new Promise((res) => {
      const id = 'c' + (++seq);
      cbs[id] = res;
      A[m](id, ...args.map((a) => (typeof a === 'string' ? a : JSON.stringify(a))));
    });
    api = {
      kind: 'android',
      discover: () => call('discover'),
      probe: (host, port) => call('probe', host, String(port)),
      trust: async (host, fp) => A.trust(host, fp),
      getHosts: async () => JSON.parse(A.getHosts() || '[]'),
      saveHosts: async (list) => A.saveHosts(JSON.stringify(list)),
      open: async (url) => A.open(url),
      home: async () => A.home(),
      wake: async (mac, bcast) => A.wake(mac, bcast || ''),
      getClipboard: async () => A.getClipboard(),
      setClipboard: async (s) => A.setClipboard(s),
      download: async (url, name) => A.download(url, name),
      keepAwake: async (on) => A.keepAwake(!!on),
      fullscreen: async (on) => A.fullscreen(!!on),
      vibrate: (ms) => A.vibrate(ms | 0),
      deviceName: async () => A.deviceName(),
      showKeyboard: (on) => A.showKeyboard(!!on),
      openExternal: (u) => A.openUrl(u),
      startSharing: () => A.startSharing(),
      stopSharing: () => A.stopSharing(),
      sharingState: () => A.sharingState(),
      openAccessibility: () => A.openAccessibility(),
    };
  } else {
    api = {
      kind: 'browser',
      getHosts: async () => { try { return JSON.parse(localStorage.kd_hosts || '[]'); } catch (e) { return []; } },
      saveHosts: async (l) => { localStorage.kd_hosts = JSON.stringify(l); },
      getClipboard: async () => { try { return await navigator.clipboard.readText(); } catch (e) { return null; } },
      setClipboard: async (s) => { try { await navigator.clipboard.writeText(s); } catch (e) {} },
      vibrate: (ms) => { try { navigator.vibrate && navigator.vibrate(ms); } catch (e) {} },
      deviceName: async () => {
        const ua = navigator.userAgent;
        if (/Android/i.test(ua)) return 'Android browser';
        if (/iPhone|iPad/i.test(ua)) return 'iPhone/iPad';
        if (/Mac/i.test(ua)) return 'Mac browser';
        return 'Browser';
      },
    };
  }
  window.KD = api;
})();
