package com.kaddudesk.app;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.ContentValues;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.graphics.Rect;
import android.media.projection.MediaProjectionManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.os.VibrationEffect;
import android.os.Vibrator;
import android.provider.MediaStore;
import android.provider.Settings;
import android.util.Log;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowManager;
import android.view.inputmethod.InputMethodManager;
import android.webkit.ConsoleMessage;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.URLUtil;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.FrameLayout;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLDecoder;
import java.util.Locale;
import java.util.concurrent.FutureTask;
import java.util.concurrent.TimeUnit;

/** KadduDesk for Android: hosts the app UI served by the bundled core. */
public class MainActivity extends Activity {
    static final String TAG = "KadduDesk";
    static final int REQ_FILE = 7, REQ_CAPTURE = 8;
    static volatile MainActivity visible;

    WebView web;
    FrameLayout root;
    final Handler ui = new Handler(Looper.getMainLooper());
    ValueCallback<Uri[]> fileCb;
    boolean immersive = false;

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        requestWindowFeature(Window.FEATURE_NO_TITLE);
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
        s.setAllowFileAccess(false);
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
        web.setDownloadListener((url, ua, disp, mime, len) -> download(url, URLUtil.guessFileName(url, disp, mime), disp));
        CookieManager.getInstance().setAcceptCookie(true);

