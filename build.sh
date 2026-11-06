#!/usr/bin/env bash
# Builds everything into dist/:
#   KadduDeskServer.exe   – host server (runs as admin; needs ffmpeg.exe next to it, or embedded)
#   KadduDesk-*.exe       – Windows client (portable + installer)
#   KadduDesk.apk         – Android client
#   kaddudesk-server-linux-*.tar.gz – Linux host server (x64, arm64) + installer
#   KadduDesk-*.AppImage    – Linux client
# Usage: ./build.sh [server|linux|android|windows|linux-client|all]   (run from Git Bash)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"
WHAT="${1:-all}"
mkdir -p dist

sync_shared() {
  : # (2.0: the app UI is embedded in the core; nothing to copy)
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

build_linux() {
  echo "== linux server"
  for arch in amd64 arm64; do
    local d="build/linux-$arch/kaddudesk-server"
    rm -rf "build/linux-$arch"; mkdir -p "$d"
    (cd server && GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "../$d/kaddudesk-server" .)
    cp linux/install-linux.sh "$d/"
    cp build/icon.png "$d/kaddudesk.png"
    local name=$arch; [ "$arch" = amd64 ] && name=x64
    python -c "
import tarfile,sys,os
src,out=sys.argv[1],sys.argv[2]
with tarfile.open(out,'w:gz') as t:
    for f in sorted(os.listdir(src)):
        ti=t.gettarinfo(os.path.join(src,f),'kaddudesk-server/'+f)
        ti.mode=0o755 if f.endswith(('.sh','server')) else 0o644
        ti.uid=ti.gid=0; ti.uname=ti.gname='root'
        t.addfile(ti,open(os.path.join(src,f),'rb'))
" "$d" "dist/kaddudesk-server-linux-$name.tar.gz"
  done
  ls -la dist/kaddudesk-server-linux-*.tar.gz
}

build_linux_client() {
  echo "== linux app"
  [ -f build/linux-amd64/kaddudesk-server/kaddudesk-server ] || build_linux
  rm -rf build/core-linux; mkdir -p build/core-linux
  cp build/linux-amd64/kaddudesk-server/kaddudesk-server build/core-linux/
  # AppImage needs symlinks (not allowed on Windows without admin) -> package the app dir ourselves
  npx electron-builder --linux dir --x64
  cp linux/client/kaddudesk-client linux/client/install-client.sh dist/electron/linux-unpacked/
  cp build/icon.png dist/electron/linux-unpacked/kaddudesk.png
  python -c "
import tarfile,sys,os
src,out,root=sys.argv[1],sys.argv[2],'kaddudesk-client-linux-x64'
exe={'kaddudesk','chrome_crashpad_handler','kaddudesk-client','install-client.sh','chrome-sandbox','kaddudesk-server'}
with tarfile.open(out,'w:gz') as t:
    for dp,dn,fn in os.walk(src):
        dn.sort()
        for n in sorted(dn)+sorted(fn):
            full=os.path.join(dp,n); rel=os.path.relpath(full,src).replace(os.sep,'/')
            ti=t.gettarinfo(full,root+'/'+rel); ti.uid=ti.gid=0; ti.uname=ti.gname='root'
            if ti.isdir(): ti.mode=0o755; t.addfile(ti)
            else:
                ti.mode=0o755 if (n in exe or n.endswith('.so') or '.so.' in n) else 0o644
                t.addfile(ti,open(full,'rb'))
" dist/electron/linux-unpacked dist/kaddudesk-client-linux-x64.tar.gz
  ls -la dist/kaddudesk-client-linux-x64.tar.gz
}

build_android() {
  echo "== android"
  SDK="${ANDROID_HOME:-$LOCALAPPDATA/Android/Sdk}"
  BT="$(ls -d "$SDK"/build-tools/* | sort -V | tail -1)"
  PLAT="$(ls -d "$SDK"/platforms/android-* | sort -V | tail -1)/android.jar"
  # prefer platform 36 if present (stable)
  [ -f "$SDK/platforms/android-36/android.jar" ] && PLAT="$SDK/platforms/android-36/android.jar"
  B="build/android"
  rm -rf "$B"; mkdir -p "$B/classes" "$B/dex" "$B/gen" "$B/lib/arm64-v8a" "$B/lib/x86_64"
  # the KadduDesk core as a native executable: arm64 for phones, x86_64 for emulators/ChromeOS
  (cd server && GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -tags kdandroid -trimpath -ldflags "-s -w" -o "../$B/lib/arm64-v8a/libkdcore.so" .)
  (cd server && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags kdandroid -trimpath -ldflags "-s -w" -o "../$B/lib/x86_64/libkdcore.so" .)
  "$BT/aapt2" compile --dir android/res -o "$B/res.zip"
  "$BT/aapt2" link -o "$B/base.apk" -I "$PLAT" --manifest android/AndroidManifest.xml \
     --min-sdk-version 24 --target-sdk-version 35 ${KD_ANDROID_DEBUG:+--debug-mode} --java "$B/gen" "$B/res.zip"
  javac --release 11 -nowarn -encoding UTF-8 -classpath "$PLAT" -d "$B/classes" \
     $(find android/src -name '*.java') $(find "$B/gen" -name '*.java')
  "$BT/d8.bat" --release --min-api 24 --lib "$PLAT" --output "$B/dex" $(find "$B/classes" -name '*.class')
  python -c "import zipfile,sys; z=zipfile.ZipFile(sys.argv[1],'a',zipfile.ZIP_DEFLATED); z.write(sys.argv[2],'classes.dex'); [z.write(sys.argv[3]+'/'+a+'/libkdcore.so','lib/'+a+'/libkdcore.so') for a in ('arm64-v8a','x86_64')]; z.close()" "$B/base.apk" "$B/dex/classes.dex" "$B/lib"
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
  echo "== windows app"
  # the app bundles the KadduDesk core (the same exe that shares this PC)
  [ -f dist/KadduDeskServer.exe ] || build_server
  rm -rf build/core-win; mkdir -p build/core-win
  cp dist/KadduDeskServer.exe build/core-win/
  npx electron-builder --win portable nsis
  cp dist/electron/KadduDesk-*-portable.exe dist/ 2>/dev/null || true
  cp dist/electron/KadduDesk-*-setup.exe dist/ 2>/dev/null || true
  ls -la dist/*.exe
}

sync_shared
case "$WHAT" in
  server) build_server ;;
  linux) build_linux ;;
  linux-client) build_linux_client ;;
  android) build_android ;;
  windows) build_windows ;;
  all) build_server; build_linux; build_android; build_windows; build_linux_client ;;
  *) echo "unknown target $WHAT"; exit 1 ;;
esac
