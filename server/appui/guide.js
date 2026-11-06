// In-app guide. `only` limits a section to certain platforms of THIS device.
window.KD_GUIDE = [
  {
    icon: '🚀', title: 'Getting started',
    html: `<p>KadduDesk is the same app on <b>Windows</b>, <b>Android</b> and <b>Linux</b>. Every copy can <b>control</b> other devices, and can be <b>shared</b> so others control it.</p>
    <ol class="steps">
      <li>Install KadduDesk on every device you want to use.</li>
      <li>On the device you want to control, open <b class="ui">This device</b> and turn sharing on (see the guide for your system below). Note its <b class="ui">PIN</b>.</li>
      <li>On the device you control from, open <b class="ui">Devices</b>. Use the chips at the top — <b class="ui">All</b>, <b class="ui">Windows</b>, <b class="ui">Linux</b>, <b class="ui">Android</b>, <b class="ui">Hubs</b> — to pick the kind of device.</li>
      <li>Tap <b class="ui">Connect</b>, check the fingerprint once, enter the PIN — done.</li>
    </ol>
    <div class="note">Tabs: <b>Devices</b> = connect to something · <b>Anywhere</b> = reach devices on other networks · <b>This device</b> = share this one · <b>Guide</b> = this help.</div>`,
  },
  {
    icon: '📶', title: 'Control a device on the same Wi-Fi',
    html: `<ol class="steps">
      <li>Make sure both devices are on the same Wi-Fi and the other one has sharing on.</li>
      <li>Open <b class="ui">Devices</b>. It appears under <b class="ui">Nearby on this network</b> within a few seconds.</li>
      <li>Tap <b class="ui">Connect</b>. The first time, compare the fingerprint with <b class="ui">This device → Certificate fingerprint</b> on the other device and tap <b class="ui">Trust &amp; connect</b>.</li>
      <li>Enter its <b class="ui">PIN</b>, keep <b class="ui">Remember this device</b> on, tap <b class="ui">Connect</b>.</li>
      <li>Next time it's under <b class="ui">My devices</b> — one tap, no PIN.</li>
    </ol>
    <div class="note">Not listed? Tap <b>＋ Add device</b> and type the address shown under <b>This device</b> on the other device. On Windows, set the Wi-Fi to <b>Private</b> (Settings → Network &amp; internet → Wi-Fi → your network).</div>`,
  },
  {
    icon: '🌍', title: 'Control from anywhere (other networks, mobile data)',
    html: `<p>Works from any network in the world, through any router — no port forwarding.</p>
    <ol class="steps">
      <li>On the device you want to control: open <b class="ui">Anywhere</b>, turn it on, tap <b class="ui">Sign in</b> and sign in with Google, Microsoft, GitHub or Apple.</li>
      <li>On the device you control from: open <b class="ui">Anywhere</b>, turn it on and sign in with the <b>same account</b>.</li>
      <li>Open <b class="ui">Devices</b>: your device appears under <b class="ui">Anywhere</b> with an <b class="ui">Internet</b> tag. Tap <b class="ui">Connect</b>, trust it once, enter the PIN.</li>
      <li>Leave Anywhere on. From now on it works wherever you are.</li>
    </ol>
    <div class="note">Before you travel, test it once on mobile data. Keep the PC switched on (or set up Wake-on-LAN through a hub) and turn on Service mode on Windows so you can get past the lock screen.</div>`,
  },
  {
    icon: '🖥', title: 'Share this Windows PC', only: ['Windows'],
    html: `<ol class="steps">
      <li>Open <b class="ui">This device</b> and tap <b class="ui">Turn on sharing</b>.</li>
      <li>Windows asks <i>"Do you want to allow this app to make changes?"</i> — tap <b class="ui">Yes</b>. (Admin rights let others control admin windows too.)</li>
      <li>Your <b class="ui">PIN</b>, <b class="ui">Device ID</b> and addresses are shown. An orange KadduDesk icon appears in the system tray.</li>
      <li>Recommended: in <b class="ui">Server</b>, turn on <b class="ui">Service mode</b>. Sharing then starts with Windows, works on the lock screen and sign-in screen, and Ctrl+Alt+Del works.</li>
      <li>For other networks, turn on <b class="ui">Anywhere</b> (the PC signs in once for the whole PC).</li>
    </ol>
    <div class="note">Pause or stop sharing at any time from the tray icon or <b>This device → Pause sharing / Stop sharing</b>.</div>`,
  },
  {
    icon: '🐧', title: 'Share this Linux computer', only: ['Linux'],
    html: `<ol class="steps">
      <li>If you haven't yet: in the folder you unpacked, run <b class="ui">sudo ./install-client.sh</b> once, then log out and back in (this allows mouse/keyboard control).</li>
      <li>Open <b class="ui">This device</b> — sharing is on while KadduDesk is open. Note the <b class="ui">PIN</b>.</li>
      <li>Wayland (Ubuntu 22.04+, Fedora) needs KMS capture: answer <b>y</b> to the KMS question in the installer (Intel/AMD). With NVIDIA, choose <b>Ubuntu on Xorg</b> on the login screen.</li>
      <li>Turn on <b class="ui">Anywhere</b> to reach it from other networks.</li>
    </ol>`,
  },
  {
    icon: '🤖', title: 'Share this Android phone', only: ['Android'],
    html: `<ol class="steps">
      <li>Open <b class="ui">This device</b> → <b class="ui">Share this phone</b> → <b class="ui">Start sharing</b>.</li>
      <li>Android asks to start recording or casting — choose <b class="ui">Entire screen</b> (if asked) and tap <b class="ui">Start</b>. A KadduDesk notification shows while sharing.</li>
      <li>To let others tap, swipe and type: tap <b class="ui">Open settings</b> next to <b class="ui">Remote control</b> → <b class="ui">Installed apps</b> / <b class="ui">Downloaded apps</b> → <b class="ui">KadduDesk</b> → turn it on → <b class="ui">Allow</b>.</li>
      <li>Note the <b class="ui">PIN</b>. Other devices find this phone under <b class="ui">Devices → Android</b>.</li>
      <li>If Android 13+ shows "restricted setting" for Accessibility: open <b>Settings → Apps → KadduDesk → ⋮ → Allow restricted settings</b>, then try again.</li>
    </ol>
    <div class="note">Android asks for screen permission each time sharing starts — this is an Android security rule. Turn off battery optimisation for KadduDesk (Settings → Apps → KadduDesk → Battery → Unrestricted) so sharing keeps running.</div>`,
  },
  {
    icon: '🔒', title: 'Lock screen, UAC prompts and Ctrl+Alt+Del (Windows)',
    html: `<ol class="steps">
      <li>On the Windows PC: <b class="ui">This device → Server → Service mode</b> → confirm.</li>
      <li>Wait a few seconds; the badge shows <b class="ui">Service · SYSTEM</b>.</li>
      <li>Test: press Win+L on the PC, connect — you'll see the lock screen and can sign in.</li>
    </ol>
    <div class="note">Service mode also starts sharing automatically when Windows boots, before anyone signs in.</div>`,
  },
  {
    icon: '🗂', title: 'Hub: one place for all your devices (accounts + 2FA)',
    html: `<p>A hub (usually your main PC) lists every device, holds accounts with two-factor sign-in, and relays connections for devices behind other routers.</p>
    <ol class="steps">
      <li>On the main PC: <b class="ui">This device → Hub → Be the hub</b>.</li>
      <li>Create the <b class="ui">owner account</b> there (only possible on that PC). Tap <b class="ui">Open hub</b>, sign in, scan the QR code with an authenticator app (Google Authenticator, Microsoft Authenticator, Authy…), and save the recovery codes.</li>
      <li>On every other PC: <b class="ui">This device → Join a hub</b> → hub address + <b class="ui">Host key</b> (shown on the hub) → <b class="ui">Join hub</b>.</li>
      <li>From any device: <b class="ui">Devices → Hubs</b> → <b class="ui">Open hub</b> → sign in → pick a device.</li>
    </ol>`,
  },
  {
    icon: '🧩', title: 'Workspaces: watch many devices, control any of them',
    html: `<ol class="steps">
      <li>Open the hub and sign in as the owner or an admin.</li>
      <li><b class="ui">Workspaces</b> → name it (e.g. <i>Home</i>) → <b class="ui">Create workspace</b> → tick its devices → choose who may use it (<b class="ui">View only</b> / <b class="ui">Full control</b>) → <b class="ui">Save</b>.</li>
      <li>Tap <b class="ui">Open</b>: every online device streams live in a grid.</li>
      <li>Tap any screen to control it at full quality — the others stay live on the side. Tap another to switch; <b class="ui">▦ All screens</b> goes back.</li>
    </ol>`,
  },
  {
    icon: '👪', title: 'Give someone else access',
    html: `<ol class="steps">
      <li>Hub → <b class="ui">Administration → Users → Add a person</b> → username → <b class="ui">Generate</b> → <b class="ui">Create</b>.</li>
      <li>Send them the username and temporary password privately.</li>
      <li>Add them to a workspace with <b class="ui">View only</b> or <b class="ui">Full control</b>.</li>
      <li>They install KadduDesk, open the hub, sign in, choose their own password and set up 2FA.</li>
    </ol>
    <div class="note">Simpler alternative: tell them a device's PIN. You can change it any time under <b>This device → New PIN</b>, and forget remembered devices there.</div>`,
  },
  {
    icon: '🖱', title: 'Using a session',
    html: `<p><b>Computer:</b> your mouse and keyboard control the remote device. Move to the <b>top centre</b> of the window for the menu (or press Ctrl+Alt+Shift+M). <b>Fullscreen</b> sends Win, Alt+Tab and Esc to the remote PC. Drop files on the window to send them.</p>
    <p><b>Phone:</b> drag one finger to move the pointer, tap to click, two-finger tap for right-click, two fingers to scroll, pinch to zoom, three-finger tap for the keyboard. The orange bubble opens the menu.</p>
    <p><b>Controlling an Android phone:</b> click/tap = tap, drag = swipe, right-click = long-press, scroll wheel = scroll. The menu has <b>Back</b>, <b>Home</b>, <b>Recent apps</b> and <b>Notifications</b>.</p>
    <p>The menu also has sound, clipboard, file transfer, shortcuts (Ctrl+Alt+Del, Task Manager…), power (lock, restart…), quality and stats.</p>`,
  },
  {
    icon: '⏰', title: 'Wake a sleeping PC',
    html: `<ol class="steps">
      <li>On the PC: enable <b>Wake on LAN</b> in the BIOS, and in Device Manager → network adapter → Power Management allow it to wake the computer.</li>
      <li>Connect to it once while it's on, so KadduDesk learns its network card.</li>
      <li>When it's asleep: <b class="ui">Devices</b> → <b class="ui">⏰ Wake</b> (same network), or open your hub → <b class="ui">⏰ Wake</b> (from anywhere — the hub sends it at home).</li>
    </ol>`,
  },
  {
    icon: '🛡', title: 'How your connection is protected',
    html: `<p>Every connection is encrypted (TLS, plus WireGuard for Anywhere). Each device's identity is pinned the first time you trust it, so impostors are detected. A PIN or a hub account with two-factor sign-in is required, wrong guesses lock out, and view-only users can't send input. KadduDesk never opens in a web browser and devices refuse browser access.</p>`,
  },
  {
    icon: '🛠', title: 'Troubleshooting',
    html: `<ol class="steps">
      <li><b>Device not in the list:</b> is sharing on there? Same Wi-Fi? Windows network set to Private? Try <b class="ui">＋ Add device</b> with its address, or use <b class="ui">Anywhere</b>.</li>
      <li><b>"Can't reach":</b> the other device is off, asleep, or on another network — turn on Anywhere on both.</li>
      <li><b>Black screen on Linux Wayland:</b> enable KMS capture (installer) or use an Xorg session.</li>
      <li><b>Android: can see but not control:</b> turn on Accessibility for KadduDesk (see "Share this Android phone").</li>
      <li><b>Laggy:</b> open the session menu → <b>Stats</b>. Use 5 GHz Wi-Fi, or choose the <b>Fastest</b> quality preset.</li>
      <li><b>Forgot a PIN:</b> it's on that device under <b class="ui">This device → Show</b>.</li>
      <li><b>Locked out of the hub:</b> on the hub PC, <b class="ui">This device → Hub → Recover…</b>.</li>
    </ol>`,
  },
];
