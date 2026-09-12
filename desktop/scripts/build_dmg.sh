#!/usr/bin/env bash
#
# Wrap dist/Clarity.app into dist/Clarity.dmg — the thing a user downloads.
# P4_DESKTOP Sprint 4 Step 3, FRD §15.2, SETUP §11.3.
#
#   cd desktop && ./scripts/build_dmg.sh
#
# create-dmg drives Finder over AppleScript to lay the window out, so the first
# run raises "Terminal wants to control Finder" — allow it or the icons land
# wherever Finder feels like.
#
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

APP_NAME="Clarity"
APP="dist/${APP_NAME}.app"
DMG="dist/${APP_NAME}.dmg"
BACKGROUND="assets/dmg_background.png"

# Icon centres, in points, inside the window. make_dmg_background.py draws the
# arrow between them — keep the two in step.
WINDOW_W=600
WINDOW_H=400
ICON_SIZE=128
APP_X=150
DROP_X=450
ICON_Y=200

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
die() { printf '\033[31merror: %s\033[0m\n' "$*" >&2; exit 1; }

command -v create-dmg >/dev/null || die "create-dmg is missing. brew install create-dmg"
[[ -d "${APP}" ]] || die "${APP} does not exist. ./scripts/build_app.sh first"

if [[ ! -f "${BACKGROUND}" ]]; then
	say "drawing ${BACKGROUND}"
	python scripts/make_dmg_background.py
fi

# create-dmg cds into whatever it is given and makes *that* the root of the
# volume, so handing it the .app would put Contents/ at the top level. It gets a
# directory holding nothing but the app. ditto, not cp, because copying an .app
# any other way can drop the code signature.
STAGING="dist/dmg-staging"
say "staging"
rm -rf "${STAGING}"
mkdir -p "${STAGING}"
ditto "${APP}" "${STAGING}/${APP_NAME}.app"

rm -f "${DMG}"
say "building ${DMG}"
create-dmg \
	--volname "${APP_NAME}" \
	--volicon assets/icon.icns \
	--background "${BACKGROUND}" \
	--window-pos 200 120 \
	--window-size "${WINDOW_W}" "${WINDOW_H}" \
	--icon-size "${ICON_SIZE}" \
	--icon "${APP_NAME}.app" "${APP_X}" "${ICON_Y}" \
	--app-drop-link "${DROP_X}" "${ICON_Y}" \
	--hide-extension "${APP_NAME}.app" \
	--no-internet-enable \
	"${DMG}" "${STAGING}"

rm -rf "${STAGING}"

# Mount it and check the app inside, because a signature that survived the build
# but not the copy would only show up on the machine we hand this to.
say "verifying the copy inside the image"
MOUNT="$(mktemp -d)"
hdiutil attach "${DMG}" -mountpoint "${MOUNT}" -nobrowse -readonly -quiet
trap 'hdiutil detach "${MOUNT}" -quiet >/dev/null 2>&1 || true; rmdir "${MOUNT}" 2>/dev/null || true' EXIT
[[ -d "${MOUNT}/${APP_NAME}.app" ]] || die "${APP_NAME}.app is not at the root of the image"
if codesign --verify --deep --strict "${MOUNT}/${APP_NAME}.app" 2>/dev/null; then
	echo "  signature intact"
else
	printf '\033[33m  unsigned or broken signature inside the image\033[0m\n'
fi

say "done"
du -sh "${DMG}" | awk '{print "  " $2 "  " $1}'
cat <<EOF

  open ${DMG}                 the layout should match the background art
  Hand dist/${APP_NAME}.app and ${DMG} to P3 for the .pkg and the release.
EOF
