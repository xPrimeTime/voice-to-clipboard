#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

shopt -s nullglob

usage() {
  cat <<USAGE
Usage: scripts/build-linux-bundle.sh [--version <version>] [--output-dir <dir>]

Builds a Linux AMD64 portable bundle with bundled runtime libraries.

Options:
  --version <version>     Version string for artifact name (default: dev-YYYYMMDD)
  --output-dir <dir>      Output directory for release artifacts (default: dist)
  -h, --help              Show this help
USAGE
}

VERSION="dev-$(date +%Y%m%d)"
OUTPUT_DIR="dist"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: required command not found: $1" >&2
    exit 1
  fi
}

require_cmd go
require_cmd patchelf
require_cmd ldd
require_cmd tar
require_cmd sha256sum

STAGE_ROOT="$PROJECT_DIR/build/bundle-linux-amd64"
APP_DIR="$STAGE_ROOT/voice-to-clipboard"
LIB_DIR="$APP_DIR/lib"
BIN_PATH="$APP_DIR/voice-to-clipboard"
ARCHIVE_NAME="voice-to-clipboard-${VERSION}-linux-amd64.tar.gz"
ARCHIVE_PATH="$PROJECT_DIR/$OUTPUT_DIR/$ARCHIVE_NAME"

rm -rf "$STAGE_ROOT"
mkdir -p "$LIB_DIR" "$PROJECT_DIR/$OUTPUT_DIR"

add_search_dir() {
  local d="$1"
  # Use an if-block (not `&& ...`) so a missing dir doesn't return non-zero and
  # trip `set -e` — several defaults below are Debian-only paths absent on
  # Arch/Fedora/etc.
  if [[ -n "$d" && -d "$d" ]]; then
    SEARCH_DIRS+=("$d")
  fi
}

declare -a SEARCH_DIRS=()

if [[ -n "${WHISPER_CT2_LIB_DIR:-}" ]]; then
  add_search_dir "$WHISPER_CT2_LIB_DIR"
fi
if [[ -n "${CT2_LIB_DIR:-}" ]]; then
  add_search_dir "$CT2_LIB_DIR"
fi
if [[ -n "${LD_LIBRARY_PATH:-}" ]]; then
  IFS=':' read -r -a ldpaths <<< "$LD_LIBRARY_PATH"
  for d in "${ldpaths[@]}"; do
    add_search_dir "$d"
  done
fi

add_search_dir "/usr/local/lib"
add_search_dir "/usr/lib"
add_search_dir "/usr/lib64"
add_search_dir "/usr/lib/x86_64-linux-gnu"

if command -v python3 >/dev/null 2>&1; then
  CT2_PY_DIR="$(python3 - <<'PY' 2>/dev/null || true
import os
try:
    import ctranslate2
    print(os.path.dirname(ctranslate2.__file__))
except Exception:
    pass
PY
)"
  if [[ -n "$CT2_PY_DIR" ]]; then
    add_search_dir "$CT2_PY_DIR"
    add_search_dir "$CT2_PY_DIR/lib"
  fi
fi

find_first_match() {
  local pattern="$1"
  local d
  for d in "${SEARCH_DIRS[@]}"; do
    local f
    for f in "$d"/$pattern; do
      if [[ -f "$f" ]]; then
        echo "$f"
        return 0
      fi
    done
  done
  return 1
}

