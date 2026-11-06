package com.kaddudesk.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.hardware.display.DisplayManager;
import android.hardware.display.VirtualDisplay;
import android.media.MediaCodec;
import android.media.MediaCodecInfo;
import android.media.MediaFormat;
import android.media.projection.MediaProjection;
import android.media.projection.MediaProjectionManager;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.IBinder;
import android.os.Looper;
import android.util.DisplayMetrics;
import android.util.Log;
import android.view.Display;
import android.view.Surface;
import android.view.WindowManager;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;

/**
 * Shares this phone's screen: MediaProjection → VirtualDisplay → hardware H.264 encoder → the
 * KadduDesk core over a loopback socket. Receives input from the core and performs it through
 * KDAccess. Runs as a foreground service so sharing continues while other apps are open.
 */
public class KDShareService extends Service {
    static final String TAG = "KadduDesk";
    static volatile boolean running;
    static volatile KDShareService self;

    private MediaProjection mp;
    private VirtualDisplay vd;
    private MediaCodec codec;
    private Surface surface;
    private volatile boolean encoding;
    private long curGen = -1;
    private int curW, curH, curFps, curKbps;
    private byte[] config;
    private Socket sock;
    private DataOutputStream out;
    private final Object wlock = new Object();
    private Handler handler;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private WifiManager.MulticastLock mlock;
    private DisplayManager.DisplayListener dl;

