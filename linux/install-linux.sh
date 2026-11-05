#!/usr/bin/env bash
# KadduDesk server installer for Linux (Ubuntu/Debian, Fedora, Arch, openSUSE ...).
#   sudo ./install-linux.sh            install / update
#   sudo ./install-linux.sh --uninstall
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
DEST=/opt/kaddudesk
USER_NAME="${SUDO_USER:-${USER}}"

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run with sudo:  sudo $0 $*"; exit 1
fi

if [ "${1:-}" = "--uninstall" ]; then
  rm -rf "$DEST" /usr/local/bin/kaddudesk-server /etc/udev/rules.d/60-kaddudesk-uinput.rules \
         /etc/modules-load.d/kaddudesk.conf /usr/share/applications/kaddudesk-server.desktop \
         /usr/share/icons/hicolor/256x256/apps/kaddudesk.png
  UHOME="$(getent passwd "$USER_NAME" | cut -d: -f6)"
  rm -f "$UHOME/.config/autostart/kaddudesk-server.desktop"
  udevadm control --reload-rules || true
  echo "KadduDesk removed. Your settings remain in ~/.config/kaddudesk (delete it to forget the PIN and devices)."
  exit 0
fi

echo "== Installing dependencies"
PKGS_DEB="ffmpeg xclip wl-clipboard zenity xdotool x11-xserver-utils libcap2-bin"
PKGS_RPM="ffmpeg xclip wl-clipboard zenity xdotool xrandr libcap"
PKGS_ARCH="ffmpeg xclip wl-clipboard zenity xdotool xorg-xrandr libcap"
if command -v apt-get >/dev/null; then apt-get update -qq && apt-get install -y $PKGS_DEB || true
elif command -v dnf >/dev/null; then dnf install -y $PKGS_RPM || echo "(Fedora: enable RPM Fusion for full ffmpeg with hardware encoders)"
elif command -v pacman >/dev/null; then pacman -S --needed --noconfirm $PKGS_ARCH || true
elif command -v zypper >/dev/null; then zypper install -y $PKGS_RPM || true
else echo "Unknown package manager: install ffmpeg, xclip or wl-clipboard, zenity, xdotool yourself."; fi
command -v ffmpeg >/dev/null || { echo "ffmpeg is required. Install it and run this again."; exit 1; }

echo "== Installing to $DEST"
mkdir -p "$DEST"
install -m 0755 "$HERE/kaddudesk-server" "$DEST/kaddudesk-server"
ln -sf "$DEST/kaddudesk-server" /usr/local/bin/kaddudesk-server
install -Dm 0644 "$HERE/kaddudesk.png" /usr/share/icons/hicolor/256x256/apps/kaddudesk.png
cat > /usr/share/applications/kaddudesk-server.desktop <<EOF
[Desktop Entry]
Type=Application
Name=KadduDesk Server
Comment=Let other devices control this computer
Exec=$DEST/kaddudesk-server
Icon=kaddudesk
Terminal=false
Categories=Network;RemoteAccess;
EOF

echo "== Allowing mouse/keyboard control (uinput)"
echo uinput > /etc/modules-load.d/kaddudesk.conf
modprobe uinput || true
cat > /etc/udev/rules.d/60-kaddudesk-uinput.rules <<'EOF'
# KadduDesk: let the logged-in desktop user create virtual input devices
KERNEL=="uinput", SUBSYSTEM=="misc", MODE="0660", GROUP="input", TAG+="uaccess", OPTIONS+="static_node=uinput"
EOF
udevadm control --reload-rules && udevadm trigger --name-match=uinput || true
getent group input >/dev/null && usermod -aG input "$USER_NAME" || true

echo
echo "== Wayland screen capture (optional)"
echo "On Wayland desktops (default on Ubuntu 22.04+, Fedora) the screen can only be captured"
echo "through KMS. That needs a private copy of ffmpeg with the CAP_SYS_ADMIN capability,"
echo "executable only by '$USER_NAME'. Works on Intel/AMD graphics. (On X11 this isn't needed.)"
read -r -p "Enable KMS capture? [y/N] " ans || ans=n
if [[ "$ans" =~ ^[Yy]$ ]]; then
  install -m 0750 -o root -g "$(id -gn "$USER_NAME")" "$(command -v ffmpeg)" "$DEST/ffmpeg-kms"
  setcap cap_sys_admin+ep "$DEST/ffmpeg-kms" && echo "   KMS capture enabled."
fi

echo "== Firewall"
if command -v ufw >/dev/null && ufw status | grep -q "Status: active"; then
  ufw allow 47800/tcp comment 'KadduDesk' && ufw allow 47810/udp comment 'KadduDesk discovery'
elif command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null 2>&1; then
  firewall-cmd --permanent --add-port=47800/tcp --add-port=47810/udp && firewall-cmd --reload
else
  echo "   no active firewall detected"
fi

echo "== Start at login"
UHOME="$(getent passwd "$USER_NAME" | cut -d: -f6)"
sudo -u "$USER_NAME" mkdir -p "$UHOME/.config/autostart"
sudo -u "$USER_NAME" tee "$UHOME/.config/autostart/kaddudesk-server.desktop" >/dev/null <<EOF
[Desktop Entry]
Type=Application
Name=KadduDesk Server
Exec=$DEST/kaddudesk-server -minimized
Icon=kaddudesk
Terminal=false
X-GNOME-Autostart-enabled=true
EOF

echo
echo "Done! Log out and back in once (so the input permission applies), then start"
echo "'KadduDesk Server' from your app menu. The control panel opens in your browser"
echo "and shows the PIN.  Tip: on Wayland without KMS capture, choose an 'Xorg' session"
echo "on the login screen (gear icon) for screen capture."
