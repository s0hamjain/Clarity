#!/usr/bin/env bash
#
# Build Clarity.pkg from the .app P4 hands over.
#
# The .pkg exists for one thing the .dmg cannot do: start Clarity at login. It
# installs the same bundle to /Applications and runs scripts/postinstall, which
# writes a LaunchAgent for the user who is installing.
#
#   ./release/build_pkg.sh [path/to/Clarity.app]
#
# Defaults to desktop/dist/Clarity.app (SETUP §11.2's output).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${1:-$REPO_ROOT/desktop/dist/Clarity.app}"
DIST="$REPO_ROOT/desktop/dist"
IDENTIFIER="com.clarity.app"
VERSION="0.1.0"

if [[ ! -d "$APP" ]]; then
  cat >&2 <<EOF
error: no app bundle at $APP

P4 builds it with:
    cd desktop && source .venv/bin/activate && ./scripts/build_app.sh

Or pass one explicitly:
    ./release/build_pkg.sh /path/to/Clarity.app
EOF
  exit 1
fi

# pkgbuild copies whatever it is given, including a stale nested dist/. Refuse
# an obviously wrong bundle rather than shipping it.
if [[ ! -x "$APP/Contents/MacOS/Clarity" ]]; then
  echo "error: $APP has no executable at Contents/MacOS/Clarity" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
# pkgbuild --root copies *everything* under the root it is given, so the bundle
# is staged alone in an otherwise empty tree and the component package is
# written outside it — put it inside and pkgbuild packages its own output.
STAGE="$WORK/root"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/Clarity.app"

mkdir -p "$DIST"
COMPONENT="$WORK/Clarity-component.pkg"

echo "==> pkgbuild"
pkgbuild \
  --root "$STAGE" \
  --install-location /Applications \
  --scripts "$REPO_ROOT/release/scripts" \
  --identifier "$IDENTIFIER" \
  --version "$VERSION" \
  "$COMPONENT"

echo "==> productbuild"
productbuild --package "$COMPONENT" "$DIST/Clarity.pkg"

echo
echo "built: $DIST/Clarity.pkg"
echo "install with:  sudo installer -pkg $DIST/Clarity.pkg -target /"
echo "or double-click it. Log out and back in; the menu bar icon should return on its own."
