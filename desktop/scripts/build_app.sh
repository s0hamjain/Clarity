#!/usr/bin/env bash
#
# Build dist/Clarity.app.  P4_DESKTOP Sprint 4 Step 2, FRD §15.2, SETUP §11.2.
#
#   cd desktop && source .venv/bin/activate && ./scripts/build_app.sh
#
# PyInstaller (one directory, wrapped in an .app), then LSUIElement so there is
# no Dock icon, then a code signature — which is the whole point of the step:
# macOS keys Screen Recording to the signature, so an unsigned build asks for
# permission again on every rebuild.
#
# Identity: $CODESIGN_IDENTITY, else "Clarity Dev" (SETUP §11.1), else the only
# codesigning identity in the keychain. Nothing suitable → the build finishes
# unsigned and says so.
#
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

APP_NAME="Clarity"
BUNDLE_ID="com.clarity.app"   # same as P3's LaunchAgent label (release/build_pkg.sh)
APP="dist/${APP_NAME}.app"
PLIST="${APP}/Contents/Info.plist"

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
die() { printf '\033[31merror: %s\033[0m\n' "$*" >&2; exit 1; }

# -- the environment --------------------------------------------------------

if [[ -z "${VIRTUAL_ENV:-}" && -f .venv/bin/activate ]]; then
	# shellcheck disable=SC1091
	source .venv/bin/activate
fi
python -c "import PyInstaller" 2>/dev/null \
	|| die "PyInstaller is missing. python3 -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt"

VERSION="$(python -c 'import clarity; print(clarity.__version__)')"
say "building ${APP_NAME} ${VERSION}"

# -- pyinstaller ------------------------------------------------------------
#
# Three things that are only wrong inside the bundle:
#   * clarity/ui and assets/ are found relative to the package, so both have to
#     be added as data under the names the code expects.
#   * pywebview picks its backend by name at runtime, and the bundled hook only
#     collects its JavaScript on Windows — without --collect-data every window
#     opens blank.
#   * clarity/__main__.py is compiled as __main__ with no parent package, which
#     is why its imports are absolute and --paths . is here.

rm -rf build "dist/${APP_NAME}" "${APP}"

pyinstaller \
	--noconfirm --clean --windowed \
	--name "${APP_NAME}" \
	--icon assets/icon.icns \
	--osx-bundle-identifier "${BUNDLE_ID}" \
	--paths . \
	--add-data "clarity/ui:clarity/ui" \
	--add-data "assets:assets" \
	--collect-data webview \
	--hidden-import rumps \
	--hidden-import webview \
	--hidden-import webview.platforms.cocoa \
	--hidden-import pynput.keyboard._darwin \
	--hidden-import pynput.mouse._darwin \
	--exclude-module tkinter \
	clarity/__main__.py

[[ -d "${APP}" ]] || die "pyinstaller produced no ${APP}"

# -- Info.plist -------------------------------------------------------------

plist_set() { # key type value
	/usr/libexec/PlistBuddy -c "Set :$1 $3" "${PLIST}" >/dev/null 2>&1 \
		|| /usr/libexec/PlistBuddy -c "Add :$1 $2 $3" "${PLIST}" >/dev/null
}

say "patching Info.plist"
plist_set LSUIElement bool true            # menu bar only, no Dock icon (FRD §15.1)
plist_set CFBundleShortVersionString string "${VERSION}"
plist_set CFBundleVersion string "${VERSION}"

# -- signature --------------------------------------------------------------

identities() {
	security find-identity -v -p codesigning | sed -n 's/^ *[0-9]*) [0-9A-F]* "\(.*\)"$/\1/p'
}

resolve_identity() {
	if [[ -n "${CODESIGN_IDENTITY:-}" ]]; then
		echo "${CODESIGN_IDENTITY}"
		return
	fi
	local all
	all="$(identities || true)"
	if grep -qx "Clarity Dev" <<<"${all}"; then
		echo "Clarity Dev"
	elif [[ "$(grep -c . <<<"${all}")" == "1" && -n "${all}" ]]; then
		echo "${all}"
	fi
}

IDENTITY="$(resolve_identity)"
if [[ -z "${IDENTITY}" ]]; then
	printf '\033[33mwarning: no code signing identity found — %s is unsigned.\033[0m\n' "${APP}"
	printf '         macOS will ask for Screen Recording again after every rebuild.\n'
	printf '         Create one: SETUP §11.1, or set CODESIGN_IDENTITY.\n'
else
	say "signing as ${IDENTITY}"
	# --deep is deprecated but it is the only one-shot way to cover the several
	# hundred .so files PyInstaller collects. Verified below, so a partial sign
	# fails the build rather than shipping.
	codesign --force --deep --sign "${IDENTITY}" "${APP}"
	codesign --verify --deep --strict "${APP}"
	codesign --display --verbose=2 "${APP}" 2>&1 | grep -E '^(Identifier|Authority|TeamIdentifier)' || true
fi

say "done"
du -sh "${APP}" | awk '{print "  " $2 "  " $1}'
cat <<'EOF'

  open dist/Clarity.app         menu bar icon, no Dock icon
  ⌘⇧E                           the spotlight box must still be translucent
                                inside the bundle — it can differ from
                                `python -m clarity` (FRD §24)
  ./scripts/build_dmg.sh        wrap it for the release
EOF
