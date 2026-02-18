#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
REPO_ROOT="$(cd "$ROOT_DIR/../.." && pwd)"
APP_NAME="OpencodeBot"
BUNDLE_NAME="opencode-bot"
BUILD_DIR="$ROOT_DIR/.build/debug"
DIST_DIR="$ROOT_DIR/dist"
APP_DIR="$DIST_DIR/$BUNDLE_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
PLIST_PATH="$CONTENTS_DIR/Info.plist"
ICON_NAME="TrayBridge"
ICON_ICNS_PATH="$RESOURCES_DIR/$ICON_NAME.icns"
EMBEDDED_SERVER_SRC="$ROOT_DIR/embedded-server"
VERSION_FILE="$REPO_ROOT/VERSION"

APP_VERSION="1.0.0"
if [ -f "$VERSION_FILE" ]; then
  CANDIDATE_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"
  if [ -n "$CANDIDATE_VERSION" ]; then
    APP_VERSION="$CANDIDATE_VERSION"
  fi
fi

generate_icon() {
  local temp_dir
  temp_dir="$(mktemp -d)"
  local png_1024="$temp_dir/icon_1024.png"
  local iconset_dir="$temp_dir/$ICON_NAME.iconset"

  cat > "$temp_dir/generate_icon.swift" <<'SWIFT'
import AppKit
import Foundation

let outputPath = CommandLine.arguments[1]
let size = CGSize(width: 1024, height: 1024)

let image = NSImage(size: size)
image.lockFocus()

NSColor(calibratedRed: 0.08, green: 0.17, blue: 0.36, alpha: 1.0).setFill()
NSBezierPath(roundedRect: CGRect(origin: .zero, size: size), xRadius: 220, yRadius: 220).fill()

let attributes: [NSAttributedString.Key: Any] = [
  .font: NSFont.systemFont(ofSize: 560, weight: .black),
  .foregroundColor: NSColor(calibratedRed: 0.95, green: 0.98, blue: 1.0, alpha: 1.0)
]
let text = NSString(string: "⚡")
let textSize = text.size(withAttributes: attributes)
let textRect = CGRect(
  x: (size.width - textSize.width) / 2,
  y: (size.height - textSize.height) / 2 - 20,
  width: textSize.width,
  height: textSize.height
)
text.draw(in: textRect, withAttributes: attributes)

image.unlockFocus()

guard
  let tiff = image.tiffRepresentation,
  let bitmap = NSBitmapImageRep(data: tiff),
  let pngData = bitmap.representation(using: .png, properties: [:])
else {
  fputs("Failed to render icon image\n", stderr)
  exit(1)
}

try pngData.write(to: URL(fileURLWithPath: outputPath))
SWIFT

  swift "$temp_dir/generate_icon.swift" "$png_1024"

  mkdir -p "$iconset_dir"
  sips -z 16 16 "$png_1024" --out "$iconset_dir/icon_16x16.png" >/dev/null
  sips -z 32 32 "$png_1024" --out "$iconset_dir/icon_16x16@2x.png" >/dev/null
  sips -z 32 32 "$png_1024" --out "$iconset_dir/icon_32x32.png" >/dev/null
  sips -z 64 64 "$png_1024" --out "$iconset_dir/icon_32x32@2x.png" >/dev/null
  sips -z 128 128 "$png_1024" --out "$iconset_dir/icon_128x128.png" >/dev/null
  sips -z 256 256 "$png_1024" --out "$iconset_dir/icon_128x128@2x.png" >/dev/null
  sips -z 256 256 "$png_1024" --out "$iconset_dir/icon_256x256.png" >/dev/null
  sips -z 512 512 "$png_1024" --out "$iconset_dir/icon_256x256@2x.png" >/dev/null
  sips -z 512 512 "$png_1024" --out "$iconset_dir/icon_512x512.png" >/dev/null
  cp "$png_1024" "$iconset_dir/icon_512x512@2x.png"

  iconutil -c icns "$iconset_dir" -o "$ICON_ICNS_PATH"
  rm -rf "$temp_dir"
}

sign_app_bundle() {
  local sign_identity="${APPLE_SIGN_IDENTITY:--}"
  echo "Signing app bundle with identity: $sign_identity"
  codesign --force --deep --sign "$sign_identity" "$APP_DIR"
}

copy_embedded_runtime() {
  if [ -d "$EMBEDDED_SERVER_SRC" ]; then
    echo "Embedding server payload..."
    rm -rf "$RESOURCES_DIR/server"
    cp -R "$EMBEDDED_SERVER_SRC" "$RESOURCES_DIR/server"
  else
    echo "Embedded server payload missing. Run scripts/prepare-embedded-server.sh first."
  fi

}

echo "Building Swift app..."
swift build --package-path "$ROOT_DIR"

echo "Packaging .app bundle..."
rm -rf "$APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

cp "$BUILD_DIR/$APP_NAME" "$MACOS_DIR/$APP_NAME"
chmod +x "$MACOS_DIR/$APP_NAME"
generate_icon
copy_embedded_runtime

if [ ! -d "$RESOURCES_DIR/server" ]; then
  echo "ERROR: Missing embedded server payload."
  echo "Run prepare-embedded-server.sh before packaging."
  exit 1
fi

cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>opencode-bot</string>
  <key>CFBundleDisplayName</key>
  <string>opencode-bot</string>
  <key>CFBundleExecutable</key>
  <string>OpencodeBot</string>
  <key>CFBundleIdentifier</key>
  <string>labs.hanami.opencode-bot</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleIconFile</key>
  <string>TrayBridge</string>
  <key>CFBundleGetInfoString</key>
  <string>Menu bar app to control opencode-bot bridge service.</string>
  <key>CFBundleVersion</key>
  <string>$APP_VERSION</string>
  <key>CFBundleShortVersionString</key>
  <string>$APP_VERSION</string>
  <key>LSMinimumSystemVersion</key>
  <string>14.0</string>
  <key>LSUIElement</key>
  <false/>
</dict>
</plist>
EOF

sign_app_bundle

echo "Done: $APP_DIR"
echo "Open with: open \"$APP_DIR\""
echo "If macOS blocks startup, run: xattr -dr com.apple.quarantine \"$APP_DIR\""
