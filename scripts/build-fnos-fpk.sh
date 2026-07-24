#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PKG_DIR="$ROOT_DIR/packaging/fnos/localtwitter"
APP_BIN="$PKG_DIR/app/localtwitter"
DIST_DIR="$ROOT_DIR/dist"
TOOLS_DIR="$ROOT_DIR/.tools/fnpack"
OUT_FPK="$DIST_DIR/localtwitter-x86_64.fpk"
CURRENT_VERSION="$(awk -F= '$1 == "version" { gsub(/[[:space:]]/, "", $2); print $2 }' "$PKG_DIR/manifest")"

bump_patch_version() {
  local version="$1"
  local major minor patch

  IFS=. read -r major minor patch <<EOF
$version
EOF
  patch=$((patch + 1))
  printf '%s.%s.%s' "$major" "$minor" "$patch"
}

NEXT_VERSION="$(bump_patch_version "$CURRENT_VERSION")"
TODAY="$(date +%F)"
CHANGELOG_NOTE="- 默认打包自动提升版本号。"

update_manifest_version() {
  python3 - "$PKG_DIR/manifest" "$NEXT_VERSION" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
version = sys.argv[2]
text = path.read_text()
lines = text.splitlines()
for i, line in enumerate(lines):
    if line.startswith("version="):
        lines[i] = f"version={version}"
        break
else:
    raise SystemExit("version field not found")
path.write_text("\n".join(lines) + "\n")
PY
}

prepend_changelog_entry() {
  local file="$1"
  python3 - "$file" "$NEXT_VERSION" "$TODAY" "$CHANGELOG_NOTE" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
version = sys.argv[2]
today = sys.argv[3]
note = sys.argv[4]
text = path.read_text()
prefix = "# Changelog\n\n"
entry = f"## {version} - {today}\n\n{note}\n\n"
if not text.startswith(prefix):
    raise SystemExit(f"unexpected changelog header in {path}")
if text.startswith(prefix + f"## {version} - {today}\n\n"):
    raise SystemExit(0)
path.write_text(prefix + entry + text[len(prefix):])
PY
}

update_release_notes() {
  update_manifest_version
  prepend_changelog_entry "$ROOT_DIR/CHANGELOG.md"
  prepend_changelog_entry "$PKG_DIR/CHANGELOG.md"
  prepend_changelog_entry "$PKG_DIR/app/CHANGELOG.md"
}

log() {
  printf '[fnos-fpk] %s\n' "$*" >&2
}

fnpack_url() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os:$arch" in
    Darwin:arm64) echo "https://static2.fnnas.com/fnpack/fnpack-1.2.1-darwin-arm64" ;;
    Darwin:x86_64) echo "https://static2.fnnas.com/fnpack/fnpack-1.2.1-darwin-amd64" ;;
    Linux:x86_64) echo "https://static2.fnnas.com/fnpack/fnpack-1.2.1-linux-amd64" ;;
    Linux:aarch64|Linux:arm64) echo "https://static2.fnnas.com/fnpack/fnpack-1.2.1-linux-arm64" ;;
    *) echo "" ;;
  esac
}

resolve_fnpack() {
  if command -v fnpack >/dev/null 2>&1; then
    command -v fnpack
    return
  fi

  local url tool
  url="$(fnpack_url)"
  if [ -z "$url" ]; then
    echo "fnpack is not installed and no download URL is known for $(uname -s)/$(uname -m)" >&2
    exit 1
  fi

  mkdir -p "$TOOLS_DIR"
  tool="$TOOLS_DIR/$(basename "$url")"
  if [ ! -x "$tool" ]; then
    log "downloading fnpack: $url"
    curl -fL "$url" -o "$tool"
    chmod +x "$tool"
  fi
  echo "$tool"
}

require_file() {
  if [ ! -e "$1" ]; then
    echo "required packaging file missing: $1" >&2
    exit 1
  fi
}

log "bumping release version to ${NEXT_VERSION}"
update_release_notes
VERSION="$(awk -F= '$1 == "version" { gsub(/[[:space:]]/, "", $2); print $2 }' "$PKG_DIR/manifest")"
VERSIONED_FPK="$DIST_DIR/localtwitter-v${VERSION}-x86_64.fpk"

log "building embedded web assets"
npm run build --prefix "$ROOT_DIR/web"

log "cross-compiling linux/amd64 binary"
rm -f "$APP_BIN"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$APP_BIN" "$ROOT_DIR/cmd/server"
chmod +x "$APP_BIN"

if command -v file >/dev/null 2>&1; then
  file "$APP_BIN"
  file "$APP_BIN" | grep -Eq 'ELF 64-bit.*x86-64|ELF 64-bit.*x86_64' || {
    echo "compiled binary is not linux x86_64" >&2
    exit 1
  }
fi

log "checking fpk template"
require_file "$PKG_DIR/manifest"
require_file "$PKG_DIR/CHANGELOG.md"
require_file "$PKG_DIR/app/CHANGELOG.md"
require_file "$PKG_DIR/config/privilege"
require_file "$PKG_DIR/config/resource"
require_file "$PKG_DIR/cmd/main"
require_file "$PKG_DIR/cmd/config_callback"
require_file "$PKG_DIR/wizard/install"
require_file "$PKG_DIR/app/ui/config"
require_file "$PKG_DIR/app/ui/images/icon-64.png"
require_file "$PKG_DIR/app/ui/images/icon-256.png"
require_file "$PKG_DIR/app/ui/images/localtwitter-icon-64.png"
require_file "$PKG_DIR/app/ui/images/localtwitter-icon-256.png"
require_file "$PKG_DIR/ICON.PNG"
require_file "$PKG_DIR/ICON_256.PNG"
chmod +x "$PKG_DIR"/cmd/*

mkdir -p "$DIST_DIR"
rm -f "$OUT_FPK" "$VERSIONED_FPK" "$ROOT_DIR/localtwitter.fpk" "$PKG_DIR"/*.fpk

FNPACK="$(resolve_fnpack)"
log "packing fpk with $FNPACK"
(
  cd "$ROOT_DIR"
  "$FNPACK" build --directory "$PKG_DIR"
)

GENERATED=""
if [ -f "$ROOT_DIR/localtwitter.fpk" ]; then
  GENERATED="$ROOT_DIR/localtwitter.fpk"
elif [ -f "$PKG_DIR/localtwitter.fpk" ]; then
  GENERATED="$PKG_DIR/localtwitter.fpk"
fi
if [ -z "$GENERATED" ]; then
  echo "fnpack completed but no .fpk file was found" >&2
  exit 1
fi
mv "$GENERATED" "$VERSIONED_FPK"
cp "$VERSIONED_FPK" "$OUT_FPK"
log "created $VERSIONED_FPK"
log "updated latest alias $OUT_FPK"