copy_so_family() {
  local source="$1"
  local dir base prefix
  dir="$(dirname "$source")"
  base="$(basename "$source")"
  prefix="${base%%.so*}.so"

  # If the dep already resolves to the bundle dir (recursive collection can hit
  # an already-copied lib), there's nothing to copy and cp would error on a
  # same-file copy under set -e.
  if [[ "$dir" -ef "$LIB_DIR" ]]; then
    return 0
  fi

  local key="$dir/$prefix"
  if [[ -n "${COPIED_FAMILIES[$key]:-}" ]]; then
    return 0
  fi

  local matches=("$dir"/"$prefix"*)
  if [[ ${#matches[@]} -eq 0 ]]; then
    echo "Error: could not find shared object family for $source" >&2
    return 1
  fi

  cp -a "${matches[@]}" "$LIB_DIR/"
  COPIED_FAMILIES[$key]=1
}

should_bundle_dep() {
  local base="$1"
  case "$base" in
    libwhisper_ct2.so*|libctranslate2.so*|libonnxruntime.so*|libmkl*.so*|libdnnl*.so*|libiomp*.so*|libomp.so*|libgomp.so*|libopenblas.so*|libtbb*.so*|libstdc++.so*|libgcc_s.so*)
      # libstdc++/libgcc_s are bundled because the C++ libs (e.g. ctranslate2)
      # need a newer GLIBCXX than older distros ship (Ubuntu 22.04/Debian 12 are
      # too old). Bundling the build host's newer, backward-compatible libstdc++
      # lets the bundle run there too.
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

collect_ldd_paths() {
  local target="$1"
  ldd "$target" | awk '/=> \/.* \(/ {print $3} /^\/.+ \(/ {print $1}'
}

echo "Building Linux bundle ${VERSION}..."
echo "[1/6] Building application binary"
go build -tags novulkan -o "$BIN_PATH" .

WHISPER_LIB="$(find_first_match 'libwhisper_ct2.so*' || true)"
if [[ -z "$WHISPER_LIB" ]]; then
  echo "Error: libwhisper_ct2.so not found. Set WHISPER_CT2_LIB_DIR or install go-whisper-ct2 runtime libs." >&2
  exit 1
fi

echo "[2/6] Collecting runtime libraries"
declare -A COPIED_FAMILIES=()
copy_so_family "$WHISPER_LIB"

while IFS= read -r dep; do
  [[ -z "$dep" ]] && continue
  if should_bundle_dep "$(basename "$dep")"; then
    copy_so_family "$dep"
  fi
done < <(collect_ldd_paths "$BIN_PATH")

# Collect second-order dependencies from bundled libs (MKL/oneDNN/OpenMP stacks).
for so in "$LIB_DIR"/*.so*; do
  [[ -f "$so" ]] || continue
  while IFS= read -r dep; do
    [[ -z "$dep" ]] && continue
    if should_bundle_dep "$(basename "$dep")"; then
      copy_so_family "$dep"
    fi
  done < <(collect_ldd_paths "$so")
done

if ! ls "$LIB_DIR"/libwhisper_ct2.so* >/dev/null 2>&1; then
  echo "Error: bundle is missing libwhisper_ct2.so*" >&2
  exit 1
fi
if ! ls "$LIB_DIR"/libctranslate2.so* >/dev/null 2>&1; then
  echo "Error: bundle is missing libctranslate2.so*" >&2
  exit 1
fi

echo "[3/6] Patching binary RPATH"
patchelf --set-rpath '$ORIGIN/lib' "$BIN_PATH"

# Repoint every bundled library to its siblings ($ORIGIN). Libraries like
# libwhisper_ct2 bake an absolute build-time RPATH (the go-whisper-ct2 .deps
# dirs) for their own deps (onnxruntime/ctranslate2); without this, those
# transitive deps resolve from the build machine and the bundle breaks on a
# clean one. Patch only real ELF files, not the version symlinks.
for so in "$LIB_DIR"/*.so*; do
  [[ -f "$so" && ! -L "$so" ]] || continue
  patchelf --set-rpath '$ORIGIN' "$so" 2>/dev/null || true
done

echo "[4/6] Writing launchers and notices"
cat > "$APP_DIR/run.sh" <<'RUN'
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export LD_LIBRARY_PATH="$SCRIPT_DIR/lib:${LD_LIBRARY_PATH:-}"
exec "$SCRIPT_DIR/voice-to-clipboard" "$@"
RUN
chmod +x "$APP_DIR/run.sh"

cat > "$APP_DIR/run-portable.sh" <<'RUNP'
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export LD_LIBRARY_PATH="$SCRIPT_DIR/lib:${LD_LIBRARY_PATH:-}"
export XDG_CONFIG_HOME="$SCRIPT_DIR/data/config"
export XDG_CACHE_HOME="$SCRIPT_DIR/data/cache"
export XDG_DATA_HOME="$SCRIPT_DIR/data/share"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$XDG_DATA_HOME"
exec "$SCRIPT_DIR/voice-to-clipboard" "$@"
RUNP
chmod +x "$APP_DIR/run-portable.sh"

cat > "$APP_DIR/THIRD_PARTY_NOTICES.txt" <<'NOTICES'
This bundle redistributes third-party runtime libraries.

Primary dependencies:
- CTranslate2: https://github.com/OpenNMT/CTranslate2
- go-whisper-ct2 runtime library: https://github.com/xPrimeTime/go-whisper-ct2
- oneAPI MKL/OpenMP or equivalent backend libraries (as packaged with CTranslate2)

You are responsible for reviewing and complying with each dependency's license terms.
NOTICES

cat > "$APP_DIR/README.md" <<'README'
# Voice to Clipboard (Linux Bundle)

## Run
- Standard mode: `./run.sh`
- Portable mode (stores config/cache in bundle folder): `./run-portable.sh`

## Notes
- On first run without a model, the app auto-downloads `base`.
- Runtime libraries are bundled in `./lib`.
README

if [[ -f "$PROJECT_DIR/LICENSE" ]]; then
  cp "$PROJECT_DIR/LICENSE" "$APP_DIR/LICENSE"
fi

echo "[5/6] Verifying linked libraries"
VERIFY_LOG="$STAGE_ROOT/ldd-verify.txt"
LD_LIBRARY_PATH="$LIB_DIR:${LD_LIBRARY_PATH:-}" ldd "$BIN_PATH" | tee "$VERIFY_LOG" >/dev/null
if grep -q "not found" "$VERIFY_LOG"; then
  echo "Error: unresolved shared libraries detected:" >&2
  grep "not found" "$VERIFY_LOG" >&2
  exit 1
fi

echo "[6/6] Creating archive + checksum"
tar -C "$STAGE_ROOT" -czf "$ARCHIVE_PATH" "voice-to-clipboard"
(
  cd "$(dirname "$ARCHIVE_PATH")"
  sha256sum "$(basename "$ARCHIVE_PATH")"
) > "$ARCHIVE_PATH.sha256"

ARCHIVE_SIZE="$(du -h "$ARCHIVE_PATH" | awk '{print $1}')"

echo ""
echo "Bundle created: $ARCHIVE_PATH"
echo "Checksum: $ARCHIVE_PATH.sha256"
echo "Size: $ARCHIVE_SIZE"
echo ""
echo "Smoke test:"
echo "  tar -xzf '$ARCHIVE_PATH' -C /tmp"
echo "  /tmp/voice-to-clipboard/run.sh"
