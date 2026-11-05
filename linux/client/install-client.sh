#!/usr/bin/env bash
# Installs the KadduDesk client to /opt/kaddudesk-client and adds it to the app menu.
#   sudo ./install-client.sh            install / update
#   sudo ./install-client.sh --uninstall
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
DEST=/opt/kaddudesk-client
[ "$(id -u)" -eq 0 ] || { echo "Please run with sudo:  sudo $0 $*"; exit 1; }

if [ "${1:-}" = "--uninstall" ]; then
  rm -rf "$DEST" /usr/local/bin/kaddudesk /usr/share/applications/kaddudesk.desktop \
         /usr/share/icons/hicolor/256x256/apps/kaddudesk-client.png
  echo "KadduDesk client removed."; exit 0
fi

rm -rf "$DEST"; mkdir -p "$DEST"
cp -a "$HERE/." "$DEST/"
chown -R root:root "$DEST"
chmod 4755 "$DEST/chrome-sandbox"      # Chromium sandbox helper must be setuid root
ln -sf "$DEST/kaddudesk-client" /usr/local/bin/kaddudesk
install -Dm 0644 "$HERE/kaddudesk.png" /usr/share/icons/hicolor/256x256/apps/kaddudesk-client.png
cat > /usr/share/applications/kaddudesk.desktop <<EOF
[Desktop Entry]
Type=Application
Name=KadduDesk
Comment=Control your computers remotely
Exec=$DEST/kaddudesk-client %U
Icon=kaddudesk-client
Terminal=false
Categories=Network;RemoteAccess;
StartupWMClass=KadduDesk
EOF
echo "Installed. Start 'KadduDesk' from your app menu, or run: kaddudesk"
