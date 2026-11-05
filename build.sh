#!/usr/bin/env bash
# Builds everything into dist/:
#   KadduDeskServer.exe   – host server (runs as admin; needs ffmpeg.exe next to it, or embedded)
#   KadduDesk-*.exe       – Windows client (portable + installer)
#   KadduDesk.apk         – Android client
# Usage: ./build.sh [server|android|windows|all]   (run from Git Bash)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"
WHAT="${1:-all}"
mkdir -p dist

sync_shared() {
  # the launcher reuses the web client's stylesheet and native bridge
  cp server/web/style.css server/web/bridge.js clients/launcher/
  cp server/web/icon.png clients/launcher/icon.png
}

fetch_ffmpeg() {
  # ffmpeg is not stored in git (170 MB). Fetch the official BtbN Windows build once.
  if [ -z "$(find third_party/ffmpeg -name ffmpeg.exe 2>/dev/null | head -1)" ]; then
    echo "   downloading ffmpeg (BtbN win64 gpl build)…"
    mkdir -p third_party/ffmpeg
    curl -fL -o third_party/ffmpeg/ffmpeg.zip https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip
    (cd third_party/ffmpeg && unzip -q -o ffmpeg.zip && rm -f ffmpeg.zip)
  fi
}

build_server() {
  echo "== server"
  [ "${EMBED_FFMPEG:-1}" = "1" ] && fetch_ffmpeg
  go run tools/mkres/main.go -ico server/assets/icon.ico -manifest server/winres/server.manifest -o server/rsrc_windows_amd64.syso
  FF="$(find third_party/ffmpeg -name ffmpeg.exe 2>/dev/null | head -1 || true)"
  local tags=""
  if [ -n "$FF" ] && [ "${EMBED_FFMPEG:-1}" = "1" ]; then
    mkdir -p server/bin
    if [ ! -f server/bin/ffmpeg.exe.gz ] || [ "$FF" -nt server/bin/ffmpeg.exe.gz ]; then
      python -c "import gzip,shutil,sys; shutil.copyfileobj(open(sys.argv[1],'rb'), gzip.open(sys.argv[2],'wb',6))" "$FF" server/bin/ffmpeg.exe.gz
    fi
    tags="-tags embedffmpeg"
    echo "   embedding $FF"
  fi
  (cd server && go build $tags -trimpath -ldflags "-s -w -H windowsgui" -o ../dist/KadduDeskServer.exe .)
  ls -la dist/KadduDeskServer.exe
}

build_android() {
  echo "== android"
  SDK="${ANDROID_HOME:-$LOCALAPPDATA/Android/Sdk}"
  BT="$(ls -d "$SDK"/build-tools/* | sort -V | tail -1)"
  PLAT="$(ls -d "$SDK"/platforms/android-* | sort -V | tail -1)/android.jar"
  # prefer platform 36 if present (stable)
  [ -f "$SDK/platforms/android-36/android.jar" ] && PLAT="$SDK/platforms/android-36/android.jar"
  B="build/android"
  rm -rf "$B"; mkdir -p "$B/assets/launcher" "$B/classes" "$B/dex" "$B/gen"
  cp clients/launcher/index.html clients/launcher/style.css clients/launcher/bridge.js clients/launcher/icon.png "$B/assets/launcher/"
  "$BT/aapt2" compile --dir android/res -o "$B/res.zip"
  "$BT/aapt2" link -o "$B/base.apk" -I "$PLAT" --manifest android/AndroidManifest.xml -A "$B/assets" \
     --min-sdk-version 24 --target-sdk-version 35 --version-code 1 --version-name 1.0.0 --java "$B/gen" "$B/res.zip"
  javac --release 11 -nowarn -encoding UTF-8 -classpath "$PLAT" -d "$B/classes" \
     $(find android/src -name '*.java') $(find "$B/gen" -name '*.java')
  "$BT/d8.bat" --release --min-api 24 --lib "$PLAT" --output "$B/dex" $(find "$B/classes" -name '*.class')
  python -c "import zipfile,sys; z=zipfile.ZipFile(sys.argv[1],'a',zipfile.ZIP_DEFLATED); z.write(sys.argv[2],'classes.dex'); z.close()" "$B/base.apk" "$B/dex/classes.dex"
  "$BT/zipalign.exe" -p -f 4 "$B/base.apk" "$B/aligned.apk"
  KEYTOOL="$(command -v keytool || true)"
  if [ -z "$KEYTOOL" ]; then
    for d in "${JAVA_HOME:-/nonexistent}/bin" "/c/Program Files/Java"/jdk*/bin "/c/Program Files/Android/Android Studio/jbr/bin"; do
      [ -x "$d/keytool.exe" ] && KEYTOOL="$d/keytool.exe" && break
    done
  fi
  KS="android/kaddudesk.keystore"; KP="android/keystore.pass"
  if [ ! -f "$KS" ]; then
    python -c "import secrets;print(secrets.token_hex(16))" > "$KP"
    "$KEYTOOL" -genkeypair -keystore "$KS" -alias kaddudesk -keyalg RSA -keysize 2048 -validity 10000 \
      -storepass "$(cat "$KP")" -keypass "$(cat "$KP")" -dname "CN=KadduDesk, O=KadduDesk" -noprompt
  fi
  "$BT/apksigner.bat" sign --ks "$KS" --ks-pass "pass:$(cat "$KP")" --key-pass "pass:$(cat "$KP")" --ks-key-alias kaddudesk \
     --out dist/KadduDesk.apk "$B/aligned.apk"
  "$BT/apksigner.bat" verify dist/KadduDesk.apk && ls -la dist/KadduDesk.apk
}

build_windows() {
  echo "== windows client"
  npx electron-builder --win portable nsis
  cp dist/electron/KadduDesk-*-portable.exe dist/ 2>/dev/null || true
  cp dist/electron/KadduDesk-*-setup.exe dist/ 2>/dev/null || true
  ls -la dist/*.exe
}

sync_shared
case "$WHAT" in
  server) build_server ;;
  android) build_android ;;
  windows) build_windows ;;
  all) build_server; build_android; build_windows ;;
  *) echo "unknown target $WHAT"; exit 1 ;;
esac
