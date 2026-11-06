#!/usr/bin/env bash
# Installs the KadduDesk app for Linux: control other devices AND share this one.
#   sudo ./install-client.sh            install / update
#   sudo ./install-client.sh --uninstall
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
DEST=/opt/kaddudesk-client
USER_NAME="${SUDO_USER:-${USER}}"
[ "$(id -u)" -eq 0 ] || { echo "Please run with sudo:  sudo $0 $*"; exit 1; }

if [ "${1:-}" = "--uninstall" ]; then
  rm -rf "$DEST" /usr/local/bin/kaddudesk /usr/share/applications/kaddudesk.desktop \
         /usr/share/icons/hicolor/256x256/apps/kaddudesk-client.png \
         /etc/udev/rules.d/60-kaddudesk-uinput.rules /etc/modules-load.d/kaddudesk.conf
  UHOME="$(getent passwd "$USER_NAME" | cut -d: -f6)"
  rm -f "$UHOME/.config/autostart/kaddudesk.desktop"
  udevadm control --reload-rules || true
  echo "KadduDesk removed. Settings stay in ~/.config/kaddudesk (delete it to forget everything)."
  exit 0
fi

echo "== Installing dependencies (screen capture, clipboard, dialogs)"
if command -v apt-get >/dev/null; then apt-get update -qq && apt-get install -y ffmpeg xclip wl-clipboard zenity xdotool x11-xserver-utils libcap2-bin || true
elif command -v dnf >/dev/null; then dnf install -y ffmpeg xclip wl-clipboard zenity xdotool xrandr libcap || echo "(Fedora: enable RPM Fusion for full ffmpeg)"
elif command -v pacman >/dev/null; then pacman -S --needed --noconfirm ffmpeg xclip wl-clipboard zenity xdotool xorg-xrandr libcap || true
elif command -v zypper >/dev/null; then zypper install -y ffmpeg xclip wl-clipboard zenity xdotool xrandr libcap || true
fi

echo "== Installing the app to $DEST"
rm -rf "$DEST"; mkdir -p "$DEST"
cp -a "$HERE/." "$DEST/"
chown -R root:root "$DEST"
chmod 4755 "$DEST/chrome-sandbox"      # Chromium sandbox helper must be setuid root
chmod 0755 "$DEST/resources/core/kaddudesk-server" 2>/dev/null || true
ln -sf "$DEST/kaddudesk-client" /usr/local/bin/kaddudesk
install -Dm 0644 "$HERE/kaddudesk.png" /usr/share/icons/hicolor/256x256/apps/kaddudesk-client.png
cat > /usr/share/applications/kaddudesk.desktop <<EOF
[Desktop Entry]
Type=Application
Name=KadduDesk
Comment=Control your devices, and share this one
Exec=$DEST/kaddudesk-client %U
Icon=kaddudesk-client
Terminal=false
Categories=Network;RemoteAccess;
StartupWMClass=KadduDesk
EOF

echo "== Allowing remote mouse/keyboard (uinput)"
echo uinput > /etc/modules-load.d/kaddudesk.conf
modprobe uinput || true
cat > /etc/udev/rules.d/60-kaddudesk-uinput.rules <<'EOF'
KERNEL=="uinput", SUBSYSTEM=="misc", MODE="0660", GROUP="input", TAG+="uaccess", OPTIONS+="static_node=uinput"
EOF
udevadm control --reload-rules && udevadm trigger --name-match=uinput || true
getent group input >/dev/null && usermod -aG input "$USER_NAME" || true

echo
echo "== Wayland screen capture (optional)"
echo "On Wayland (default on Ubuntu 22.04+ / Fedora) sharing this screen needs KMS capture:"
echo "a private copy of ffmpeg with CAP_SYS_ADMIN, runnable only by '$USER_NAME' (Intel/AMD graphics)."
read -r -p "Enable KMS capture? [y/N] " ans || ans=n
if [[ "$ans" =~ ^[Yy]$ ]] && command -v ffmpeg >/dev/null; then
  mkdir -p /opt/kaddudesk
  install -m 0750 -o root -g "$(id -gn "$USER_NAME")" "$(command -v ffmpeg)" /opt/kaddudesk/ffmpeg-kms
  setcap cap_sys_admin+ep /opt/kaddudesk/ffmpeg-kms && echo "   KMS capture enabled."
fi

echo "== Firewall"
if command -v ufw >/dev/null && ufw status | grep -q "Status: active"; then
  ufw allow 47800/tcp comment 'KadduDesk' && ufw allow 47810/udp comment 'KadduDesk discovery' && ufw allow 47820/tcp comment 'KadduDesk hub'
elif command -v firewall-cmd >/dev/null && firewall-cmd --state >/dev/null 2>&1; then
  firewall-cmd --permanent --add-port=47800/tcp --add-port=47810/udp --add-port=47820/tcp && firewall-cmd --reload
fi

echo
echo "Done! Log out and back in once, then open KadduDesk from your app menu (or run: kaddudesk)."
echo "This computer is shared while KadduDesk is open; see the app's Guide tab for everything else."
