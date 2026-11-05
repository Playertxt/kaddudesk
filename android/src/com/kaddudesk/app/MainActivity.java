package com.kaddudesk.app;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.ContentValues;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ActivityInfo;
import android.content.pm.ApplicationInfo;
import android.graphics.Rect;
import android.net.Uri;
import android.net.http.SslCertificate;
import android.net.http.SslError;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.os.VibrationEffect;
import android.os.Vibrator;
import android.provider.MediaStore;
import android.util.Log;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.webkit.ConsoleMessage;
import android.webkit.JavascriptInterface;
import android.webkit.SslErrorHandler;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.HttpURLConnection;
import java.net.InetAddress;
import java.net.InterfaceAddress;
import java.net.NetworkInterface;
import java.net.SocketTimeoutException;
import java.net.URL;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.Collections;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.FutureTask;
import java.util.concurrent.TimeUnit;

import javax.net.ssl.HostnameVerifier;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

public class MainActivity extends Activity {
    static final String TAG = "KadduDesk";
    static final String LAUNCHER = "file:///android_asset/launcher/index.html";
    static final int REQ_FILE = 7;

    WebView web;
    FrameLayout root;
    SharedPreferences prefs;
    final Handler ui = new Handler(Looper.getMainLooper());
    ValueCallback<Uri[]> fileCb;
    boolean immersive = true;

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        requestWindowFeature(Window.FEATURE_NO_TITLE);
        prefs = getSharedPreferences("kaddudesk", MODE_PRIVATE);
        Window w = getWindow();
        w.setFlags(WindowManager.LayoutParams.FLAG_HARDWARE_ACCELERATED, WindowManager.LayoutParams.FLAG_HARDWARE_ACCELERATED);
        if (Build.VERSION.SDK_INT >= 28) {
            WindowManager.LayoutParams lp = w.getAttributes();
            lp.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
            w.setAttributes(lp);
        }
        w.setStatusBarColor(0xFF0F1115);
        w.setNavigationBarColor(0xFF0F1115);
        if ((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) WebView.setWebContentsDebuggingEnabled(true);

        root = new FrameLayout(this);
        root.setBackgroundColor(0xFF0F1115);
        web = new WebView(this);
        root.addView(web, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        setContentView(root);

        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setDatabaseEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setAllowFileAccess(true);
        s.setAllowContentAccess(false);
        s.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        s.setSupportZoom(false);
        s.setBuiltInZoomControls(false);
        s.setDisplayZoomControls(false);
        s.setUseWideViewPort(true);
        s.setLoadWithOverviewMode(true);
        s.setTextZoom(100);
        s.setOffscreenPreRaster(true);
        web.setBackgroundColor(0xFF0F1115);
        web.setOverScrollMode(View.OVER_SCROLL_NEVER);
        web.setLayerType(View.LAYER_TYPE_HARDWARE, null);
        web.addJavascriptInterface(new Bridge(), "KDAndroid");
        web.setWebViewClient(new Client());
        web.setWebChromeClient(new Chrome());

        // keep the page sized above the soft keyboard even in immersive fullscreen
        root.getViewTreeObserver().addOnGlobalLayoutListener(() -> {
            Rect r = new Rect();
            root.getWindowVisibleDisplayFrame(r);
            int full = root.getRootView().getHeight();
            int kb = full - r.bottom;
            if (kb < full * 0.15) kb = 0;
            FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) web.getLayoutParams();
            if (lp.bottomMargin != kb) { lp.bottomMargin = kb; web.setLayoutParams(lp); }
        });

        applyImmersive();
        web.loadUrl(LAUNCHER);
    }

    void applyImmersive() {
        View d = getWindow().getDecorView();
        int f = View.SYSTEM_UI_FLAG_LAYOUT_STABLE;
        if (immersive) f |= View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_FULLSCREEN;
        d.setSystemUiVisibility(f);
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (hasFocus) applyImmersive();
    }

    @Override
    protected void onPause() { super.onPause(); web.onPause(); }

    @Override
    protected void onResume() { super.onResume(); web.onResume(); applyImmersive(); }

    @Override
    public void onBackPressed() {
        web.evaluateJavascript("(window.kdBack && window.kdBack()) ? 1 : 0", v -> {
            if ("1".equals(v)) return;
            String u = web.getUrl();
            if (u == null || u.startsWith("file:")) finish();
            else goHome();
        });
    }

