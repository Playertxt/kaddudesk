# KadduDesk ⚡

**Fast, low-latency remote desktop for Windows and Linux.** Control your PC from an Android phone, a Windows or Linux PC, or any modern browser. It works like Chrome Remote Desktop, but it encodes the screen on your GPU (NVENC / AMF / Quick Sync) and decodes it in hardware on the client. The result is noticeably snappier.

> *Kaddu* means pumpkin, hence the orange.

---

## Table of contents
0. [**Step-by-step guides** (start here)](#step-by-step-guides)
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

## Step-by-step guides

Each guide below can be followed start to finish on its own. **Bold text** is exactly what you'll see on screen.

| I want to… | Follow |
|---|---|
| Get the right files | [Guide A](#guide-a--download-the-right-files) |
| Control my Windows PC from my Android phone at home | [Guide B](#guide-b--control-your-windows-pc-from-your-android-phone-same-wi-fi) |
| Control a Windows PC from another Windows PC | [Guide C](#guide-c--control-a-windows-pc-from-another-windows-or-linux-pc) |
| Have it start by itself and work on the lock screen | [Guide D](#guide-d--start-automatically-and-work-on-the-lock-screen) |
| Use it when I'm away from home | [Guide E](#guide-e--use-it-from-anywhere-outside-your-home) |
| Control **all** my computers from **any** device, AnyDesk-style | [Guides F → G → H](#guide-f--make-your-main-pc-the-hub) |
| Watch many computers at once and pick one to control | [Guide I](#guide-i--create-a-workspace-and-control-many-devices-at-once) |
| Give a family member or colleague access | [Guide J](#guide-j--give-someone-else-access) |
| Add a Linux computer | [Guide K](#guide-k--add-a-linux-computer) |
| Get back in after losing my phone | [Guide L](#guide-l--you-lost-your-phone-or-cant-sign-in) |
| Update to a new version | [Guide M](#guide-m--update-to-a-new-version) |

---

### Guide A — Download the right files

1. On the device where you'll install something, open **https://github.com/Playertxt/kaddudesk/releases/latest** and sign in to GitHub (the repository is private).
2. Under **Assets**, download only what you need:

   | Device | Role | Download |
   |---|---|---|
   | Windows PC | **to be controlled** (or the hub) | `KadduDeskServer.exe` |
   | Linux PC / Raspberry Pi | **to be controlled** | `kaddudesk-server-linux-x64.tar.gz` (Pi: `-arm64`) |
   | Android phone/tablet | to control others | `KadduDesk.apk` |
   | Windows PC | to control others | `KadduDesk-1.2.0-setup.exe` (or `-portable.exe`, no install) |
   | Linux PC | to control others | `kaddudesk-client-linux-x64.tar.gz` |

   A PC can be both: install the server on it to make it controllable, and the client to use it to control others.
3. Optional: check a download with `certutil -hashfile <file> SHA256` (Windows) or `sha256sum <file>` (Linux), and compare the result with `SHA256SUMS.txt`.

---

### Guide B — Control your Windows PC from your Android phone (same Wi-Fi)

**On the PC (about 2 minutes)**
1. Double-click **`KadduDeskServer.exe`**.
2. If Windows shows **Windows protected your PC**, click **More info → Run anyway**. The file isn't code-signed, which is why Windows warns.
3. Windows asks **Do you want to allow this app to make changes to your device?** Click **Yes**. KadduDesk needs admin rights to control admin windows such as Task Manager.
4. Your browser opens the **KadduDesk control panel**, and an orange icon appears in the system tray (bottom-right; click **^** if you can't see it).
5. In the **Connect a device** card, click **Show** next to **PIN** and keep the page open.
6. Make sure the PC's network is **Private**: **Start → Settings → Network & internet → Wi-Fi** (or **Ethernet**) → your network → **Network profile type: Private**.

**On the phone (about 3 minutes)**
7. Get `KadduDesk.apk` onto the phone: download it there from the release page, or send it to yourself.
8. Open the file. If Android says installing is blocked, tap **Settings**, turn on **Allow from this source**, go back, and tap **Install**. If Play Protect warns, tap **More details → Install anyway**.
9. Open **KadduDesk**. Your PC appears under **Nearby on this network** within a few seconds (the phone must be on the same Wi-Fi).
   - Not there? Type the PC's address from the control panel (e.g. `192.168.1.104`) under **Add a PC by address**, leave the port at **47800**, and tap **Connect**.
10. Tap **Connect** next to your PC.
11. A **Trust this PC?** box shows a fingerprint. Check that it matches **Certificate fingerprint** in the PC's control panel, then tap **Trust & connect**. You only do this once.
12. Enter the **PIN** from step 5, keep **Remember this device** ticked, and tap **Connect**.
13. 🎉 You see your PC's screen. Drag one finger to move the pointer and tap to click. Tap the orange **bubble** for the menu (keyboard, sound, shortcuts, files, power…). All gestures are listed in [section 10](#10-using-a-session-controls-gestures-shortcuts).
14. Next time, just open the app and tap **Connect** under **My PCs**. No PIN needed.

---

### Guide C — Control a Windows PC from another Windows (or Linux) PC

1. Set up the PC you want to control: **Guide B, steps 1–6**.
2. On the other PC, run **`KadduDesk-1.2.0-setup.exe`** (or the portable exe). If SmartScreen warns, click **More info → Run anyway**, then follow the installer.
   - Linux: see [Guide K, part 2](#guide-k--add-a-linux-computer).
3. Open **KadduDesk**. Under **Nearby on this network**, click **Connect** next to the PC (or add it by address with port **47800**).
4. Check the fingerprint, click **Trust & connect**, enter the PIN, and click **Connect**.
5. Your mouse and keyboard now control the remote PC. Useful tips:
   - Move the mouse to the **top centre** of the window to open the **KadduDesk** menu, or press **Ctrl+Alt+Shift+M**.
   - Menu → **Fullscreen** sends Win, Alt+Tab and Esc to the remote PC.
   - Drag files onto the window to copy them to the remote PC's **Downloads\KadduDesk** folder.

---

### Guide D — Start automatically and work on the lock screen

Pick **one** of these, in the PC's control panel → **Server** card.

**Option 1 — Service mode (recommended).** Works on the lock screen, the sign-in screen and UAC prompts, sends a real Ctrl+Alt+Del, and runs even before anyone signs in.
1. Turn on **Service mode** and click **OK** on the confirmation.
2. Wait about 5 seconds. The page reloads and the badge shows **Service · SYSTEM**.
3. Test it: press **Win+L** on the PC, then connect from your phone. You should see the lock screen and be able to type your PIN or password.

**Option 2 — Start with Windows.** Simpler, but it can't see the lock screen or UAC prompts.
1. Turn on **Start with Windows (as admin, no UAC prompt)**.

---

### Guide E — Use it from anywhere (outside your home)

**Option 1 — Tailscale (easiest and safest, no router changes)**
1. On the PC: go to **https://tailscale.com/download**, install Tailscale and sign in (Google, Microsoft or GitHub account).
2. On your phone: install **Tailscale** from the Play Store and sign in with **the same account**.
3. In the Tailscale app on the phone, find your PC and note its address, which starts with **100.** (e.g. `100.101.102.103`).
4. In KadduDesk on the phone: **Add a PC by address** → that `100.x.x.x` address, port **47800** → **Connect** → trust → PIN.
5. It now works over mobile data or any Wi-Fi, as long as Tailscale is on.
   - Using a hub ([Guide F](#guide-f--make-your-main-pc-the-hub))? Add the hub's `100.x.x.x` address with port **47820** instead.

**Option 2 — Router port forwarding**
1. Give the PC a fixed address: open your router's settings (usually **http://192.168.1.1** or **http://192.168.0.1**; the login is often on the router's sticker), find **DHCP reservation** / **Address reservation**, and reserve the PC's current IP.
2. Find **Port forwarding** / **Virtual server** / **NAT** and add a rule: **TCP**, external port **47800**, internal IP = the PC, internal port **47800**. If you use a hub, forward **47820** to the hub PC instead; one rule covers all your devices.
3. Find your public IP (search "what is my IP" on the PC). In the app, add that IP with the port you forwarded.
4. Your public IP can change. Consider a free dynamic-DNS name (many routers have **DDNS** built in) and use the name instead.
5. Recommended: turn on **Ask me before anyone connects** (control panel → **Permissions**), or use the hub with 2FA accounts ([Guide F](#guide-f--make-your-main-pc-the-hub)).
6. If it doesn't work, your provider may use CGNAT (no real public IP). Use Tailscale instead.

---

### Guide F — Make your main PC the hub

The hub lets you control **all** your computers from any device, through one address, with personal accounts and two-factor sign-in.

1. Set up KadduDesk on your main PC: **Guide B, steps 1–6**. Ideally also turn on **Service mode** ([Guide D](#guide-d--start-automatically-and-work-on-the-lock-screen)), so the hub is always running.
2. In the control panel, find the **Hub · control every device from anywhere** card and turn on **Be the hub**. The badge turns **running**.
3. In the same card, under **Create the owner account**, enter:
   - **Username**, e.g. `kabir`
   - **Password**: at least 10 characters, using at least 3 of lowercase, UPPERCASE, digits and symbols (or 16+ characters). Use a password manager if you can.

   Click **Create owner**. (For security, the owner can only be created here, on this PC.)
4. Click **Open hub** (or open `https://<this-PC's-IP>:47820/` on any device). In a browser, accept the certificate warning (**Advanced → Proceed**). The KadduDesk apps handle this with their **Trust** dialog.
5. Sign in with the username and password from step 3.
6. **Turn on two-factor sign-in** (required):
   1. On your phone, install an authenticator app: **Google Authenticator**, **Microsoft Authenticator**, **Authy** or **1Password**.
   2. In the app, tap **+** → **Scan a QR code** and scan the QR code on screen (or type the key shown below it).
   3. Type the 6-digit code the app shows and click **Turn on 2FA**.
7. **Save your recovery codes.** Click **Copy** or **Download** and store them in a password manager or on paper. Each code gets you in once if you lose your phone. Tick **I saved my recovery codes** and click **Continue**.
8. You're in the hub. Your main PC already appears under **All devices** with a **hub** badge.
9. Back in the control panel's **Hub** card, click **Show** next to **Host key** and note it. Other PCs need it in [Guide G](#guide-g--add-your-other-pcs-to-the-hub).
10. For use away from home, set up Tailscale or forward **TCP 47820** ([Guide E](#guide-e--use-it-from-anywhere-outside-your-home)). The other PCs don't need any port forwarding.

---

### Guide G — Add your other PCs to the hub

Repeat for every computer you want to control (Windows; for Linux see [Guide K](#guide-k--add-a-linux-computer)):

1. Install KadduDesk on it: **Guide B, steps 1–4** (and optionally Service mode, [Guide D](#guide-d--start-automatically-and-work-on-the-lock-screen)).
2. In its control panel, find the **Join a hub** card. Note **This PC's ID** (9 digits, e.g. `096 323 761`).
3. **Hub address**: the main PC's address.
   - Same network: its local IP, e.g. `192.168.1.10`.
   - Elsewhere: its Tailscale `100.x.x.x` address or your public IP / DDNS name. Add `:47820` only if you changed the hub port.
4. **Host key**: the key from [Guide F step 9](#guide-f--make-your-main-pc-the-hub).
5. Click **Join hub**. A box shows the hub's certificate fingerprint. Compare it with **Certificate fingerprint** in the main PC's **Connect a device** card, and click **OK** if they match.
6. The card shows **connected**. The PC now appears in the hub under **All devices**.
7. Leave **Let hub accounts connect without this PC's PIN** on (recommended). Hub users then don't need this PC's PIN; the hub has already checked their password and 2FA, and they need your permission to see it.

---

### Guide H — Control any device through the hub

**From the KadduDesk app (Android / Windows / Linux)**
1. Open KadduDesk. On the same network the hub appears under **Nearby** as **🌐 <PC name> · Hub**. Tap **Open hub**.
   - Elsewhere: **Add a PC by address** → hub address, port **47820** → **Connect**.
2. First time only: check the fingerprint and tap **Trust & connect**.
3. Sign in with your **username** and **password**. Tick **Keep me signed in on this device** if it's your personal device.
4. Enter the **6-digit code** from your authenticator app and tap **Verify**.
5. Tap **All devices**, then **Connect** on any device. Or open a workspace to see several at once ([Guide I](#guide-i--create-a-workspace-and-control-many-devices-at-once)).
6. To return to the list, open the in-session menu (bubble or top tab) → **Disconnect**.

**From any browser (no install)**
1. Open `https://<hub-address>:47820/` in Chrome or Edge, accept the certificate warning (**Advanced → Proceed**), then follow steps 3–5 above.

**A device is offline (asleep)?** If you set up Wake-on-LAN ([section 9](#9-wake-on-lan)), click **⏰ Wake** on it in **All devices**. The hub sends the wake signal from your home network, so it works even from mobile data.

---

### Guide I — Create a workspace and control many devices at once

1. Sign in to the hub as the owner or an admin ([Guide H](#guide-h--control-any-device-through-the-hub)).
2. Open the **Workspaces** tab. In **New workspace name**, type e.g. `Home` and click **Create workspace**.
3. The editor opens:
   1. Under **Devices**, tick every computer that belongs in this workspace.
   2. Under **Who can use it**, choose **View only** or **Full control** for each person (owners and admins can always use everything).
   3. Click **Save**.
4. Back on **Workspaces**, click **Open (N live)** on the workspace.
5. Every online device in it now streams live in a grid. Thumbnails are small and low-frame-rate to save bandwidth.
6. **Click a screen** to control it. It becomes the big view at full quality, with sound and your keyboard, and the others shrink to a live strip on the side (along the bottom on phones).
7. Click another screen in the strip to switch instantly. **▦ All screens** goes back to the grid, **⛶ Fullscreen** fills your display, and **← Back** closes the workspace.

---

### Guide J — Give someone else access

1. Sign in to the hub as the owner or an admin → **Administration** → **Users**.
2. Under **Add a person**, enter a **Username**, click **Generate** for a temporary password, choose **Member** (or **Admin**, owner only), and click **Create**.
3. Send them, privately (not in a public chat):
   - the hub address (and that it uses port **47820**)
   - their username and temporary password
   - which app to install ([Guide A](#guide-a--download-the-right-files))
4. Give them devices: **Workspaces** tab → **Edit** on a workspace → set them to **View only** or **Full control** → **Save**.
5. At their first sign-in they must choose their own password and set up 2FA (steps 6–7 of [Guide F](#guide-f--make-your-main-pc-the-hub)).
6. Later, under **Administration → Users → Actions…**, you can reset their password, reset 2FA (if they lost their phone), unlock, disable or delete them. Everything they do appears in **Administration → Audit log**.

---

### Guide K — Add a Linux computer

**Part 1 — make it controllable**
1. Download `kaddudesk-server-linux-x64.tar.gz` (Raspberry Pi: `-arm64`).
2. Open a terminal in the download folder and unpack it:
   ```bash
   tar xzf kaddudesk-server-linux-x64.tar.gz && cd kaddudesk-server
   ```
3. Run the installer:
   ```bash
   sudo ./install-linux.sh
   ```
   - Asked **Enable KMS capture? [y/N]**? Answer **y** if you use **Wayland** (default on Ubuntu 22.04+ and Fedora) with Intel or AMD graphics. Answer **N** on X11, or with NVIDIA graphics (then choose **Ubuntu on Xorg** with the gear icon on the login screen).
4. **Log out and back in** once.
5. Open your app menu → **KadduDesk Server**. The control panel opens in your browser with the PIN.
6. Connect to it like any PC ([Guide B](#guide-b--control-your-windows-pc-from-your-android-phone-same-wi-fi) / [C](#guide-c--control-a-windows-pc-from-another-windows-or-linux-pc)), or add it to your hub ([Guide G](#guide-g--add-your-other-pcs-to-the-hub), steps 2–7).

**Part 2 — use Linux to control other computers**
1. Download `kaddudesk-client-linux-x64.tar.gz` and unpack it:
   ```bash
   tar xzf kaddudesk-client-linux-x64.tar.gz && cd kaddudesk-client-linux-x64
   ```
2. Install it:
   ```bash
   sudo ./install-client.sh
   ```
3. Open **KadduDesk** from your app menu.

---

### Guide L — You lost your phone or can't sign in

- **You still have a recovery code:** at the **Two-factor check**, click **Use a recovery code instead**, enter one code (e.g. `abcde-fghjk`) and click **Verify**. Then go to **Account & security** and make new recovery codes; set up 2FA again on your new phone if needed.
- **Someone else is an admin:** ask them to use **Administration → Users → Actions… → Reset 2FA** on your account.
- **You're the owner and have no codes:** on the hub PC itself, open the control panel → **Hub** card → **Recover…**. Enter your username and a new password. This resets your 2FA, and you set it up again at the next sign-in.
- **This account is temporarily locked** after wrong attempts: wait 15 minutes, or have an admin choose **Unlock**.
- **Forgot a device's PIN:** it's in that PC's control panel → **Connect a device** → **Show**.

---

### Guide M — Update to a new version

1. Download the new files ([Guide A](#guide-a--download-the-right-files)).
2. **Windows PC in normal mode:** tray icon → **Quit KadduDesk** → replace `KadduDeskServer.exe` with the new one → start it.
3. **Windows PC in Service mode:** control panel → turn **Service mode** off → run the new `KadduDeskServer.exe` → turn **Service mode** on again. This copies the new version into Program Files.
4. **Linux:** unpack the new tarball and run `sudo ./install-linux.sh` again.
5. **Android:** open the new `KadduDesk.apk` and tap **Update**. Your PCs and settings stay.
6. **Windows/Linux client:** run the new installer, or replace the portable exe.

Settings, PINs, accounts, workspaces and trusted fingerprints are kept across updates.

---

## 1. What you get

| Build output (`dist/`) | Runs on | Purpose |
|---|---|---|
| `KadduDeskServer.exe` | The PC you want to control (Windows 10/11, 64-bit) | Host server. Single file with ffmpeg embedded. Asks for admin rights. Lives in the system tray. |
| `KadduDesk.apk` | Android 7.0+ | Phone/tablet client. |
| `KadduDesk-1.2.0-setup.exe` | Windows 10/11 | Desktop client, installer. |
| `KadduDesk-1.2.0-portable.exe` | Windows 10/11 | Desktop client, no install. |
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
- **Workspaces**: group devices, watch them all live at once, click one to take control; per-person *view only* / *full control*
- **Accounts with strong security**: Argon2id passwords, mandatory 2FA (authenticator app + recovery codes), lockout, session management, audit log
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

- Install `KadduDesk-1.2.0-setup.exe`, or just run `KadduDesk-1.2.0-portable.exe`.
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

Make **your main PC the hub**. Every other computer (Windows or Linux) *joins* it and keeps an outbound connection to it, so it's reachable from anywhere, even behind another router or on mobile data. Each device gets a 9-digit **device ID** like AnyDesk. People **sign in to the hub with their own account** (password + two-factor code). They see the devices you've shared with them, grouped into **workspaces**, and can watch them all at once and take control of any one. **All traffic is relayed through the main PC**, so the load sits there.

```
  phone / laptop / browser ──► HUB (main PC, port 47820) ◄── PC #2 (joined, outbound)
         signs in: password + 2FA     ▲          ▲
                                      │          └──── Linux box (joined, outbound)
                                      └─ the main PC itself is a device too
```

### Set up the hub (main PC)
1. Control panel → **Hub** card → turn on **Be the hub**.
2. In the same card, **create the owner account** (username + strong password). For security this can only be done on the hub PC itself.
3. Open the hub (`https://<hub-address>:47820/`, or **Open hub** in the app) and sign in. You'll be asked to **set up two-factor authentication**: scan the QR code with an authenticator app (Google Authenticator, Microsoft Authenticator, Authy, 1Password…) and **save the 10 recovery codes** somewhere safe.
4. Note the **Host key** in the Hub card. Other PCs need it to join.
5. For access from outside your home, forward **TCP 47820** on your router to the main PC, or use Tailscale. That's the only port you need.

### Join your other PCs
1. Install and run KadduDesk Server on each PC (Windows `.exe` or the Linux package).
2. Control panel → **Join a hub** → hub address (e.g. `192.168.1.10`, `my-home.example.com` or a Tailscale IP) + **host key** → **Join hub**, and confirm the fingerprint matches the hub's.
3. Leave **"Let hub accounts connect without this PC's PIN"** on, so signed-in hub users with access don't need each PC's PIN. Turn it off to always require this PC's own PIN as well.

### Workspaces: watch many devices, control any one
- **Workspaces** tab → *Create workspace* (owner/admins) (e.g. "Home", "Office"). Tick the devices it contains, and pick who can use it: **Full control** or **View only**.
- **Open** a workspace and every online device in it streams live in a grid. Thumbnails run at 10 fps / 360p to save bandwidth.
- **Click any screen** to take control. It becomes the big view at full quality, with sound and keyboard focus. The others keep updating on the side. Click another screen to switch instantly, or **▦ All screens** to go back to the wall.
- **All devices** lists every device you can access, with search by name or ID, **Connect** (single full-screen session) and **⏰ Wake** (Wake-on-LAN sent from the hub's network).

### People and permissions
| Role | Can do |
|---|---|
| **Owner** (first account) | Everything, including the security policy and making admins |
| **Admin** | Manage members and workspaces; use every device |
| **Member** | Only the devices in workspaces they're added to, with *view only* or *full control* per workspace |

**Administration → Users → Add a person** creates an account with a temporary password (use **Generate**). Share it privately. At first sign-in they must choose their own password and set up 2FA. From the same list you can reset a password, reset 2FA (lost phone), unlock, disable or delete an account.

### Login security
- **Passwords:** hashed with **Argon2id** (64 MiB, 3 passes). Minimum 10 characters, mixing 3 character types (or 16+ characters). Common passwords and ones containing the username are rejected.
- **Two-factor authentication:** TOTP (RFC 6238) from any authenticator app, **required for everyone by default**. Used codes can't be replayed. 10 single-use recovery codes are stored only as hashes.
- **Brute-force protection:** after 5 failures the account locks, for 15 min at first and doubling up to 24 h. The network address is also locked. Errors never reveal whether a username exists.
- **Sessions:** random 256-bit token in a `__Host-` cookie that is HttpOnly, Secure and SameSite=Strict. The server stores only its hash. Sessions sign out after inactivity (default 60 min), or last up to 30 days with *Keep me signed in*. **Account & security** lists your sessions and can sign them out remotely; changing your password signs out every other session.
- **CSRF protection** on every change, a strict Content-Security-Policy on the hub pages, and HSTS / no-referrer / frame protections.
- **Device trust:** devices accept a hub user only over their authenticated, certificate-pinned hub tunnel. The same headers on a direct connection are ignored. View-only users can't send input, files, clipboard or power commands; the device itself enforces this.
- **Audit log** (Administration → Audit log): every sign-in, failure, lockout, 2FA change, user/workspace change and device session, with time and IP.
- **Locked out?** On the hub PC: control panel → Hub → **Recover…** resets that account's password and 2FA. This only works on the hub PC itself.
- The owner can change the policy under **Administration → Security policy**: require 2FA, inactivity timeout, keep-signed-in length.

The hub terminates TLS and relays the traffic, so run it on a PC you trust: your own main PC.

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