        // keep the page above the soft keyboard
        root.getViewTreeObserver().addOnGlobalLayoutListener(() -> {
            Rect r = new Rect();
            root.getWindowVisibleDisplayFrame(r);
            int full = root.getRootView().getHeight();
            int kb = full - r.bottom;
            if (kb < full * 0.15) kb = 0;
            FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) web.getLayoutParams();
            if (lp.bottomMargin != kb) { lp.bottomMargin = kb; web.setLayoutParams(lp); }
        });

        web.loadData("<body style='background:#0f1115;color:#9aa1ad;font:16px sans-serif;display:flex;align-items:center;justify-content:center;height:90vh'>Starting KadduDesk…</body>", "text/html", "utf-8");
        KDCore.start(this, new KDCore.Callback() {
            public void ok(String url) { ui.post(() -> web.loadUrl(url)); }
            public void fail(String why) { ui.post(() -> web.loadData("<body style='background:#0f1115;color:#ff9a9d;font:16px sans-serif;padding:30px'>KadduDesk could not start: " + why + "</body>", "text/html", "utf-8")); }
        });
    }

    @Override protected void onResume() { super.onResume(); visible = this; web.onResume(); applyImmersive(); }
    @Override protected void onPause() { super.onPause(); if (visible == this) visible = null; web.onPause(); }

    void applyImmersive() {
        View d = getWindow().getDecorView();
        int f = View.SYSTEM_UI_FLAG_LAYOUT_STABLE;
        if (immersive) f |= View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_FULLSCREEN;
        d.setSystemUiVisibility(f);
    }

    @Override
    public void onBackPressed() {
        web.evaluateJavascript("(window.kdBack && window.kdBack()) ? 1 : 0", v -> {
            if ("1".equals(v)) return;
            String u = web.getUrl();
            if (KDCore.origin != null && u != null && u.startsWith(KDCore.origin) && !u.contains("/app/#") && !u.endsWith("/app/")) web.loadUrl(KDCore.origin + "/app/");
            else moveTaskToBack(true); // keep the core (and sharing) running
        });
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        if (req == REQ_CAPTURE) {
            if (res == RESULT_OK && data != null) {
                KDShareService.start(this, res, data);
                toast("Sharing this phone's screen");
            } else toast("Screen sharing was not allowed");
            return;
        }
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

    class Client extends WebViewClient {
        @Override
        public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest r) {
            String u = r.getUrl().toString();
            if (KDCore.origin != null && u.startsWith(KDCore.origin + "/")) return false;
            if (u.startsWith("https://")) openExternal(u);
            return true;
        }
    }

    class Chrome extends WebChromeClient {
        @Override
        public boolean onShowFileChooser(WebView v, ValueCallback<Uri[]> cb, FileChooserParams p) {
            if (fileCb != null) fileCb.onReceiveValue(null);
            fileCb = cb;
            Intent i = new Intent(Intent.ACTION_GET_CONTENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*").putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
            try { startActivityForResult(Intent.createChooser(i, "Send to device"), REQ_FILE); } catch (Exception e) { fileCb = null; return false; }
            return true;
        }

        @Override
        public boolean onConsoleMessage(ConsoleMessage m) { Log.d(TAG, m.message() + " @" + m.sourceId() + ":" + m.lineNumber()); return true; }
    }

    void toast(String s) { ui.post(() -> Toast.makeText(this, s, Toast.LENGTH_LONG).show()); }

    static void openExternal(String u) {
        MainActivity a = visible;
        if (a == null || u == null || !u.startsWith("https://")) return;
        a.ui.post(() -> { try { a.startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(u))); } catch (Exception ignored) {} });
    }

    /** A connection request when "Ask me before anyone connects" is on. */
    static void ask(String id, String title, String msg) {
        MainActivity a = visible;
        if (a == null) { KDShareService.answer(id, false); return; }
        a.ui.post(() -> new AlertDialog.Builder(a).setTitle(title).setMessage(msg).setCancelable(false)
                .setPositiveButton("Allow", (d, w) -> KDShareService.answer(id, true))
                .setNegativeButton("Deny", (d, w) -> KDShareService.answer(id, false)).show());
    }

    void download(String url, String name, String disp) {
        new Thread(() -> {
            try {
                String fn = name;
                if (disp != null && disp.contains("filename*=UTF-8''")) fn = URLDecoder.decode(disp.substring(disp.indexOf("filename*=UTF-8''") + 17), "UTF-8");
                fn = fn.replaceAll("[\\\\/:*?\"<>|]", "_");
                HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
                String ck = CookieManager.getInstance().getCookie(url);
                if (ck != null) c.setRequestProperty("Cookie", ck);
                c.setConnectTimeout(8000);
                c.setReadTimeout(60000);
                InputStream in = c.getInputStream();
                OutputStream out;
                String where;
                if (Build.VERSION.SDK_INT >= 29) {
                    ContentValues v = new ContentValues();
                    v.put(MediaStore.Downloads.DISPLAY_NAME, fn);
                    v.put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS + "/KadduDesk");
                    Uri dst = getContentResolver().insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, v);
                    out = getContentResolver().openOutputStream(dst);
                    where = "Downloads/KadduDesk";
                } else {
                    File dir = new File(getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS), "KadduDesk");
                    dir.mkdirs();
                    out = new FileOutputStream(new File(dir, fn));
                    where = dir.getAbsolutePath();
                }
                byte[] buf = new byte[65536];
                int n;
                while ((n = in.read(buf)) > 0) out.write(buf, 0, n);
                out.close();
                in.close();
                toast("Saved " + fn + " to " + where);
            } catch (Exception e) {
                toast("Download failed: " + e.getMessage());
            }
        }).start();
    }

    class Bridge {
        @JavascriptInterface public void home() { ui.post(() -> { if (KDCore.origin != null) web.loadUrl(KDCore.origin + "/app/"); }); }
        @JavascriptInterface public void openUrl(String u) { openExternal(u); }
        @JavascriptInterface public String getClipboard() {
            FutureTask<String> t = new FutureTask<>(() -> {
                ClipboardManager cm = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
                if (cm == null || !cm.hasPrimaryClip() || cm.getPrimaryClip().getItemCount() == 0) return "";
                CharSequence cs = cm.getPrimaryClip().getItemAt(0).coerceToText(MainActivity.this);
                return cs == null ? "" : cs.toString();
            });
            ui.post(t);
            try { return t.get(1500, TimeUnit.MILLISECONDS); } catch (Exception e) { return ""; }
        }
        @JavascriptInterface public void setClipboard(String s) {
            ui.post(() -> { ClipboardManager cm = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE); if (cm != null) cm.setPrimaryClip(ClipData.newPlainText("KadduDesk", s)); });
        }
        @JavascriptInterface public void download(String url, String name) { MainActivity.this.download(url, name, null); }
        @JavascriptInterface public void keepAwake(boolean on) {
            ui.post(() -> { if (on) getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON); else getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON); });
        }
        @JavascriptInterface public void fullscreen(boolean on) { ui.post(() -> { immersive = on; applyImmersive(); }); }
        @JavascriptInterface public void vibrate(int ms) {
            Vibrator v = (Vibrator) getSystemService(VIBRATOR_SERVICE);
            if (v == null || !v.hasVibrator()) return;
            if (Build.VERSION.SDK_INT >= 26) v.vibrate(VibrationEffect.createOneShot(Math.max(1, ms), VibrationEffect.DEFAULT_AMPLITUDE)); else v.vibrate(ms);
        }
        @JavascriptInterface public String deviceName() { return KDCore.deviceName(); }
        @JavascriptInterface public void showKeyboard(boolean on) {
            ui.post(() -> {
                InputMethodManager imm = (InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
                if (imm == null) return;
                if (on) { web.requestFocus(); imm.showSoftInput(web, 0); } else imm.hideSoftInputFromWindow(web.getWindowToken(), 0);
            });
        }
        @JavascriptInterface public void startSharing() {
            ui.post(() -> {
                if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED)
                    requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 9);
                MediaProjectionManager mpm = (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
                startActivityForResult(mpm.createScreenCaptureIntent(), REQ_CAPTURE);
            });
        }
        @JavascriptInterface public void stopSharing() { ui.post(() -> KDShareService.stop(MainActivity.this)); }
        @JavascriptInterface public String sharingState() {
            try { return new JSONObject().put("sharing", KDShareService.running).put("accessibility", KDAccess.inst != null).toString(); } catch (Exception e) { return "{}"; }
        }
        @JavascriptInterface public void openAccessibility() {
            ui.post(() -> {
                toast("Find KadduDesk in the list and turn it on");
                startActivity(new Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            });
        }
    }

    @SuppressWarnings("unused") private static String lc(String s) { return s.toLowerCase(Locale.US); }
}
