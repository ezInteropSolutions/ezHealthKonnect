#!/usr/bin/env bash
# Build self-contained macOS installers for ezHealthKonnect.
# Produces TWO binaries: arm64 (Apple Silicon) and amd64 (Intel).
# Requires Docker to be running (cross-compilation via golang:1.25-alpine).
#
# Usage:
#   cd installer
#   ./build-mac-release.sh              # full build (both arches)
#   ./build-mac-release.sh --arm64      # Apple Silicon only
#   ./build-mac-release.sh --amd64      # Intel only
#   ./build-mac-release.sh --skip-go    # reuse cached go-api binaries
#
# Output:
#   ../dist/ezHealthKonnect-Setup-Mac-arm64   (Apple Silicon M1/M2/M3)
#   ../dist/ezHealthKonnect-Setup-Mac-x64     (Intel)
#
# NOTE: macOS Gatekeeper will block unsigned binaries on first run.
# Users must: System Settings -> Privacy & Security -> Open Anyway
# Or: xattr -d com.apple.quarantine ./ezHealthKonnect-Setup-Mac-arm64

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
ASSETS_DIR="$SCRIPT_DIR/assets"

SKIP_GO_COMPILE=false
BUILD_ARM64=true
BUILD_AMD64=true
for arg in "$@"; do
  [[ "$arg" == "--skip-go" ]] && SKIP_GO_COMPILE=true
  [[ "$arg" == "--arm64"  ]] && BUILD_AMD64=false
  [[ "$arg" == "--amd64"  ]] && BUILD_ARM64=false
done

step()  { echo -e "\n\033[36m-- $1\033[0m"; }
ok()    { echo -e "   \033[32mOK\033[0m  $1"; }
warn()  { echo -e " \033[33mWARN\033[0m  $1"; }
fail()  { echo -e "\n \033[31mFAIL\033[0m  $1\n       Build aborted."; exit 1; }

echo ""
echo "  ezHealthKonnect -- macOS Installer Build"
echo "  ========================================="
echo "  Repo    : $REPO_ROOT"
echo "  Outputs : dist/ezHealthKonnect-Setup-Mac-arm64  (Apple Silicon)"
echo "            dist/ezHealthKonnect-Setup-Mac-x64    (Intel)"
if $SKIP_GO_COMPILE; then
  echo -e "  Mode    : \033[33mFast (skipping Go compile)\033[0m"
fi
echo ""

command -v docker >/dev/null 2>&1 || fail "Docker not found."
docker info >/dev/null 2>&1        || fail "Docker daemon not running."
ok "Docker ready"
mkdir -p "$DIST_DIR" "$ASSETS_DIR"

# ── Helper: build one architecture ───────────────────────────────────────────