    @Override public IBinder onBind(Intent i) { return null; }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && "stop".equals(intent.getAction())) { stopSelf(); return START_NOT_STICKY; }
        startInForeground();
        if (mp == null && intent != null) {
            MediaProjectionManager mpm = (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
            Intent data = Build.VERSION.SDK_INT >= 33 ? intent.getParcelableExtra("data", Intent.class) : intent.getParcelableExtra("data");
            int code = intent.getIntExtra("code", 0);
            try { mp = mpm.getMediaProjection(code, data); } catch (Exception e) { Log.e(TAG, "projection", e); }
            if (mp == null) { stopSelf(); return START_NOT_STICKY; }
            HandlerThread ht = new HandlerThread("kd-share");
            ht.start();
            handler = new Handler(ht.getLooper());
            mp.registerCallback(new MediaProjection.Callback() {
                @Override public void onStop() { ui.post(KDShareService.this::stopSelf); }
            }, handler);
            running = true;
            self = this;
            WifiManager wm = (WifiManager) getApplicationContext().getSystemService(WIFI_SERVICE);
            if (wm != null) { mlock = wm.createMulticastLock("kd-share"); mlock.acquire(); }
            DisplayManager dm = (DisplayManager) getSystemService(DISPLAY_SERVICE);
            dl = new DisplayManager.DisplayListener() {
                @Override public void onDisplayAdded(int id) {}
                @Override public void onDisplayRemoved(int id) {}
                @Override public void onDisplayChanged(int id) { if (id == Display.DEFAULT_DISPLAY) sendScreen(); }
            };
            dm.registerDisplayListener(dl, handler);
            new Thread(this::bridgeLoop, "kd-bridge").start();
        }
        return START_NOT_STICKY;
    }

    private void startInForeground() {
        NotificationManager nm = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        if (Build.VERSION.SDK_INT >= 26) nm.createNotificationChannel(new NotificationChannel("share", "Screen sharing", NotificationManager.IMPORTANCE_LOW));
        Intent open = new Intent(this, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent pi = PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE);
        PendingIntent stop = PendingIntent.getService(this, 1, new Intent(this, KDShareService.class).setAction("stop"), PendingIntent.FLAG_IMMUTABLE);
        Notification.Builder b = Build.VERSION.SDK_INT >= 26 ? new Notification.Builder(this, "share") : new Notification.Builder(this);
        Notification n = b.setSmallIcon(android.R.drawable.ic_menu_view).setContentTitle("KadduDesk is sharing this phone")
                .setContentText("Other devices can see this screen. Tap to open.").setContentIntent(pi).setOngoing(true)
                .addAction(new Notification.Action.Builder(null, "Stop sharing", stop).build()).build();
        if (Build.VERSION.SDK_INT >= 29) startForeground(1, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION);
        else startForeground(1, n);
    }

    @Override
    public void onDestroy() {
        running = false;
        self = null;
        stopEncoder();
        if (vd != null) { vd.release(); vd = null; }
        if (mp != null) { try { mp.stop(); } catch (Exception ignored) {} mp = null; }
        if (dl != null) ((DisplayManager) getSystemService(DISPLAY_SERVICE)).unregisterDisplayListener(dl);
        if (mlock != null && mlock.isHeld()) mlock.release();
        try { if (sock != null) sock.close(); } catch (Exception ignored) {}
        sendStateQuiet(false);
        super.onDestroy();
    }

    // ------------------------------------------------------------ bridge to the core

    private void bridgeLoop() {
        while (running) {
            try {
                Socket s = new Socket();
                s.setTcpNoDelay(true);
                s.connect(new InetSocketAddress("127.0.0.1", KDCore.bridgePort), 3000);
                sock = s;
                out = new DataOutputStream(new BufferedOutputStream(s.getOutputStream(), 1 << 16));
                sendJson(new JSONObject().put("t", "hello").put("token", KDCore.token));
                sendScreen();
                sendState();
                JSONArray ips = new JSONArray();
                for (String ip : KDCore.ips(this)) ips.put(ip);
                sendJson(new JSONObject().put("t", "net").put("ips", ips));
                ui.postDelayed(this::stateTick, 3000);
                DataInputStream in = new DataInputStream(new BufferedInputStream(s.getInputStream()));
                while (running) {
                    int len = in.readInt();
                    int type = in.readUnsignedByte();
                    byte[] b = new byte[len];
                    in.readFully(b);
                    if (type == 3) handle(new JSONObject(new String(b, StandardCharsets.UTF_8)));
                }
            } catch (Exception e) {
                if (running) Log.w(TAG, "bridge: " + e);
            }
            try { Thread.sleep(1000); } catch (InterruptedException ignored) {}
        }
    }

    private void stateTick() {
        if (!running) return;
        sendState();
        ui.postDelayed(this::stateTick, 3000);
    }

    private void sendScreen() {
        DisplayMetrics m = new DisplayMetrics();
        ((WindowManager) getSystemService(WINDOW_SERVICE)).getDefaultDisplay().getRealMetrics(m);
        try { sendJson(new JSONObject().put("t", "screen").put("w", m.widthPixels).put("h", m.heightPixels)); } catch (Exception ignored) {}
    }

    private void sendState() { sendStateQuiet(running); }

    private void sendStateQuiet(boolean on) {
        try { sendJson(new JSONObject().put("t", "state").put("sharing", on).put("access", KDAccess.inst != null)); } catch (Exception ignored) {}
    }

    private void sendJson(JSONObject o) {
        byte[] b = o.toString().getBytes(StandardCharsets.UTF_8);
        synchronized (wlock) {
            try {
                if (out == null) return;
                out.writeInt(b.length);
                out.writeByte(3);
                out.write(b);
                out.flush();
            } catch (Exception ignored) {}
        }
    }

    private void sendFrame(boolean key, byte[] data, int len) {
        synchronized (wlock) {
            try {
                if (out == null) return;
                out.writeInt(len + 1);
                out.writeByte(1);
                out.writeByte(key ? 1 : 0);
                out.write(data, 0, len);
                out.flush();
            } catch (Exception ignored) {}
        }
    }

    private void handle(JSONObject m) {
        String t = m.optString("t");
        KDAccess a = KDAccess.inst;
        switch (t) {
            case "vcfg": {
                long g = m.optLong("gen");
                int w = m.optInt("w"), h = m.optInt("h"), fps = m.optInt("fps", 30), kbps = m.optInt("kbps", 8000);
                handler.post(() -> {
                    curGen = g;
                    if (codec != null && encoding && w == curW && h == curH && fps == curFps && kbps == curKbps) requestSync();
                    else startEncoder(w, h, fps, kbps);
                });
                break;
            }
            case "vstop": {
                long g = m.optLong("gen");
                handler.post(() -> { if (g == curGen) stopEncoder(); });
                break;
            }
            case "down": case "move": case "up": case "scroll": case "key": case "text": case "global":
                if (a == null) { sendNoAccess(); break; }
                float x = (float) m.optDouble("x"), y = (float) m.optDouble("y");
                ui.post(() -> {
                    switch (t) {
                        case "down": a.down(x, y); break;
                        case "move": a.move(x, y); break;
                        case "up": a.up(x, y, m.optInt("b")); break;
                        case "scroll": a.scroll(x, y, (float) m.optDouble("dx"), (float) m.optDouble("dy")); break;
                        case "key": a.key(m.optString("k")); break;
                        case "text": a.text(m.optString("s")); break;
                        case "global": a.global(m.optString("a")); break;
                    }
                });
                break;
            case "clip":
                ui.post(() -> {
                    ClipboardManager cm = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
                    if (cm != null) cm.setPrimaryClip(ClipData.newPlainText("KadduDesk", m.optString("s")));
                });
                break;
            case "ask":
                MainActivity.ask(m.optString("id"), m.optString("title"), m.optString("msg"));
                break;
            case "open":
                MainActivity.openExternal(m.optString("url"));
                break;
        }
    }

    private long lastNoAccess;
    private void sendNoAccess() {
        if (System.currentTimeMillis() - lastNoAccess < 5000) return;
        lastNoAccess = System.currentTimeMillis();
        try { sendJson(new JSONObject().put("t", "noaccess")); } catch (Exception ignored) {}
    }

    static void answer(String id, boolean ok) {
        KDShareService s = self;
        if (s != null) try { s.sendJson(new JSONObject().put("t", "answer").put("id", id).put("ok", ok)); } catch (Exception ignored) {}
    }

    // ------------------------------------------------------------ encoder

    private void requestSync() {
        try { Bundle b = new Bundle(); b.putInt(MediaCodec.PARAMETER_KEY_REQUEST_SYNC_FRAME, 0); codec.setParameters(b); } catch (Exception ignored) {}
    }

    private void startEncoder(int w, int h, int fps, int kbps) {
        stopEncoder();
        w = Math.max(160, w & ~15);
        h = Math.max(160, h & ~15);
        try {
            MediaFormat f = MediaFormat.createVideoFormat(MediaFormat.MIMETYPE_VIDEO_AVC, w, h);
            f.setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatSurface);
            f.setInteger(MediaFormat.KEY_BIT_RATE, kbps * 1000);
            f.setInteger(MediaFormat.KEY_FRAME_RATE, fps);
            f.setInteger(MediaFormat.KEY_I_FRAME_INTERVAL, 10);
            f.setLong(MediaFormat.KEY_REPEAT_PREVIOUS_FRAME_AFTER, 1_000_000L / Math.max(5, fps));
            f.setInteger(MediaFormat.KEY_PRIORITY, 0);
            if (Build.VERSION.SDK_INT >= 30) f.setInteger(MediaFormat.KEY_LATENCY, 0);
            if (Build.VERSION.SDK_INT >= 29) f.setInteger(MediaFormat.KEY_MAX_B_FRAMES, 0);
            MediaCodec c = MediaCodec.createEncoderByType(MediaFormat.MIMETYPE_VIDEO_AVC);
            c.configure(f, null, null, MediaCodec.CONFIGURE_FLAG_ENCODE);
            Surface s = c.createInputSurface();
            c.start();
            DisplayMetrics m = new DisplayMetrics();
            ((WindowManager) getSystemService(WINDOW_SERVICE)).getDefaultDisplay().getRealMetrics(m);
            if (vd == null) {
                vd = mp.createVirtualDisplay("KadduDesk", w, h, m.densityDpi, DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, s, null, handler);
            } else {
                vd.resize(w, h, m.densityDpi);
                vd.setSurface(s);
            }
            codec = c; surface = s; encoding = true; config = null;
            curW = w; curH = h; curFps = fps; curKbps = kbps;
            final MediaCodec cc = c;
            new Thread(() -> drain(cc), "kd-enc").start();
            Log.i(TAG, "encoder " + w + "x" + h + " @" + fps + " " + kbps + "kbps");
        } catch (Exception e) {
            Log.e(TAG, "encoder", e);
        }
    }

    private void drain(MediaCodec c) {
        MediaCodec.BufferInfo info = new MediaCodec.BufferInfo();
        byte[] buf = new byte[1 << 20];
        while (encoding && codec == c) {
            int idx;
            try { idx = c.dequeueOutputBuffer(info, 20000); } catch (Exception e) { break; }
            if (idx < 0) continue;
            try {
                ByteBuffer bb = c.getOutputBuffer(idx);
                if (bb != null && info.size > 0) {
                    bb.position(info.offset);
                    bb.limit(info.offset + info.size);
                    if ((info.flags & MediaCodec.BUFFER_FLAG_CODEC_CONFIG) != 0) {
                        config = new byte[info.size];
                        bb.get(config);
                    } else {
                        boolean key = (info.flags & MediaCodec.BUFFER_FLAG_KEY_FRAME) != 0;
                        int pre = key && config != null ? config.length : 0;
                        int need = pre + info.size;
                        if (buf.length < need) buf = new byte[need * 2];
                        if (pre > 0) System.arraycopy(config, 0, buf, 0, pre);
                        bb.get(buf, pre, info.size);
                        sendFrame(key, buf, need);
                    }
                }
                c.releaseOutputBuffer(idx, false);
            } catch (Exception e) { break; }
        }
    }

    private void stopEncoder() {
        encoding = false;
        if (vd != null) { try { vd.setSurface(null); } catch (Exception ignored) {} }
        if (codec != null) {
            try { codec.stop(); } catch (Exception ignored) {}
            try { codec.release(); } catch (Exception ignored) {}
            codec = null;
        }
        if (surface != null) { surface.release(); surface = null; }
    }

    static void start(Context c, int code, Intent data) {
        Intent i = new Intent(c, KDShareService.class).putExtra("code", code).putExtra("data", data);
        if (Build.VERSION.SDK_INT >= 26) c.startForegroundService(i); else c.startService(i);
    }

    static void stop(Context c) {
        c.startService(new Intent(c, KDShareService.class).setAction("stop"));
    }
}