    void goHome() {
        getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        web.loadUrl(LAUNCHER);
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        if (req != REQ_FILE || fileCb == null) { super.onActivityResult(req, res, data); return; }
        Uri[] out = null;
        if (res == RESULT_OK && data != null) {
            if (data.getClipData() != null) {
                int n = data.getClipData().getItemCount();
                out = new Uri[n];
                for (int i = 0; i < n; i++) out[i] = data.getClipData().getItemAt(i).getUri();
            } else if (data.getData() != null) out = new Uri[]{data.getData()};
        }
        fileCb.onReceiveValue(out);
        fileCb = null;
    }

    // ------------------------------------------------------------------ certificate pinning

    static String sha256(byte[] b) {
        try {
            byte[] d = MessageDigest.getInstance("SHA-256").digest(b);
            StringBuilder sb = new StringBuilder();
            for (byte x : d) sb.append(String.format(Locale.US, "%02X", x));
            return sb.toString();
        } catch (Exception e) { return ""; }
    }

    static byte[] certDer(SslCertificate c) {
        try {
            if (Build.VERSION.SDK_INT >= 29) return c.getX509Certificate().getEncoded();
            Bundle b = SslCertificate.saveState(c);
            return b.getByteArray("x509-certificate");
        } catch (Exception e) { return null; }
    }

    String pinFor(String host) { return host == null ? null : prefs.getString("pin:" + host, null); }

    boolean allowedUrl(Uri u) {
        if (u == null) return false;
        String s = u.toString();
        if (s.startsWith("file:///android_asset/")) return true;
        return "https".equals(u.getScheme()) && pinFor(u.getHost()) != null;
    }

    class Client extends WebViewClient {
        @Override
        public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest r) {
            return !allowedUrl(r.getUrl());
        }