build_arch() {
  local ARCH="$1"         # arm64 or amd64
  local SUFFIX="$2"       # Mac-arm64 or Mac-x64
  local BUNDLE_ZIP="$ASSETS_DIR/bundle-darwin.zip"
  local CACHED_GO_API="$DIST_DIR/go-api-darwin-$ARCH"
  local TMP_DIR
  TMP_DIR="$(mktemp -d)"
  local BUNDLE_DIR="$TMP_DIR/ezhealthkonnect"
  mkdir -p "$BUNDLE_DIR"

  cleanup_arch() { rm -rf "$TMP_DIR" 2>/dev/null || true; }
  trap cleanup_arch EXIT

  # Step A: Compile go-api
  step "[$SUFFIX] Compiling go-api for darwin/$ARCH"
  GO_API_IN_BUNDLE="$BUNDLE_DIR/go-api"
  if $SKIP_GO_COMPILE; then
    [[ -f "$CACHED_GO_API" ]] || fail "No cached go-api at dist/go-api-darwin-$ARCH. Run without --skip-go first."
    cp "$CACHED_GO_API" "$GO_API_IN_BUNDLE"
    ok "go-api (cached, $(du -sh "$GO_API_IN_BUNDLE" | cut -f1))"
  else
    docker run --rm \
      -v "$REPO_ROOT:/work" \
      -w /work \
      -e GOOS=darwin \
      -e GOARCH="$ARCH" \
      -e CGO_ENABLED=0 \
      golang:1.25-alpine \
      go build -ldflags "-s -w" -o "/work/installer/assets/_go-api-darwin-$ARCH-tmp" .

    mv "$ASSETS_DIR/_go-api-darwin-$ARCH-tmp" "$GO_API_IN_BUNDLE"
    chmod +x "$GO_API_IN_BUNDLE"
    cp "$GO_API_IN_BUNDLE" "$CACHED_GO_API"
    ok "go-api compiled and cached ($(du -sh "$GO_API_IN_BUNDLE" | cut -f1))"
  fi

  # Step B: Assemble bundle
  step "[$SUFFIX] Assembling app bundle"
  local EXCLUDE_NAMES=(".git" ".github" "installer" "dist" "dist-go" "architecture" "docs"
                       "connectivity" "tests" "logs" "schemas" "downloads" "node_modules")

  for item in "$REPO_ROOT"/*/; do
    name="$(basename "$item")"
    skip=false
    for ex in "${EXCLUDE_NAMES[@]}"; do [[ "$name" == "$ex" ]] && skip=true && break; done
    $skip || cp -r "$item" "$BUNDLE_DIR/"
  done
  for f in "$REPO_ROOT"/{package.json,package-lock.json,app.js,server.js,go.mod,go.sum,*.sql,*.md}; do
    [[ -f "$f" ]] && cp "$f" "$BUNDLE_DIR/" 2>/dev/null || true
  done

  # Core HL7 schemas
  local SCHEMA_SRC="$REPO_ROOT/schemas"
  local SCHEMA_DST="$BUNDLE_DIR/schemas"
  mkdir -p "$SCHEMA_DST"
  for ver in v2.3 v2.5 v2.5.1; do
    src="$SCHEMA_SRC/hl7/$ver"
    [[ -d "$src" ]] && mkdir -p "$SCHEMA_DST/hl7" && cp -r "$src" "$SCHEMA_DST/hl7/$ver" && ok "  Schema hl7/$ver"
  done
  [[ -d "$SCHEMA_SRC/fhir" ]] && cp -r "$SCHEMA_SRC/fhir" "$SCHEMA_DST/fhir" && ok "  Schema fhir"

  FILE_COUNT=$(find "$BUNDLE_DIR" -type f | wc -l | tr -d ' ')
  ok "Bundle: $FILE_COUNT files, $(du -sh "$BUNDLE_DIR" | cut -f1) uncompressed"

  # Step C: Zip
  step "[$SUFFIX] Compressing bundle"
  [[ -f "$BUNDLE_ZIP" ]] && rm -f "$BUNDLE_ZIP"
  (cd "$TMP_DIR" && zip -r -q "$BUNDLE_ZIP" "ezhealthkonnect")
  ok "bundle-darwin.zip ready ($(du -sh "$BUNDLE_ZIP" | cut -f1))"

  # Step D: Cross-compile installer
  step "[$SUFFIX] Cross-compiling installer with embedded bundle"
  local INSTALLER="$DIST_DIR/ezHealthKonnect-Setup-$SUFFIX"

  docker run --rm \
    -v "$REPO_ROOT:/work" \
    -w /work/installer \
    -e GOOS=darwin \
    -e GOARCH="$ARCH" \
    -e CGO_ENABLED=0 \
    golang:1.25-alpine \
    go build -tags embedded -ldflags "-s -w" \
    -o "/work/dist/ezHealthKonnect-Setup-$SUFFIX" .

  rm -f "$BUNDLE_ZIP"
  chmod +x "$INSTALLER"
  ok "$SUFFIX ready ($(du -sh "$INSTALLER" | cut -f1))"
  trap - EXIT
  rm -rf "$TMP_DIR"
}

# ── Build requested architectures ─────────────────────────────────────────────

$BUILD_ARM64 && build_arch arm64 Mac-arm64
$BUILD_AMD64 && build_arch amd64 Mac-x64

# ── Summary ───────────────────────────────────────────────────────────────────

echo ""
echo -e "  \033[32m=================================================\033[0m"
echo -e "  \033[32mBuild complete!\033[0m"
echo -e "  \033[32m=================================================\033[0m"
$BUILD_ARM64 && echo "  dist/ezHealthKonnect-Setup-Mac-arm64  (Apple Silicon M1/M2/M3)"
$BUILD_AMD64 && echo "  dist/ezHealthKonnect-Setup-Mac-x64    (Intel)"
echo ""
echo "  IMPORTANT: macOS will block unsigned binaries by default."
echo "  Instruct users to do ONE of the following after downloading:"
echo ""
echo "  Option 1 (GUI): System Settings > Privacy & Security > Open Anyway"
echo "  Option 2 (terminal): xattr -d com.apple.quarantine ./ezHealthKonnect-Setup-Mac-arm64"
echo ""
echo "  Then to run: ./ezHealthKonnect-Setup-Mac-arm64"
echo ""
echo "  Fast rebuild (JS/Node/SQL only):"
echo "    ./build-mac-release.sh --skip-go"
echo -e "  \033[32m=================================================\033[0m"
echo ""
