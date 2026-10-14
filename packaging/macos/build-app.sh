#!/bin/sh
set -eu

cd "$(dirname "$0")/../.."

APP_NAME="CueDeck"
BUNDLE="dist/$APP_NAME.app"
ICONSET="dist/$APP_NAME.iconset"

if ! command -v iconutil >/dev/null 2>&1; then
	echo "iconutil not found: this script builds a macOS .app and must run on macOS" >&2
	exit 1
fi

rm -rf "$BUNDLE" "$ICONSET"
mkdir -p "$BUNDLE/Contents/MacOS" "$BUNDLE/Contents/Resources"

echo "building $BUNDLE"
go build -trimpath -ldflags "-s -w" -o "$BUNDLE/Contents/MacOS/$APP_NAME" .

echo "drawing the icon"
go run ./tools/icon -iconset "$ICONSET"
iconutil --convert icns --output "$BUNDLE/Contents/Resources/$APP_NAME.icns" "$ICONSET"
rm -rf "$ICONSET"

cp packaging/macos/Info.plist "$BUNDLE/Contents/Info.plist"

if codesign --force --sign - "$BUNDLE" 2>/dev/null; then
	echo "signed ad-hoc"
else
	echo "note: ad-hoc signing failed, the app will still run locally"
fi

echo
echo "done. Run it with:"
echo "  open $BUNDLE"
echo "or drag it to /Applications so the Dock, Cmd+Tab and Spotlight keep the icon."
