package com.kaddudesk.app;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkAddress;
import android.net.LinkProperties;
import android.net.Network;
import android.os.Build;
import android.os.Environment;
import android.util.Log;

import java.io.BufferedReader;
import java.io.File;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.security.SecureRandom;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/** Starts and owns the bundled KadduDesk core (libkdcore.so, a native executable). */
final class KDCore {
    static final String TAG = "KadduDesk";
    interface Callback { void ok(String url); void fail(String why); }

    static Process proc;
    static OutputStream stdin; // held open: the core exits when the app goes away
    static volatile String url, origin;
    static int bridgePort;
    static String token;

    static synchronized void start(Context ctx, Callback cb) {
        if (url != null && proc != null && alive()) { cb.ok(url); return; }
        final Context c = ctx.getApplicationContext();
        new Thread(() -> {
            try {
                File bin = new File(c.getApplicationInfo().nativeLibraryDir, "libkdcore.so");
                File data = new File(c.getFilesDir(), "kd");
                data.mkdirs();
                File up = c.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS);
                if (up == null) up = new File(data, "received");
                bridgePort = freePort();
                byte[] r = new byte[16];
                new SecureRandom().nextBytes(r);
                StringBuilder sb = new StringBuilder();
                for (byte b : r) sb.append(String.format(Locale.US, "%02x", b));
                token = sb.toString();
                List<String> cmd = new ArrayList<>();
                cmd.add(bin.getAbsolutePath());
                cmd.add("-app"); cmd.add("-parentwatch");
                cmd.add("-datadir"); cmd.add(data.getAbsolutePath());
                cmd.add("-devname"); cmd.add(deviceName());
                cmd.add("-bridge"); cmd.add(String.valueOf(bridgePort));
                cmd.add("-dns"); cmd.add(dnsList(c));
                cmd.add("-uploaddir"); cmd.add(up.getAbsolutePath());
                cmd.add("-ips"); cmd.add(String.join(",", ips(c)));
                ProcessBuilder pb = new ProcessBuilder(cmd);
                pb.environment().put("KD_BRIDGE_TOKEN", token);
                pb.environment().put("HOME", data.getAbsolutePath());
                pb.environment().put("TMPDIR", c.getCacheDir().getAbsolutePath());
                pb.redirectErrorStream(true);
                pb.directory(data);
                proc = pb.start();
                stdin = proc.getOutputStream();
                BufferedReader in = new BufferedReader(new InputStreamReader(proc.getInputStream()));
                String line;
                boolean told = false;
                while ((line = in.readLine()) != null) {
                    if (!told && line.startsWith("KDAPP ")) {
                        url = line.substring(6).trim();
                        int i = url.indexOf("/", "http://".length());
                        origin = i > 0 ? url.substring(0, i) : url;
                        told = true;
                        cb.ok(url);
                    } else Log.d(TAG, "core: " + line);
                }
                url = null;
                if (!told) cb.fail("The KadduDesk core stopped while starting.");
            } catch (Exception e) {
                Log.e(TAG, "core start", e);
                cb.fail(e.getMessage());
            }
        }, "kd-core").start();
    }

    static boolean alive() {
        try { proc.exitValue(); return false; } catch (IllegalThreadStateException e) { return true; }
    }

    static int freePort() throws Exception {
        try (ServerSocket s = new ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"))) { return s.getLocalPort(); }
    }

    static String deviceName() {
        String m = Build.MANUFACTURER, model = Build.MODEL;
        if (model.toLowerCase(Locale.US).startsWith(m.toLowerCase(Locale.US))) return model;
        return m.substring(0, 1).toUpperCase(Locale.US) + m.substring(1) + " " + model;
    }

    static String dnsList(Context c) {
        StringBuilder sb = new StringBuilder();
        try {
            ConnectivityManager cm = (ConnectivityManager) c.getSystemService(Context.CONNECTIVITY_SERVICE);
            Network n = cm.getActiveNetwork();
            LinkProperties lp = n == null ? null : cm.getLinkProperties(n);
            if (lp != null) for (InetAddress a : lp.getDnsServers()) {
                if (sb.length() > 0) sb.append(',');
                String h = a.getHostAddress();
                if (h.contains(":")) h = "[" + h + "]:53";
                sb.append(h);
            }
        } catch (Exception ignored) {}
        return sb.toString();
    }

    /** This phone's IPv4 addresses (Android 11+ hides interfaces from native code). */
    static List<String> ips(Context c) {
        List<String> out = new ArrayList<>();
        try {
            ConnectivityManager cm = (ConnectivityManager) c.getSystemService(Context.CONNECTIVITY_SERVICE);
            for (Network n : cm.getAllNetworks()) {
                LinkProperties lp = cm.getLinkProperties(n);
                if (lp == null) continue;
                for (LinkAddress la : lp.getLinkAddresses()) {
                    InetAddress a = la.getAddress();
                    if (a.getAddress().length == 4 && !a.isLoopbackAddress()) out.add(a.getHostAddress());
                }
            }
        } catch (Exception ignored) {}
        return out;
    }
}
