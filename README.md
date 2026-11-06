# KadduDesk ⚡

**Fast remote desktop between Windows, Android and Linux — any device controls any device, from anywhere in the world.** One app per system, the same look everywhere. It uses your GPU's hardware video encoder (NVENC / AMF / Quick Sync, or the phone's MediaCodec), hardware decoding on the other side, and end-to-end encryption.

> *Kaddu* means pumpkin, hence the orange.

---

## Contents
1. [What's new in 2.0](#1-whats-new-in-20)
2. [Downloads](#2-downloads)
3. [The app at a glance](#3-the-app-at-a-glance)
4. [Step-by-step guides](#4-step-by-step-guides) — install, connect, share, anywhere, hub, workspaces, people, recovery, updates
5. [Using a session](#5-using-a-session)
6. [Security](#6-security)
7. [Troubleshooting](#7-troubleshooting)
8. [Building from source](#8-building-from-source)
9. [How it works](#9-how-it-works)
10. [Project layout](#10-project-layout)
11. [Known limitations](#11-known-limitations)

The full guide is also **inside the app** — open the **Guide** tab.

---

## 1. What's new in 2.0

- **One KadduDesk app on every system.** Windows, Android and Linux all get the same app with the same screens. Each copy can *control* other devices **and** *share* itself.
- **Pick devices by type.** The **Devices** tab has filter chips: **All · Windows · Linux · Android · Hubs**.
- **Connect from anywhere in the world** with the **Anywhere** tab. It works through any router, CGNAT (Jio, Airtel …), mobile data or hotel Wi-Fi, with no port forwarding. You sign in once per device.
- **Android can be controlled now.** You can watch an Android phone's screen and tap, swipe, type and press Back/Home/Recents on it from a PC or another phone.
- **No web pages.** Everything happens in the apps. Devices refuse browser access, and the old browser control panel is now the app's **This device** tab.
- **Guide built in**, matched to the system you're on.

---

## 2. Downloads

Get the files from the **[latest release](https://github.com/Playertxt/kaddudesk/releases/latest)**. Sign in to GitHub first, because the repository is private.

| System | File | What it is |
|---|---|---|
| **Windows 10/11** | `KadduDesk-2.0.0-setup.exe` | The KadduDesk app (installer). Controls other devices and shares this PC. |
| Windows 10/11 | `KadduDesk-2.0.0-portable.exe` | Same app, no install. |
| **Android 7+** | `KadduDesk.apk` | The KadduDesk app for phones and tablets. |
| **Linux (x64)** | `kaddudesk-client-linux-x64.tar.gz` | The KadduDesk app + installer. |
| Windows (optional) | `KadduDeskServer.exe` | Sharing only, no window. For PCs you never sit at: tray icon, Service mode. |
| Linux (optional) | `kaddudesk-server-linux-x64.tar.gz` / `-arm64` | Sharing only (e.g. Raspberry Pi). |
| — | `SHA256SUMS.txt` | Checksums. |

Most people only need **the app** on each device.

---

## 3. The app at a glance

| Tab | What you do there |
|---|---|
| **Devices** | Find and connect to devices: **My devices** (saved), **Nearby on this network**, **Anywhere**. Filter by **All / Windows / Linux / Android / Hubs**, search by name or 9-digit ID, **＋ Add device** by address, **⏰ Wake** a sleeping PC. |
| **Anywhere** | Turn on worldwide connections and sign in. |
| **This device** | Share this device: turn sharing on, PIN, device ID, permissions, connected people, hub, Service mode (Windows), Android sharing and remote-control switches. |
| **Guide** | Step-by-step help, matched to this system. |

---

## 4. Step-by-step guides

### Guide A — Install the app

**Windows**
1. Run **`KadduDesk-2.0.0-setup.exe`**. If SmartScreen says *Windows protected your PC*, click **More info → Run anyway**, because the file isn't code-signed.
2. Follow the installer and open **KadduDesk** from the Start menu.

**Android**
1. Open **`KadduDesk.apk`** on the phone. If installing is blocked, tap **Settings → Allow from this source**, go back, and tap **Install**. If Play Protect warns, tap **More details → Install anyway**.
2. Open **KadduDesk**.

**Linux**
1. Unpack the app:
   ```bash
   tar xzf kaddudesk-client-linux-x64.tar.gz && cd kaddudesk-client-linux-x64
   ```
2. Install it:
   ```bash
   sudo ./install-client.sh
   ```
   This installs dependencies, the menu entry and the remote mouse/keyboard permission. If you use **Wayland** (default on Ubuntu 22.04+ / Fedora) with Intel or AMD graphics, answer **y** to *Enable KMS capture?*.
3. Log out and back in once, then open **KadduDesk** from your app menu.

---

### Guide B — Control a device on the same Wi-Fi

1. On the device you want to control, turn sharing on ([Guide C](#guide-c--share-this-device)).
2. On the device you control from, open **Devices**. Within a few seconds the other one appears under **Nearby on this network**. Use the chips at the top to show only **Windows**, **Linux** or **Android**.
3. Tap **Connect**.
4. **First time only:** a box shows a fingerprint. Check that it matches **Certificate fingerprint** under **This device** on the other device, then tap **Trust & connect**.
5. Enter that device's **PIN** (shown under **This device → Show**), keep **Remember this device** ticked, and tap **Connect**.
6. You're in. Next time it's under **My devices**: one tap, no PIN.

Not listed? Tap **＋ Add device** and type an address shown under **This device** on the other device (e.g. `192.168.1.20`). On Windows, make sure the Wi-Fi is set to **Private** (Settings → Network & internet → Wi-Fi → your network).

---

### Guide C — Share this device

**Windows PC**
1. Open **This device** and click **Turn on sharing**.
2. Windows asks *Do you want to allow this app to make changes to your device?* Click **Yes**. Admin rights let others control admin windows too.
3. The PIN, **Device ID** and addresses appear, and an orange KadduDesk icon appears in the tray. Use **Pause sharing** or **Stop sharing** whenever you like.
4. Recommended: turn on **Service mode** ([Guide D](#guide-d--lock-screen-uac-prompts-and-ctrlaltdel-windows)).

**Android phone**
1. Open **This device → Share this phone → Start sharing**.
2. Android asks *Share your screen with KadduDesk?* Choose **Share entire screen**, then tap **Share screen**. Allow notifications if asked; the notification shows while you're sharing.
3. To let others tap and type: next to **Remote control** tap **Open settings** → **Downloaded apps** / **Installed apps** → **KadduDesk remote control** → turn it on → **Allow**.
   - If Android says *Restricted setting*: open **Settings → Apps → KadduDesk → ⋮ → Allow restricted settings**, then try again.
4. Optional, keeps sharing running in the background: **Settings → Apps → KadduDesk → Battery → Unrestricted**.
5. Android asks for screen permission each time sharing starts. That's an Android security rule.

**Linux computer**
1. Install with `install-client.sh` ([Guide A](#guide-a--install-the-app)) and log out and back in once.
2. Sharing is on while KadduDesk is open. See the PIN under **This device**.

---

### Guide D — Lock screen, UAC prompts and Ctrl+Alt+Del (Windows)

1. On the Windows PC: **This device → Server → Service mode** → confirm.
2. Wait a few seconds. The badge shows **Service · SYSTEM**.
3. Test it: press **Win+L** on the PC, then connect. You'll see the lock screen and can sign in. **Ctrl+Alt+Del** sends the real one.
4. Service mode also starts sharing automatically when Windows boots, before anyone signs in.

---

### Guide E — Connect from anywhere (other networks, mobile data)

1. On the device you want to control: open **Anywhere** → turn on **Anywhere** → **Sign in**. A sign-in page opens; sign in with Google, Microsoft, GitHub or Apple. It's free.
2. On the device you control from: open **Anywhere** → turn it on → **Sign in** with **the same account**.
3. Open **Devices**. The other device appears under **Anywhere (your account, any network)** with an **Internet** tag. Tap **Connect**, trust it once and enter its PIN.
4. Leave **Anywhere** on. It now works wherever you are. On a Windows PC that shares, one sign-in covers the whole PC.

Before you travel, test it once on mobile data. Keep the PC on (or use **⏰ Wake** through a hub) and turn on Service mode, so you can get past the lock screen.

---

### Guide F — Make your main PC the hub (accounts + 2FA)

A hub lists every device in one place and holds people's accounts with two-factor sign-in. It also relays connections for devices behind other routers.

1. On the main PC, turn sharing on ([Guide C](#guide-c--share-this-device)). Service mode is recommended.
2. **This device → Hub → Be the hub.**
3. Under **Create the owner account**, enter a username and a strong password (10+ characters, at least 3 of: lowercase, UPPERCASE, digits, symbols), then **Create owner**. This is only possible on the hub PC itself.
4. Click **Open hub** and sign in.
5. **Set up two-factor sign-in:** install an authenticator app on your phone (Google Authenticator, Microsoft Authenticator, Authy, 1Password …). In it, tap **+ → Scan a QR code**, scan the code on screen, enter the 6-digit code, then **Turn on 2FA**.
6. **Save your recovery codes** (**Copy** or **Download**). Tick **I saved my recovery codes**, then **Continue**.
7. Back in **This device → Hub**, click **Show** next to **Host key**. Other PCs need it in [Guide G](#guide-g--add-your-other-pcs-to-the-hub).

---

### Guide G — Add your other PCs to the hub

1. On each other PC: install the app and turn sharing on.
2. **This device → Join a hub**: enter the **Hub address** (the main PC's IP, or its Anywhere address) and the **Host key**, then **Join hub**.
3. Compare the fingerprint with the hub's and confirm. The card shows **connected**, and the PC gets a 9-digit ID.
4. Leave **Let hub accounts connect without this PC's PIN** on. Signed-in hub users with access then don't need this PC's PIN.

---

### Guide H — Use the hub from any device

1. Open **Devices → Hubs** (or it shows under Nearby/My devices) → **Open hub**.
2. Sign in with your username, password and the 6-digit code.
3. **All devices** lists everything you have access to, with **Connect** and **⏰ Wake**. The hub sends the wake signal from home, so it works from anywhere.
4. **Disconnect** in a session brings you back to the device list.

---

### Guide I — Workspaces: watch many devices, control any of them

1. Open the hub and sign in as the owner or an admin.
2. **Workspaces** → name it (e.g. *Home*) → **Create workspace** → tick its devices → choose who may use it (**View only** / **Full control**) → **Save**.
3. Click **Open (N live)**. Every online device streams live in a grid.
4. **Click any screen** to control it at full quality. The others stay live on the side. Click another to switch; **▦ All screens** goes back; **⛶ Fullscreen** fills the display.

---

### Guide J — Give someone else access

- **Simple:** tell them one device's PIN. You can change it any time (**This device → New PIN**) and remove remembered devices there.
- **With accounts:** hub → **Administration → Users → Add a person** → username → **Generate** → **Create**. Send them the username and temporary password privately, and add them to a workspace. At first sign-in they choose their own password and set up 2FA. Everything they do appears in **Administration → Audit log**.

---

### Guide K — Lost your phone, or can't sign in

- **You have a recovery code:** at **Two-factor check** tap **Use a recovery code instead**, enter one code and **Verify**. Then make new codes under **Account & security**.
- **Another admin exists:** they choose **Administration → Users → Actions… → Reset 2FA** for you.
- **You're the owner with no codes:** on the hub PC, **This device → Hub → Recover…** resets your password and 2FA.
- **This account is temporarily locked:** wait 15 minutes, or ask an admin to **Unlock**.
- **Forgot a device's PIN:** it's on that device under **This device → Show**.

---

### Guide L — Update to a new version

- **Windows app:** run the new setup (or replace the portable exe). If you use Service mode, turn it off and on again afterwards (**This device → Server**) so the service gets the new version.
- **Android:** open the new `KadduDesk.apk` and tap **Update**. Your devices and settings stay.
- **Linux:** unpack the new tarball and run `sudo ./install-client.sh` again.

Settings, PINs, accounts, workspaces and trusted fingerprints are kept.

---

## 5. Using a session

- **From a computer:**
  - Your mouse and keyboard control the remote device.
  - Move to the **top centre** for the menu, or press **Ctrl+Alt+Shift+M**.
  - **Fullscreen** sends Win, Alt+Tab and Esc to the remote PC.
  - Drop files on the window to send them.
- **From a phone:**
  - Drag one finger to move the pointer and tap to click.
  - Two-finger tap = right-click; two fingers = scroll; pinch = zoom; three-finger tap = keyboard.
  - The orange bubble opens the menu. **Input → Direct touch** taps exactly where you touch.
- **Controlling an Android phone:**
  - Click or tap = tap, drag = swipe, right-click = long-press, scroll = scroll.
  - The menu adds **Back**, **Home**, **Recents** and **Notifications**.
  - Typing goes into the focused text box. Esc = Back, Win = Home.
- **Menu:** sound, clipboard sync, files (send and download), shortcuts (Ctrl+Alt+Del, Task Manager, Alt+Tab …), power (lock, sleep, restart …), quality (**Auto / Fastest / Balanced / Sharpest / Custom**), view (fit, fill, 1:1), stats overlay, game mode (pointer lock).

---

## 6. Security

- **Encryption:** every connection uses TLS. Anywhere adds WireGuard (via the Tailscale network), end to end between your devices.
- **Device identity:** each device has its own certificate. The app pins it on first connect and warns loudly if it ever changes, which detects impostors.
- **PIN access:** a salted PBKDF2-hashed PIN with lockout after wrong guesses. *Remember this device* uses a random 256-bit token you can revoke under **This device**.
- **Hub accounts:**
  - Argon2id password hashing and a password policy
  - Two-factor sign-in required by default, with one-time recovery codes
  - Lockout per account and per network
  - HttpOnly, Secure, SameSite=Strict sessions with idle timeout and remote sign-out
  - CSRF protection, roles (owner / admin / member), view-only enforcement on the device itself, and a full audit log
- **No web access:** devices refuse browsers. The app's UI is built into the app and served only on that device's loopback address, locked with a per-launch secret.
- **Optional prompt:** *Ask me before anyone connects* shows a Yes/No prompt on the shared device.

---

## 7. Troubleshooting

| Problem | Fix |
|---|---|
| Device not under **Nearby** | Is sharing on there? Same Wi-Fi? Windows network set to **Private**? Guest Wi-Fi often blocks devices from seeing each other. Use **＋ Add device**, or **Anywhere**. |
| "Can't reach …" | The other device is off, asleep or on another network. Turn on **Anywhere** on both. |
| Android: picture but no control | Turn on **KadduDesk remote control** under Accessibility ([Guide C](#guide-c--share-this-device)). |
| Android: "isn't sharing its screen" | On the phone, **This device → Start sharing** (Android asks every time). |
| Linux Wayland: black screen | Enable KMS capture in the installer, or pick an **Xorg** session on the login screen. |
| Lock screen / UAC prompt not visible (Windows) | Turn on **Service mode** ([Guide D](#guide-d--lock-screen-uac-prompts-and-ctrlaltdel-windows)). |
| Laggy | Session menu → **Stats**. Use 5 GHz Wi-Fi or the **Fastest** preset. |
| **Anywhere** says sign in again | Tap **Sign in**. Sessions last months, but signing out elsewhere ends them. |
| "This device changed" warning | Only expected if KadduDesk was reinstalled there. Otherwise cancel. |

Logs:
- **Windows:** `%APPDATA%\KadduDesk\server.log` and `app\app.log`; `C:\ProgramData\KadduDesk\` in Service mode.
- **Linux:** `~/.config/kaddudesk/`

---

## 8. Building from source

Requirements: Go 1.24+, Node.js 20+, a JDK 17+, the Android SDK (build-tools + platform 34+), Git Bash, and Python 3.

```bash
git clone https://github.com/Playertxt/kaddudesk.git
```
```bash
cd kaddudesk && npm install
```
```bash
./build.sh all
```

Targets: `server`, `linux`, `android`, `windows`, `linux-client`, `all`. Output goes to `dist/`.
- **Windows:** `KadduDeskServer.exe` embeds ffmpeg (downloaded on first build) and is also the core bundled in the Windows app.
- **Android:** built without Gradle. The core is compiled for arm64 and x86_64 and packed as `lib/<abi>/libkdcore.so`. The signing key is created at `android/kaddudesk.keystore`; it's git-ignored, so **back it up**. Android only accepts updates signed with the same key. Use `KD_ANDROID_DEBUG=1` for a debuggable test build.
- **Linux:** a pure-Go cross-compile, and the Electron app is packed as a tarball.

---

## 9. How it works

```
 ┌───────────── KadduDesk app (any OS) ─────────────┐        ┌────── device being controlled ───────┐
 │  native shell (Electron / Android WebView)        │        │  KadduDesk core as host               │
 │   └─ shows the app UI served by ──┐               │  TLS   │   capture: ddagrab (Win) · x11grab/    │
 │  KadduDesk core (Go, -app)  ◄──────┘               │◄──────►│            kmsgrab (Linux) ·          │
 │   • LAN discovery, certificate pinning            │  over  │            MediaProjection (Android)  │
 │   • sessions → direct / hub / Anywhere            │ LAN,   │   encode: NVENC/AMF/QSV/VAAPI/x264 ·  │
 │   • hub sign-in, workspaces                       │ hub or │           MediaCodec                   │
 │   • Anywhere = embedded Tailscale node            │ mesh   │   input: SendInput · uinput ·          │
 └───────────────────────────────────────────────────┘        │          AccessibilityService          │
                                                              └───────────────────────────────────────┘
```

Video is H.264 with no B-frames or lookahead, using hardware encode and hardware decode (WebCodecs). Each frame is drawn the moment it arrives, with no jitter buffer, and slow viewers resync on a fresh keyframe instead of building lag. The same core binary runs everywhere. On Windows it's both the app's core and the (elevated) sharing host; on Linux and Android the app runs sharing inside itself.

---

## 10. Project layout

```
server/                    the KadduDesk core (Go): host + app core + hub
  appcore.go               app mode: loopback UI, discovery, pinning, session proxy, local control
  appui/                   the app's home screens (Devices, Anywhere, This device, Guide)
  web/                     the remote-session screen
  hubweb/                  hub screens (sign-in, workspaces, multi-view, admin)
  admin/                   "This device" panel
  mesh.go                  Anywhere (embedded Tailscale)
  hub.go accounts*.go      hub relay, accounts, 2FA, workspaces, audit
  tunnel.go                joining a hub (outbound link, dial-back)
  *_windows.go *_linux.go  platform capture / input / audio / service
  android_platform.go      Android bridge (frames in, touches out)
android/                   Android app (Java): core launcher, screen share service, accessibility control
clients/windows/           Electron shell for Windows and Linux
linux/                     Linux installers
tools/                     icon generator, resource compiler
```

---

## 11. Known limitations

- **Anywhere** uses the free Tailscale network, so each device signs in once with a Google/Microsoft/GitHub/Apple account. Tailscale's free plan covers personal use.
- **Android** asks for screen permission each time sharing starts (an Android rule). Its sound isn't shared yet, and reading the phone's clipboard from outside isn't possible on modern Android.
- **Not yet tested on real hardware:** Anywhere sign-in end to end, Linux desktops, Windows Service mode, and a physical Android phone. The Android emulator, the Windows apps and the Windows/Android combinations were tested.
- The files aren't code-signed, so SmartScreen / Play Protect warn once.
- No iOS app.

## License
MIT. See [LICENSE](LICENSE). Windows builds embed FFmpeg (GPL). Anywhere uses Tailscale's open-source client (BSD-3).
