# KadduDesk ⚡

**Fast, low-latency remote desktop for Windows and Linux.** Control your PC from an Android phone, a Windows or Linux PC, or any modern browser. It works like Chrome Remote Desktop, but it encodes the screen on your GPU (NVENC / AMF / Quick Sync) and decodes it in hardware on the client. The result is noticeably snappier.

> *Kaddu* means pumpkin, hence the orange.

---

## Table of contents
1. [What you get](#1-what-you-get)
2. [Quick start (5 minutes)](#2-quick-start-5-minutes)
3. [Installing the server on your PC](#3-installing-the-server-on-your-pc)
3b. [Installing the server on Linux](#3b-installing-the-server-on-linux)
4. [Connecting from Android](#4-connecting-from-android)
5. [Connecting from another Windows PC](#5-connecting-from-another-windows-pc)
5b. [Connecting from Linux](#5b-connecting-from-linux)
6. [Connecting from a browser](#6-connecting-from-a-browser)
7. [Service mode: lock screen, UAC, Ctrl+Alt+Del](#7-service-mode-lock-screen-uac-ctrlaltdel)
7b. [Hub: control any device from any device (AnyDesk-style)](#7b-hub-control-any-device-from-any-device-anydesk-style)
8. [Using it from outside your home](#8-using-it-from-outside-your-home)
9. [Wake-on-LAN](#9-wake-on-lan)
10. [Using a session: controls, gestures, shortcuts](#10-using-a-session-controls-gestures-shortcuts)
11. [The control panel (server settings)](#11-the-control-panel-server-settings)
12. [Security model](#12-security-model)
13. [Troubleshooting](#13-troubleshooting)
14. [Building from source](#14-building-from-source)
15. [How it works](#15-how-it-works)
16. [Project layout](#16-project-layout)
17. [Known limitations / roadmap](#17-known-limitations--roadmap)

---

## 1. What you get

| Build output (`dist/`) | Runs on | Purpose |
|---|---|---|
| `KadduDeskServer.exe` | The PC you want to control (Windows 10/11, 64-bit) | Host server. Single file with ffmpeg embedded. Asks for admin rights. Lives in the system tray. |
| `KadduDesk.apk` | Android 7.0+ | Phone/tablet client. |
| `KadduDesk-1.0.0-setup.exe` | Windows 10/11 | Desktop client, installer. |
| `KadduDesk-1.0.0-portable.exe` | Windows 10/11 | Desktop client, no install. |
| `kaddudesk-server-linux-x64.tar.gz` / `-arm64` | Linux PC / Raspberry Pi to control | Host server + installer script. |
| `kaddudesk-client-linux-x64.tar.gz` | Linux desktop | Desktop client + installer script. |
| any browser | Chrome / Edge / Samsung Internet | Zero-install client at `https://<pc-ip>:47800/`. |

**Feature list**

- Remote view & full control: mouse, keyboard (scancodes, so every key works), scroll, all 5 mouse buttons
- Multi-monitor switching
- Two-way clipboard sync
- File transfer both ways: send files to the PC, browse and download from the PC
- PC sound streaming (Opus, 10 ms frames)
- Ctrl+Alt+Del and a shortcuts menu (Task Manager, Alt+Tab, Win+D, snip, media and volume keys …)
- Fullscreen with keyboard lock: Win, Alt+Tab and Esc go to the remote PC
- PIN login, remembered devices, per-device revoke, kick, pause sharing, optional "ask me first" prompt
- **Service mode**: control the lock screen, sign-in screen and UAC prompts; works before anyone signs in
- **Hub (AnyDesk-style)**: your main PC relays for all your devices; 9-digit device IDs; control any of them from anywhere; one port to forward
- **Phone-first touch**: trackpad mode or direct-touch mode, pinch-zoom, sticky Ctrl/Alt/Shift/Win key bar, haptics
- LAN auto-discovery (PCs just appear), Wake-on-LAN, power menu (lock / sleep / restart / shut down / sign out)
- Live stats overlay (latency, ping, decode time, fps, bitrate), Auto quality, game mode (pointer lock)
- TLS encryption with certificate pinning, brute-force lockout, localhost-only admin panel

---

## 2. Quick start (5 minutes)

1. **On the PC:** double-click `KadduDeskServer.exe` and click **Yes** on the Windows admin prompt. A control panel opens in your browser and shows a **PIN**.
2. **On the phone:** install `KadduDesk.apk` and open **KadduDesk**. Your PC appears under **Nearby on this network** (phone and PC must be on the same Wi-Fi).
3. Tap **Connect**. Check the fingerprint matches the one in the control panel, then tap **Trust & connect**.
4. Type the PIN, keep **Remember this device** ticked, and tap **Connect**. You're in.

---

## 3. Installing the server on your PC

1. Copy `KadduDeskServer.exe` anywhere, for example `C:\Tools\KadduDesk\`.
2. Run it and accept the UAC (administrator) prompt.
   - *Why admin?* Without it Windows blocks input into elevated windows (Task Manager, installers, admin terminals).
   - Windows SmartScreen may warn because the exe isn't code-signed. Click **More info → Run anyway**.
3. On first run it:
   - creates a private TLS certificate and a random 6-digit PIN in `%APPDATA%\KadduDesk\`
   - adds a Windows Firewall rule for **private** networks
   - detects the best video encoder (NVIDIA → AMD → Intel → CPU)
   - opens the control panel and puts an orange icon in the tray (right-click it for the menu)
4. **Make it start automatically**, using either option in the control panel's **Server** card:
   - **Service mode** *(recommended; see [§7](#7-service-mode-lock-screen-uac-ctrlaltdel))*, or
   - **Start with Windows**: a Task Scheduler entry that runs it elevated at logon without a UAC prompt.

Your Wi-Fi must be a **Private** network in Windows (Settings → Network & internet → Wi-Fi → *your network* → Network profile type: **Private**). Otherwise the firewall blocks phones. The control panel has an *Allow on public networks* button if you really need it.

## 3b. Installing the server on Linux

Works on Ubuntu/Debian, Fedora, Arch, openSUSE and others (x64 and ARM64, e.g. Raspberry Pi 4/5).

1. Download `kaddudesk-server-linux-x64.tar.gz` (or `-arm64`) and unpack it:
   ```bash
   tar xzf kaddudesk-server-linux-x64.tar.gz && cd kaddudesk-server
   ```
2. Run the installer once:
   ```bash
   sudo ./install-linux.sh
   ```
   It:
   - installs `ffmpeg`, `xclip`/`wl-clipboard`, `zenity`, `xdotool`
   - copies the server to `/opt/kaddudesk`
   - lets your desktop user create virtual mouse/keyboard devices (a `uinput` udev rule)
   - opens ports 47800/tcp and 47810/udp in `ufw`/`firewalld`
   - adds a menu entry and start-at-login
   - optionally sets up **KMS capture** for Wayland (see below)
3. **Log out and back in once** so the input permission applies.
4. Start **KadduDesk Server** from your app menu, or run `kaddudesk-server`. The control panel opens in your browser with the PIN, the same as on Windows.

**X11 vs Wayland**

| Session | Screen capture | Notes |
|---|---|---|
| X11 / Xorg | `x11grab`, any GPU (NVIDIA NVENC, Intel/AMD VAAPI, or CPU) | Works out of the box, including the lock screen. |
| Wayland (Ubuntu 22.04+ / Fedora default) | **KMS capture** + VAAPI | Answer **y** to "Enable KMS capture?" in the installer. Intel/AMD only. |
| Wayland + NVIDIA | — | Pick **"Ubuntu on Xorg"** (gear icon on the login screen). |

What's different from Windows:
- Mouse and keyboard go through `/dev/uinput`, so they work on X11 and Wayland.
- Characters outside US-ASCII are typed with `xdotool` (X11) or `wtype` (Wayland) if installed.
- Sound is the PulseAudio/PipeWire monitor of your default output.
- Clipboard sync needs `wl-clipboard` (Wayland) or `xclip` (X11).
- The power menu uses `loginctl`/`systemctl`.
- *Ask before connecting* needs `zenity` or `kdialog`.
- Service mode and Ctrl+Alt+Del are Windows-only.

Uninstall with:
```bash
sudo ./install-linux.sh --uninstall
```

---

## 4. Connecting from Android

1. Copy `KadduDesk.apk` to the phone (USB, Google Drive, Telegram to yourself …) and open it.
2. Allow **Install unknown apps** for the app you opened it with, then tap **Install**. If Play Protect asks, choose *Install anyway*; the app is just unsigned by Google.
3. Open **KadduDesk**:
   - **Nearby on this network** lists PCs running the server. Tap **Connect**.
   - Or type the PC's IP (shown in the control panel) under **Add a PC by address**.
4. **First connection:** compare the fingerprint with the PC's control panel and tap **Trust & connect**. The app remembers this, and will warn you loudly if the PC's identity ever changes.
5. Enter the PIN. With *Remember this device*, you won't be asked again.
6. Saved PCs stay under **My PCs**, with a green dot when online, **⏰ Wake** (Wake-on-LAN) and **✕** (remove).

The Android back button closes panels and the keyboard first, then opens the menu. Use **Disconnect** in the menu to leave.

---

## 5. Connecting from another Windows PC

- Install `KadduDesk-1.0.0-setup.exe`, or just run `KadduDesk-1.0.0-portable.exe`.
- Same flow as Android: discovered PCs show up automatically, or add one by IP. Then trust, then PIN.
- Tips:
  - Your local mouse pointer **is** the remote pointer, with the shape synced live, so movement has zero lag.
  - **Fullscreen** (menu → ⛶) also captures Win, Alt+Tab and Esc for the remote PC.
  - Drag & drop files onto the window to send them to the PC.
  - Open the menu with the tab at the top centre, or **Ctrl+Alt+Shift+M**.

## 5b. Connecting from Linux

1. Download `kaddudesk-client-linux-x64.tar.gz` and unpack it:
   ```bash
   tar xzf kaddudesk-client-linux-x64.tar.gz && cd kaddudesk-client-linux-x64
   ```
2. Install it to the app menu (recommended; this also sets up Chromium's sandbox properly):
   ```bash
   sudo ./install-client.sh
   ```
   Or run it without installing:
   ```bash
   ./kaddudesk-client
   ```
3. It works the same as the Windows client: discovered PCs appear automatically, you trust the PC once, then enter the PIN.

---

## 6. Connecting from a browser

Works in Chrome, Edge and Samsung Internet (needs WebCodecs). Firefox and Safari aren't supported yet.

1. Open `https://<pc-ip>:47800/`, for example `https://192.168.1.104:47800/`, or scan the QR code in the control panel.
2. The browser warns about the certificate: it's self-signed by your own PC. Choose **Advanced → Proceed**. Check that the fingerprint shown on the login card matches the control panel.
3. Enter the PIN.

---

## 7. Service mode: lock screen, UAC, Ctrl+Alt+Del

Normal mode runs as *you*. Windows hides the **lock screen**, the **sign-in screen** and **UAC prompts** ("secure desktop") from normal programs, so in normal mode they appear as a message to the viewer.

**Service mode** fixes that the same way Chrome Remote Desktop does:

- A Windows service (**KadduDesk Remote Desktop**) runs as SYSTEM and starts with Windows, before anyone signs in.
- It launches a SYSTEM *agent* in the active console session, which captures and controls whichever desktop is active: normal, lock screen, or UAC.
- **Ctrl+Alt+Del** sends the real secure-attention sequence.
- The agent restarts automatically after sign-out, user switch or crashes.

**Turn it on:** Control panel → **Server** → **Service mode** → confirm. KadduDesk then:
1. copies itself to `C:\Program Files\KadduDesk\` (an admin-only folder, so the binary a SYSTEM service runs can't be tampered with)
2. moves settings to `C:\ProgramData\KadduDesk\`, readable only by SYSTEM and Administrators, keeping your **PIN, remembered devices and certificate** so phones don't need re-pairing
3. sets the `SoftwareSASGeneration` policy so a service may send Ctrl+Alt+Del
4. registers and starts the service. Connected devices reconnect within a few seconds.

**Turn it off:** the same switch. The service is removed; run `KadduDeskServer.exe` again for normal mode.

Command line alternative (from an admin terminal):
```bash
KadduDeskServer.exe -install-service
```
```bash
KadduDeskServer.exe -uninstall-service
```

Apps opened from the power menu (Task Manager, Explorer, Terminal, Settings) start as the signed-in user, not as SYSTEM.

## 7b. Hub: control any device from any device (AnyDesk-style)

Make **your main PC the hub**. Every other computer (Windows or Linux) *joins* it and keeps an outbound connection to it, so it's reachable from anywhere, even behind another router or on mobile data. Each device gets a 9-digit **device ID** like AnyDesk. Any phone or computer that opens the hub sees all your devices and can control any of them. **All traffic is relayed through the main PC**, so the load sits there.

```
  phone / laptop / browser ──► HUB (main PC, port 47820) ◄── PC #2 (joined, outbound)
                                     ▲          ▲
                                     │          └──── Linux box (joined, outbound)
                                     └─ the main PC itself is a device too
```

**1. On the main PC (the hub)**
1. Open the control panel → **Hub** card → turn on **Be the hub**.
2. Note the **Hub password** (for people) and the **Host key** (for PCs that join). Click *Show*; you can change both.
3. For access from outside your home, forward **TCP 47820** on your router to the main PC, or use Tailscale. That's the only port you need; other PCs don't need any port forwarding.

**2. On every other PC**
1. Install and run KadduDesk Server (Windows `.exe`, or the Linux package).
2. Control panel → **Join a hub** → enter the hub address (e.g. `192.168.1.10`, `my-home.example.com` or a Tailscale IP; port 47820 is the default) and the **host key** → **Join hub**.
3. Compare the fingerprint it shows with the main PC's certificate fingerprint and confirm. The card turns **connected**, and the PC shows up on the hub with its ID.

**3. Connect from anywhere**
- In the **KadduDesk app** (Android / Windows / Linux), the hub appears under *Nearby* as **🌐 … · Hub** on the same network. You can also add it by address with port **47820**. Tap **Open hub**, enter the hub password once, then pick a device.
- In a **browser**: open `https://<hub-address>:47820/`.
- Each device still asks for **its own PIN** the first time (with *Remember this device* you won't be asked again). Knowing the hub password alone doesn't give control.
- **⏰ Wake** on an offline device sends Wake-on-LAN *from the hub's network*, so you can wake a sleeping home PC from your phone on mobile data.

**Security**
- Device ↔ hub uses TLS with the hub's certificate **pinned** on each device. Devices need the host key to join; regenerating it disconnects every device until it re-joins.
- Client ↔ hub uses TLS with the hub pinned in the apps, a hub password (brute-force limited) and an HttpOnly cookie. Then the device's own PIN or remembered-device token is checked on the device itself.
- The hub sees relayed traffic (it terminates TLS), so run it on a PC you trust: your own main PC.

---

## 8. Using it from outside your home

KadduDesk connects directly to your PC; there's no cloud relay. Choose one:

**A. Tailscale (easiest, most secure)**
1. Install [Tailscale](https://tailscale.com/download) on the PC and on your phone/laptop, and sign in with the same account on both.
2. In the KadduDesk app, add the PC by its **Tailscale IP** (`100.x.y.z`, shown in the Tailscale app). It also appears in the control panel's address list.
3. Works from anywhere: mobile data, other Wi-Fi networks.

**B. Router port forwarding**
1. Give the PC a fixed LAN IP (DHCP reservation in your router).
2. Forward **TCP 47800** → that IP, port 47800.
3. Connect to your public IP (or a dynamic-DNS name). Everything is TLS-encrypted, PIN-protected and brute-force-limited. Still, consider turning on *Ask me before anyone connects*.
4. If your ISP uses CGNAT (no real public IP), port forwarding won't work. Use Tailscale.

---

## 9. Wake-on-LAN

The app learns the PC's network adapter address the first time you connect. **⏰ Wake** in *My PCs* sends the magic packet.

To make it work:
1. **BIOS/UEFI:** enable *Wake on LAN* / *Power On by PCI-E*.
2. **Windows:** Device Manager → your network adapter → **Power Management**: tick *Allow this device to wake the computer* and *Only allow a magic packet*. On the **Advanced** tab, set *Wake on Magic Packet* = Enabled.
3. Wired Ethernet is most reliable. Wi-Fi wake only works from sleep on some adapters.
4. From outside your home, Wake-on-LAN needs a router that forwards UDP 9 to the broadcast address, or a Tailscale subnet router.

---

## 10. Using a session: controls, gestures, shortcuts

### Opening the menu
- **Phone:** tap the floating orange bubble. Drag it to any edge. Double-tap it to reset zoom.
- **PC:** click the tab at the top centre, or press **Ctrl+Alt+Shift+M**.

Quick buttons: **Keyboard**, **Sound**, **Fullscreen**, **Stats**, **Ctrl+Alt+Del**, **Disconnect**.

### Touch gestures

| Gesture | Trackpad mode (default) | Direct touch mode |
|---|---|---|
| One finger drag | Move pointer (with acceleration) | Click-and-drag at that spot |
| Tap | Left click | Click where you tap |
| Double-tap & drag / long-press | Drag (hold left button) | Long-press = right click |
| Two-finger tap | Right click | Right click |
| Two-finger drag | Scroll (natural by default) | Scroll |
| Pinch | Zoom the view (pointer is followed) | Zoom the view |
| Three-finger tap | Show/hide keyboard | Show/hide keyboard |

Switch mode and pointer speed under **Menu → Input**.

### Keyboard on the phone
**Keyboard** opens the phone keyboard plus a key bar: **Ctrl Alt Shift Win** (sticky; tap, then press a letter: Ctrl then C = copy), Esc, Tab, arrows, ⌫, Del, ⏎, Home/End/PgUp/PgDn, F1–F12, PrtSc. Double-tap **Win** to press Win by itself.

### Shortcuts tab
One-tap: Ctrl+Alt+Del, Task Manager, Start, Alt+Tab, Alt+F4, Show desktop, Explorer, Run, Lock, Task view, Snip, Copy/Paste/Cut/Undo/Select all/Save, volume and media keys. There's also **Type text on the PC** for pasting long text.

### Display tab
- **Screen**: pick the monitor.
- **Quality**:
  - *Auto*: adapts bitrate to your network.
  - *Fastest*: 720p, 8 Mbps.
  - *Balanced*: 20 Mbps.
  - *Sharpest*: native resolution, 45 Mbps.
  - *Custom*: 15–144 fps, 1–100 Mbps, resolution.
- **View**: Fit / Fill / 1:1 pixels.

### Other tabs
- **Files:** **Send files to PC** saves to `Downloads\KadduDesk` on the PC. Browse the PC's drives and tap a file to download it (on Android it lands in `Downloads/KadduDesk`).
- **Clipboard:** auto-sync both ways, or send/copy manually.
- **Power:** Lock, Sleep, Restart, Shut down, Sign out, plus quick launch of Task Manager, Explorer, Terminal and Settings. Destructive actions ask first.
- **Input → Game mode:** locks the mouse to the window and sends raw relative movement (for games). Press Esc to release.

### Stats overlay
Shows estimated end-to-end latency, ping, network queueing, decode time, fps, bitrate and encoder. The status dot in the top tab is green/amber/red by latency.

---

## 11. The control panel (server settings)

Open it from the tray icon (**Open control panel**), or by running `KadduDeskServer.exe` again. It only works on the PC itself (`127.0.0.1:47801`).

| Section | What's there |
|---|---|
| **Connect a device** | PIN (show / new / set your own), addresses, QR code, certificate fingerprint |
| **Live** | viewers, fps, bitrate, encoder, connected devices (disconnect), remembered devices (forget) |
| **Permissions** | ask before connecting · allow control (off = view-only) · files · clipboard · sound · power actions |
| **Server** | Service mode · start with Windows · PC name · default fps/bitrate · encoder · received-files folder · port · firewall buttons |
| **Log** | recent server log (also in `server.log` in the data folder) |

The tray menu has: Pause sharing, Disconnect everyone, Quit.

---

## 12. Security model

- **Encryption:** all traffic (video, input, files) is TLS 1.2+ over a certificate generated on your PC.
- **Identity:** the apps **pin** the PC's certificate fingerprint on first connect (trust-on-first-use) and refuse to connect if it changes, which detects man-in-the-middle attacks.
- **Login:** PIN, stored as a salted PBKDF2 hash (120k iterations). After 5 wrong attempts the IP is locked out for 30 s, doubling up to 32 min.
- **Tokens:** remembered devices get a random 256-bit token, stored hashed. Revoke any device in the control panel.
- **Admin panel:** bound to 127.0.0.1 only. It requires a per-install secret key and checks the Host header, which blocks CSRF and DNS-rebinding from websites.
- **Service mode:** binary in Program Files and data in ProgramData, both admin-only. User-facing apps launch as the signed-in user.
- **Ask before connecting:** optional Yes/No prompt on the PC for every login.

Anyone with the PIN can fully control the PC, including files and power. Choose a strong PIN if you expose the port to the internet.

---

## 13. Troubleshooting

| Problem | Fix |
|---|---|
| Phone doesn't see the PC under *Nearby* | Same Wi-Fi? Network set to **Private** on the PC? Some routers block device-to-device traffic ("AP/client isolation", common on guest networks). Try adding by IP. |
| "Can't reach …" when connecting | Is the server running (tray icon)? Click **Re-add firewall rule** in the control panel. Check the port (default 47800). |
| Black screen / "Connecting…" forever | Control panel → **Live** shows the encoder and the last ffmpeg message. Click **Re-detect encoder & screens**. Update your GPU driver. |
| Message about lock screen / UAC prompt | Normal mode can't see secure desktops. Turn on **Service mode** (§7). |
| Laggy | Menu → **Stats**. High *net queue* means Wi-Fi congestion: use 5 GHz, the *Auto* or *Fastest* preset, or a lower resolution. High *decode* means the device is slow: lower the resolution. |
| No sound | Menu → **Sound** on; control panel → Permissions → *Stream PC sound*. Sound comes from the PC's **default** playback device. |
| "Certificate changed" warning | Expected only if you reinstalled or reset KadduDesk on the PC (new certificate). Otherwise **don't** trust it. |
| Keys type the wrong characters | Keys are sent as physical positions; the PC's keyboard layout decides the character. Use *Type text on the PC* for exact text. |
| Forgot the PIN | It's shown in the control panel on the PC (**Show**). |
| Wrong address shown first | The panel lists every adapter (Wi-Fi, VPN, VirtualBox). Use the one on the same network as your phone. |

Files and logs:
- Normal mode: `%APPDATA%\KadduDesk\` (`server.json`, `cert.pem`, `key.pem`, `server.log`)
- Service mode: `C:\ProgramData\KadduDesk\` (plus `service.log`)
- Extracted ffmpeg: `%LOCALAPPDATA%\KadduDesk\`

---

## 14. Building from source

### Prerequisites (Windows)
| Tool | For | Notes |
|---|---|---|
| [Go](https://go.dev/dl/) 1.24+ | server | |
| [Node.js](https://nodejs.org/) 20+ | Windows client | `npm install` once |
| JDK 17+ | Android | e.g. Oracle JDK or Android Studio's JBR |
| Android SDK | Android | build-tools 34+ and platform 34+ (from Android Studio's SDK Manager). Gradle isn't needed. |
| Git Bash | build script | ships with Git for Windows |
| Python 3 (+ Pillow) | icons, packaging helpers | `pip install pillow` (only to regenerate icons) |

### Build
```bash
git clone https://github.com/Playertxt/kaddudesk.git
```
```bash
cd kaddudesk && npm install
```
```bash
./build.sh all
```

Targets: `./build.sh server | linux | android | windows | linux-client | all`. Output goes to `dist/`.

- **Linux server:** pure-Go cross-compile (`CGO_ENABLED=0`) for x64 and arm64, packaged with `linux/install-linux.sh`. On Linux it uses the system ffmpeg instead of an embedded one.
- **Linux client:** electron-builder `dir` target, packaged as a tarball with `linux/client/` scripts. AppImage needs symlink rights, so it isn't built on Windows. To get an AppImage, run `npx electron-builder --linux AppImage` on a Linux machine.

- **Server:** downloads the official FFmpeg build (BtbN, GPL) into `third_party/` on first build and embeds it. `EMBED_FFMPEG=0 ./build.sh server` builds a small exe that looks for `ffmpeg.exe` next to it instead. The admin manifest and icon are compiled by `tools/mkres`.
- **Android:** assembled directly with aapt2 → javac → d8 → zipalign → apksigner. On the first build a signing key is created at `android/kaddudesk.keystore` (+ `keystore.pass`). These are **git-ignored. Back them up**: Android only installs updates signed with the same key.
- **Windows client:** electron-builder produces the installer and portable exe.
- **Icons:** `python tools/make_icons.py` regenerates every icon from code.

### Developing
- `npm start` runs the Windows client from source.
- Test the server without elevation and get a plain-HTTP loopback port (WebCodecs works on `http://localhost`):
  ```bash
  cd server && go build -o kd.exe . && ./kd.exe -notray -minimized -devhttp 127.0.0.1:47899
  ```
  The `.syso` with the admin manifest is created by `build.sh` and git-ignored, so a plain `go build` runs un-elevated.
- Server flags: `-notray`, `-minimized`, `-devhttp addr`, `-install-service`, `-uninstall-service` (`-service` and `-agent` are internal).

---

## 15. How it works

```
 PC (server)                                                     Client (app / browser)
 ───────────                                                     ──────────────────────
 Desktop Duplication (ddagrab, GPU) ─┐
                                     ├─ NVENC/AMF/QSV H.264 ─┐
 ffmpeg (zero-copy, no B-frames,     │  ultra-low-latency    │    WebCodecs VideoDecoder
 no lookahead, 2-frame VBV)          ┘                       │    (hardware) → canvas
                                                             │    (desynchronized = no compositor wait)
 RTP on localhost → Go depacketizer → access units ──────────┼──► TLS WebSocket (binary frames)
                                                             │
 WASAPI loopback → Opus (10 ms) ─────────────────────────────┤    WebCodecs AudioDecoder → WebAudio
                                                             │
 SendInput (scancodes / absolute / relative) ◄───────────────┤◄── JSON input messages
 Clipboard · cursor shape+pos · files (HTTPS) · power  ◄─────┘
```

Why it's quicker than Chrome Remote Desktop:
- **Hardware encode** with ultra-low-latency settings, instead of software VP8/VP9.
- **No jitter buffer:** each frame is decoded and drawn the moment it arrives.
- **Local cursor:** pointer movement never waits for the network.
- **Overload handling:** if a viewer falls behind, the server drops frames and resyncs on a fresh keyframe instead of queueing seconds of video. Auto quality lowers the bitrate before that happens.

Measured on an RTX 3060 at 1080p: a steady 60 fps, about 0.6 ms client decode, first frame about 0.5 s after connecting.

Protocol summary (`wss://host:47800/ws?token=…`):
- binary `0x01` video: flags, server µs timestamp, sequence number, Annex-B H.264
- binary `0x02` audio: timestamp + Opus packet
- text JSON for everything else: `hello`, `cfg`, `mm`/`mr`/`mb`/`wh` mouse, `k` key, `txt`, `combo`, `clip`, `cur`/`cp` cursor, `ping`/`pong`, `st` stats, `desk`, `power`, `notice`
- HTTPS: `/api/info`, `/api/login`, `/api/files`, `/api/download`, `/api/upload`. UDP 47810: LAN discovery.

---

## 16. Project layout

```
server/                 Go host server
  main.go               startup, tray, modes
  media.go              ffmpeg control, encoder probing, RTP depacketizer, audio pipeline
  wasapi.go             system-audio loopback capture
  session.go            per-client WebSocket session, input dispatch, cursor/clipboard/desktop watchers
  *_windows.go          Windows: SendInput, DXGI monitors, ddagrab/NVENC args, WASAPI, service mode
  *_linux.go            Linux: uinput input, xrandr/DRM monitors, x11grab/kmsgrab + VAAPI/NVENC, PulseAudio, xclip/wl-clipboard
  web.go                HTTPS server, login, files, LAN discovery
  admin.go admin/       localhost control panel
  service.go proc.go    Service mode (SYSTEM service + agent), desktop-aware process launch
  config.go tls.go      settings, PIN hashing, tokens, certificate
  web/                  the client UI served to apps and browsers (index.html, app.js, style.css, bridge.js)
clients/launcher/       "My PCs" home screen shared by both apps
clients/windows/        Electron shell: certificate pinning, discovery, Wake-on-LAN, clipboard
android/                Android app (Java WebView + native bridge), no Gradle needed
linux/                  Linux server installer, client launcher + installer
tools/mkres/            writes the .syso (admin manifest + icon) for the server exe
tools/make_icons.py     generates all icons
build.sh                one-command build
```

---

## 17. Known limitations / roadmap

- **The Linux server and client were cross-compiled on Windows but haven't been run on a real Linux desktop yet.** If something fails, run `kaddudesk-server -notray` in a terminal and check `~/.config/kaddudesk/server.log`.
- On Linux the remote cursor shape isn't synced (you always see an arrow), and Wayland capture needs KMS (Intel/AMD).
- **Service mode was not tested on real hardware before release.** If something misbehaves, check `C:\ProgramData\KadduDesk\service.log` and `server.log`, then turn Service mode off.
- H.264 4:2:0 only, so very fine coloured text can look slightly soft. A 4:4:4 / AV1 option is planned.
- Over the internet it's TCP only; a UDP/WebRTC transport for lossy mobile networks is planned.
- Android phones can control devices but can't be controlled yet (no screen-sharing host on Android).
- No iOS app yet. iPhone/iPad browsers may lack the WebCodecs H.264 decoding the browser client needs.
- Executables aren't code-signed (SmartScreen warns once).
- GPU scaling isn't used yet (`scale_d3d11` fails in current ffmpeg builds), so lowered resolutions are scaled on the CPU.

## License
MIT. See [LICENSE](LICENSE). Server builds embed FFmpeg (GPL).
