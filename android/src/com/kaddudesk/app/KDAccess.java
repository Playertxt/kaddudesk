package com.kaddudesk.app;

import android.accessibilityservice.AccessibilityService;
import android.accessibilityservice.GestureDescription;
import android.graphics.Path;
import android.os.Build;
import android.os.Bundle;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;

/** Remote control for this phone: taps, swipes, scrolling, typing and system buttons. */
public class KDAccess extends AccessibilityService {
    static volatile KDAccess inst;

    private Path path;
    private long t0;
    private float sx, sy, lx, ly;

    @Override protected void onServiceConnected() { inst = this; }
    @Override public boolean onUnbind(android.content.Intent i) { inst = null; return super.onUnbind(i); }
    @Override public void onDestroy() { inst = null; super.onDestroy(); }
    @Override public void onAccessibilityEvent(AccessibilityEvent e) {}
    @Override public void onInterrupt() {}

    void down(float x, float y) {
        path = new Path();
        path.moveTo(x, y);
        sx = lx = x; sy = ly = y;
        t0 = System.currentTimeMillis();
    }

    void move(float x, float y) {
        if (path == null) return;
        if (Math.abs(x - lx) + Math.abs(y - ly) < 3) return;
        path.lineTo(x, y);
        lx = x; ly = y;
    }

    void up(float x, float y, int button) {
        if (path == null) { down(x, y); }
        long dur = Math.max(40, Math.min(5000, System.currentTimeMillis() - t0));
        Path p = path;
        if (button == 2) { // right click = long press
            p = new Path(); p.moveTo(sx, sy); dur = 700;
        } else if (Math.abs(x - sx) + Math.abs(y - sy) >= 3) {
            p.lineTo(x, y);
        }
        stroke(p, dur);
        path = null;
    }

    void scroll(float x, float y, float dx, float dy) {
        float k = 1.6f;
        float ex = x + Math.max(-900, Math.min(900, -dx * k));
        float ey = y + Math.max(-900, Math.min(900, -dy * k));
        Path p = new Path();
        p.moveTo(x, y);
        p.lineTo(ex, ey);
        stroke(p, 160);
    }

    private void stroke(Path p, long dur) {
        try {
            GestureDescription g = new GestureDescription.Builder().addStroke(new GestureDescription.StrokeDescription(p, 0, dur)).build();
            dispatchGesture(g, null, null);
        } catch (Exception ignored) {}
    }

    void global(String a) {
        int act;
        switch (a) {
            case "back": act = GLOBAL_ACTION_BACK; break;
            case "home": act = GLOBAL_ACTION_HOME; break;
            case "recents": act = GLOBAL_ACTION_RECENTS; break;
            case "notifications": act = GLOBAL_ACTION_NOTIFICATIONS; break;
            case "quicksettings": act = GLOBAL_ACTION_QUICK_SETTINGS; break;
            case "power": act = GLOBAL_ACTION_POWER_DIALOG; break;
            case "lock": if (Build.VERSION.SDK_INT >= 28) { act = 8; break; } else return;
            case "screenshot": if (Build.VERSION.SDK_INT >= 28) { act = 9; break; } else return;
            default: return;
        }
        performGlobalAction(act);
    }

    private AccessibilityNodeInfo focused() {
        AccessibilityNodeInfo n = findFocus(AccessibilityNodeInfo.FOCUS_INPUT);
        return (n != null && n.isEditable()) ? n : null;
    }

    private String currentText(AccessibilityNodeInfo n) {
        CharSequence t = n.getText();
        if (t == null) return "";
        if (Build.VERSION.SDK_INT >= 26 && n.isShowingHintText()) return "";
        return t.toString();
    }

    private void setText(AccessibilityNodeInfo n, String s) {
        Bundle b = new Bundle();
        b.putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, s);
        n.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, b);
        Bundle sel = new Bundle();
        sel.putInt(AccessibilityNodeInfo.ACTION_ARGUMENT_SELECTION_START_INT, s.length());
        sel.putInt(AccessibilityNodeInfo.ACTION_ARGUMENT_SELECTION_END_INT, s.length());
        n.performAction(AccessibilityNodeInfo.ACTION_SET_SELECTION, sel);
    }

    void text(String s) {
        AccessibilityNodeInfo n = focused();
        if (n != null) setText(n, currentText(n) + s);
    }

    void key(String k) {
        AccessibilityNodeInfo n = focused();
        switch (k) {
            case "Backspace":
                if (n != null) { String t = currentText(n); if (!t.isEmpty()) setText(n, t.substring(0, t.offsetByCodePoints(t.length(), -1))); }
                break;
            case "Enter": case "NumpadEnter":
                if (n != null) {
                    if (Build.VERSION.SDK_INT >= 30) n.performAction(0x01020036 /* ACTION_IME_ENTER */);
                    else setText(n, currentText(n) + "\n");
                }
                break;
            case "Tab":
                if (n != null) setText(n, currentText(n) + "\t");
                break;
        }
    }
}