        @Override
        public void onReceivedSslError(WebView v, SslErrorHandler h, SslError e) {
            String host = Uri.parse(e.getUrl()).getHost();
            String pin = pinFor(host);
            byte[] der = certDer(e.getCertificate());
            if (pin != null && der != null && pin.equals(sha256(der))) h.proceed();
            else {
                h.cancel();
                toast("Security check failed for " + host + " — certificate does not match.");
                ui.post(MainActivity.this::goHome);
            }
        }
    }

    class Chrome extends WebChromeClient {
        @Override
        public boolean onShowFileChooser(WebView v, ValueCallback<Uri[]> cb, FileChooserParams p) {
            if (fileCb != null) fileCb.onReceiveValue(null);
            fileCb = cb;
            Intent i = new Intent(Intent.ACTION_GET_CONTENT);
            i.addCategory(Intent.CATEGORY_OPENABLE);
            i.setType("*/*");
            i.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
            try { startActivityForResult(Intent.createChooser(i, "Send to PC"), REQ_FILE); }
            catch (Exception e) { fileCb = null; return false; }
            return true;
        }

        @Override
        public boolean onConsoleMessage(ConsoleMessage m) {
            Log.d(TAG, m.message() + " @" + m.sourceId() + ":" + m.lineNumber());
            return true;
        }
    }

    // ------------------------------------------------------------------ helpers

    void toast(String s) { ui.post(() -> Toast.makeText(this, s, Toast.LENGTH_LONG).show()); }

    void callback(String id, String json) {
        ui.post(() -> web.evaluateJavascript("window.__kdcb(" + JSONObject.quote(id) + "," + JSONObject.quote(json) + ")", null));
    }

    /** TLS context that accepts any cert but records it, so we can pin / compare fingerprints ourselves. */
    static class Capture implements X509TrustManager {
        volatile X509Certificate seen;
        final String expect;
        Capture(String expect) { this.expect = expect; }
        public void checkClientTrusted(X509Certificate[] c, String a) {}
        public void checkServerTrusted(X509Certificate[] c, String a) throws java.security.cert.CertificateException {
            seen = c[0];
            if (expect != null && !expect.equals(sha256(c[0].getEncoded())))
                throw new java.security.cert.CertificateException("fingerprint mismatch");
        }
        public X509Certificate[] getAcceptedIssuers() { return new X509Certificate[0]; }
    }

    static HttpsURLConnection openPinned(String url, Capture cap) throws Exception {
        SSLContext ctx = SSLContext.getInstance("TLS");
        ctx.init(null, new TrustManager[]{cap}, new SecureRandom());
        HttpsURLConnection c = (HttpsURLConnection) new URL(url).openConnection();
        c.setSSLSocketFactory(ctx.getSocketFactory());
        HostnameVerifier any = (h, s) -> true; // identity is enforced by the fingerprint pin instead
        c.setHostnameVerifier(any);
        c.setConnectTimeout(4000);
        c.setReadTimeout(8000);
        return c;
    }

    static String readAll(InputStream in) throws Exception {
        ByteArrayOutputStream bo = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) > 0) bo.write(buf, 0, n);
        return bo.toString("UTF-8");
    }

    Set<InetAddress> broadcasts() {
        Set<InetAddress> out = new HashSet<>();
        try { out.add(InetAddress.getByName("255.255.255.255")); } catch (Exception ignored) {}
        try {
            for (NetworkInterface ni : Collections.list(NetworkInterface.getNetworkInterfaces())) {
                if (!ni.isUp() || ni.isLoopback()) continue;
                for (InterfaceAddress ia : ni.getInterfaceAddresses()) if (ia.getBroadcast() != null) out.add(ia.getBroadcast());
            }
        } catch (Exception ignored) {}
        return out;
    }

    // ------------------------------------------------------------------ JS bridge

    class Bridge {
        @JavascriptInterface
        public void discover(String cb) {
            new Thread(() -> {
                WifiManager wm = (WifiManager) getApplicationContext().getSystemService(Context.WIFI_SERVICE);
                WifiManager.MulticastLock lock = wm != null ? wm.createMulticastLock("kd") : null;
                Map<String, JSONObject> found = new LinkedHashMap<>();
                try (DatagramSocket s = new DatagramSocket()) {
                    if (lock != null) lock.acquire();
                    s.setBroadcast(true);
                    s.setSoTimeout(250);
                    byte[] ping = "KADDUDESK?".getBytes("UTF-8");
                    for (int round = 0; round < 2; round++)
                        for (InetAddress a : broadcasts()) {
                            try { s.send(new DatagramPacket(ping, ping.length, a, 47810)); } catch (Exception ignored) {}
                        }
                    long end = System.currentTimeMillis() + 1300;
                    byte[] buf = new byte[4096];
                    while (System.currentTimeMillis() < end) {
                        DatagramPacket p = new DatagramPacket(buf, buf.length);
                        try { s.receive(p); } catch (SocketTimeoutException t) { continue; }
                        try {
                            JSONObject j = new JSONObject(new String(p.getData(), 0, p.getLength(), "UTF-8"));
                            if (!"kaddudesk".equals(j.optString("app"))) continue;
                            String host = p.getAddress().getHostAddress();
                            JSONObject o = new JSONObject();
                            o.put("host", host); o.put("port", j.optInt("port", 47800));
                            o.put("name", j.optString("name")); o.put("fp", j.optString("fp"));
                            o.put("macs", j.optJSONArray("macs"));
                            found.put(host + ":" + o.optInt("port"), o);
                        } catch (Exception ignored) {}
                    }
                } catch (Exception e) {
                    Log.w(TAG, "discover", e);
                } finally {
                    if (lock != null && lock.isHeld()) lock.release();
                }
                JSONArray arr = new JSONArray();
                for (JSONObject o : found.values()) arr.put(o);
                callback(cb, arr.toString());
            }).start();
        }

        @JavascriptInterface
        public void probe(String cb, String host, String port) {
            new Thread(() -> {
                JSONObject r = new JSONObject();
                Capture cap = new Capture(null);
                try {
                    String h = host.contains(":") ? "[" + host + "]" : host;
                    HttpsURLConnection c = openPinned("https://" + h + ":" + port + "/api/info", cap);
                    String body = readAll(c.getInputStream());
                    r.put("ok", true);
                    r.put("fp", sha256(cap.seen.getEncoded()));
                    try { r.put("info", new JSONObject(body)); } catch (Exception ignored) {}
                } catch (Exception e) {
                    try {
                        if (cap.seen != null) { r.put("ok", true); r.put("fp", sha256(cap.seen.getEncoded())); }
                        else { r.put("ok", false); r.put("error", e.getClass().getSimpleName() + ": " + e.getMessage()); }
                    } catch (Exception ignored) {}
                }
                callback(cb, r.toString());
            }).start();
        }

        @JavascriptInterface
        public void trust(String host, String fp) {
            if (host != null && fp != null && fp.matches("[0-9A-F]{64}")) prefs.edit().putString("pin:" + host, fp).apply();
        }

        @JavascriptInterface
        public String getHosts() { return prefs.getString("hosts", "[]"); }

        @JavascriptInterface
        public void saveHosts(String json) { prefs.edit().putString("hosts", json).apply(); }

        @JavascriptInterface
        public void open(String url) {
            ui.post(() -> { if (allowedUrl(Uri.parse(url))) web.loadUrl(url); });
        }

        @JavascriptInterface
        public void home() { ui.post(MainActivity.this::goHome); }

        @JavascriptInterface
        public void wake(String mac, String bcast) {
            new Thread(() -> {
                String hex = mac.replaceAll("[^0-9a-fA-F]", "");
                if (hex.length() != 12) return;
                byte[] m = new byte[6];
                for (int i = 0; i < 6; i++) m[i] = (byte) Integer.parseInt(hex.substring(i * 2, i * 2 + 2), 16);
                byte[] pkt = new byte[102];
                for (int i = 0; i < 6; i++) pkt[i] = (byte) 0xFF;
                for (int i = 0; i < 16; i++) System.arraycopy(m, 0, pkt, 6 + i * 6, 6);
                try (DatagramSocket s = new DatagramSocket()) {
                    s.setBroadcast(true);
                    Set<InetAddress> t = broadcasts();
                    try { if (bcast != null && !bcast.isEmpty()) t.add(InetAddress.getByName(bcast)); } catch (Exception ignored) {}
                    for (InetAddress a : t) for (int port : new int[]{9, 7}) s.send(new DatagramPacket(pkt, pkt.length, a, port));
                } catch (Exception e) { Log.w(TAG, "wol", e); }
            }).start();
        }

        @JavascriptInterface
        public String getClipboard() {
            FutureTask<String> t = new FutureTask<>(() -> {
                ClipboardManager cm = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
                if (cm == null || !cm.hasPrimaryClip() || cm.getPrimaryClip().getItemCount() == 0) return "";
                CharSequence cs = cm.getPrimaryClip().getItemAt(0).coerceToText(MainActivity.this);
                return cs == null ? "" : cs.toString();
            });
            ui.post(t);
            try { return t.get(1500, TimeUnit.MILLISECONDS); } catch (Exception e) { return ""; }
        }

        @JavascriptInterface
        public void setClipboard(String s) {
            ui.post(() -> {
                ClipboardManager cm = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
                if (cm != null) cm.setPrimaryClip(ClipData.newPlainText("KadduDesk", s));
            });
        }

        @JavascriptInterface
        public void download(String url, String name) {
            new Thread(() -> {
                try {
                    Uri u = Uri.parse(url);
                    String pin = pinFor(u.getHost());
                    if (pin == null) throw new Exception("PC not trusted");
                    HttpsURLConnection c = openPinned(url, new Capture(pin));
                    c.setReadTimeout(30000);
                    InputStream in = c.getInputStream();
                    String safe = name.replaceAll("[\\\\/:*?\"<>|]", "_");
                    OutputStream out;
                    String where;
                    if (Build.VERSION.SDK_INT >= 29) {
                        ContentValues v = new ContentValues();
                        v.put(MediaStore.Downloads.DISPLAY_NAME, safe);
                        v.put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS + "/KadduDesk");
                        Uri dst = getContentResolver().insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, v);
                        out = getContentResolver().openOutputStream(dst);
                        where = "Downloads/KadduDesk";
                    } else {
                        File dir = new File(getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS), "KadduDesk");
                        dir.mkdirs();
                        File f = new File(dir, safe);
                        out = new FileOutputStream(f);
                        where = f.getParent();
                    }
                    byte[] buf = new byte[65536];
                    int n;
                    while ((n = in.read(buf)) > 0) out.write(buf, 0, n);
                    out.close();
                    in.close();
                    toast("Saved " + safe + " to " + where);
                } catch (Exception e) {
                    toast("Download failed: " + e.getMessage());
                }
            }).start();
        }

        @JavascriptInterface
        public void keepAwake(boolean on) {
            ui.post(() -> {
                if (on) getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                else getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            });
        }

        @JavascriptInterface
        public void fullscreen(boolean on) { ui.post(() -> { immersive = on; applyImmersive(); }); }

        @JavascriptInterface
        public void vibrate(int ms) {
            Vibrator v = (Vibrator) getSystemService(VIBRATOR_SERVICE);
            if (v == null || !v.hasVibrator()) return;
            if (Build.VERSION.SDK_INT >= 26) v.vibrate(VibrationEffect.createOneShot(Math.max(1, ms), VibrationEffect.DEFAULT_AMPLITUDE));
            else v.vibrate(ms);
        }

        @JavascriptInterface
        public String deviceName() {
            String m = Build.MANUFACTURER, model = Build.MODEL;
            if (model.toLowerCase(Locale.US).startsWith(m.toLowerCase(Locale.US))) return model;
            return m.substring(0, 1).toUpperCase(Locale.US) + m.substring(1) + " " + model;
        }

        @JavascriptInterface
        public void showKeyboard(boolean on) {
            ui.post(() -> {
                InputMethodManager imm = (InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
                if (imm == null) return;
                if (on) { web.requestFocus(); imm.showSoftInput(web, 0); }
                else imm.hideSoftInputFromWindow(web.getWindowToken(), 0);
            });
        }
    }
}
