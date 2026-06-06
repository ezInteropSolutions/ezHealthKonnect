#!/usr/bin/env bash
# Build a self-contained Linux installer for ezHealthKonnect.
# Requires Docker to be running (cross-compilation via golang:1.25-alpine).
#
# Usage:
#   cd installer
#   ./build-linux-release.sh              # full build
#   ./build-linux-release.sh --skip-go    # reuse cached go-api (JS/SQL changes only)
#
# Output: ../dist/ezHealthKonnect-Setup-Linux-x64

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
ASSETS_DIR="$SCRIPT_DIR/assets"
BUNDLE_ZIP="$ASSETS_DIR/bundle-linux-amd64.zip"
CACHED_GO_API="$DIST_DIR/go-api-linux-amd64"   # persisted between builds
TMP_DIR="$(mktemp -d)"
BUNDLE_DIR="$TMP_DIR/ezhealthkonnect"

SKIP_GO_COMPILE=false
for arg in "$@"; do
  [[ "$arg" == "--skip-go" ]] && SKIP_GO_COMPILE=true
done

step()  { echo -e "\n\033[36m-- Step $1: $2\033[0m"; }
ok()    { echo -e "   \033[32mOK\033[0m  $1"; }
warn()  { echo -e " \033[33mWARN\033[0m  $1"; }
fail()  { echo -e "\n \033[31mFAIL\033[0m  $1\n       Build aborted."; rm -rf "$TMP_DIR"; exit 1; }

cleanup() { rm -rf "$TMP_DIR" 2>/dev/null || true; }
trap cleanup EXIT

echo ""
echo "  ezHealthKonnect -- Linux Installer Build"
echo "  ========================================="
echo "  Repo  : $REPO_ROOT"
echo "  Output: $DIST_DIR/ezHealthKonnect-Setup-Linux-x64"
if $SKIP_GO_COMPILE; then
  echo -e "  Mode  : \033[33mFast (skipping Go compile -- JS/Node/SQL changes only)\033[0m"
else
  echo "  Mode  : Full (compiling go-api + installer)"
fi
echo ""

# ---- Preflight ---------------------------------------------------------------
command -v docker >/dev/null 2>&1 || fail "Docker not found. Install Docker and try again."
docker info >/dev/null 2>&1        || fail "Docker daemon is not running. Start it and try again."
ok "Docker ready"

mkdir -p "$DIST_DIR" "$ASSETS_DIR" "$BUNDLE_DIR"

# ---- Step 1: Cross-compile go-api (or reuse cache) --------------------------
step 1 "Compiling go-api for linux/amd64"
GO_API_IN_BUNDLE="$BUNDLE_DIR/go-api"

if $SKIP_GO_COMPILE; then
  [[ -f "$CACHED_GO_API" ]] || fail "No cached go-api found at dist/go-api-linux-amd64. Run without --skip-go first."
  cp "$CACHED_GO_API" "$GO_API_IN_BUNDLE"
  ok "go-api (cached, $(du -sh "$GO_API_IN_BUNDLE" | cut -f1))"
else
  docker run --rm \
    -v "$REPO_ROOT:/work" \
    -w /work \
    -e GOOS=linux \
    -e GOARCH=amd64 \
    -e CGO_ENABLED=0 \
    golang:1.25-alpine \
    go build -ldflags "-s -w" -o /work/installer/assets/_go-api-linux-tmp .

  mv "$ASSETS_DIR/_go-api-linux-tmp" "$GO_API_IN_BUNDLE"
  chmod +x "$GO_API_IN_BUNDLE"
  cp "$GO_API_IN_BUNDLE" "$CACHED_GO_API"
  ok "go-api compiled and cached ($(du -sh "$GO_API_IN_BUNDLE" | cut -f1))"
fi

# ---- Step 2: Assemble app bundle --------------------------------------------
step 2 "Assembling app bundle"

EXCLUDE_NAMES=(".git" ".github" "installer" "dist" "dist-go" "architecture" "docs"
               "connectivity" "tests" "logs" "schemas" "downloads" "node_modules")

for item in "$REPO_ROOT"/*/; do
  name="$(basename "$item")"
  skip=false
  for ex in "${EXCLUDE_NAMES[@]}"; do [[ "$name" == "$ex" ]] && skip=true && break; done
  for ex in ".env" ".env.production" "go-api" "go-api-linux" "ezhealthkonnect"; do
    [[ "$name" == "$ex" ]] && skip=true
  done
  $skip || cp -r "$item" "$BUNDLE_DIR/"
done

# Copy file-level items (non-directories) from repo root
for f in "$REPO_ROOT"/{package.json,package-lock.json,app.js,server.js,go.mod,go.sum,*.sql,*.md}; do
  [[ -f "$f" ]] && cp "$f" "$BUNDLE_DIR/" 2>/dev/null || true
done

# Core HL7 schemas
SCHEMA_SRC="$REPO_ROOT/schemas"
SCHEMA_DST="$BUNDLE_DIR/schemas"
mkdir -p "$SCHEMA_DST"
for ver in v2.3 v2.5 v2.5.1; do
  src="$SCHEMA_SRC/hl7/$ver"
  if [[ -d "$src" ]]; then
    mkdir -p "$SCHEMA_DST/hl7"
    cp -r "$src" "$SCHEMA_DST/hl7/$ver"
    ok "  Schema hl7/$ver included"
  else
    warn "  Schema hl7/$ver not found in repo -- skipping"
  fi
done
[[ -d "$SCHEMA_SRC/fhir" ]] && cp -r "$SCHEMA_SRC/fhir" "$SCHEMA_DST/fhir" && ok "  Schema fhir included"

FILE_COUNT=$(find "$BUNDLE_DIR" -type f | wc -l | tr -d ' ')
BUNDLE_MB=$(du -sh "$BUNDLE_DIR" | cut -f1)
ok "Bundle staged: $FILE_COUNT files, $BUNDLE_MB uncompressed"

# ---- Step 3: Zip bundle ------------------------------------------------------
step 3 "Compressing bundle -> installer/assets/bundle-linux-amd64.zip"

[[ -f "$BUNDLE_ZIP" ]] && rm -f "$BUNDLE_ZIP"
(cd "$TMP_DIR" && zip -r -q "$BUNDLE_ZIP" "ezhealthkonnect")

ZIP_MB=$(du -sh "$BUNDLE_ZIP" | cut -f1)
ok "bundle-linux-amd64.zip ready ($ZIP_MB)"

# ---- Step 4: Cross-compile installer with embedded bundle -------------------
step 4 "Cross-compiling installer exe with embedded bundle"

INSTALLER="$DIST_DIR/ezHealthKonnect-Setup-Linux-x64"

docker run --rm \
  -v "$REPO_ROOT:/work" \
  -w /work/installer \
  -e GOOS=linux \
  -e GOARCH=amd64 \
  -e CGO_ENABLED=0 \
  golang:1.25-alpine \
  go build -tags embedded -ldflags "-s -w" \
  -o /work/dist/ezHealthKonnect-Setup-Linux-x64 .

rm -f "$BUNDLE_ZIP"

chmod +x "$INSTALLER"
FINAL_MB=$(du -sh "$INSTALLER" | cut -f1)

echo ""
echo -e "  \033[32m=================================================\033[0m"
echo -e "  \033[32mBuild complete!\033[0m"
echo -e "  \033[32m=================================================\033[0m"
echo    "  dist/ezHealthKonnect-Setup-Linux-x64  ($FINAL_MB)"
echo ""
echo    "  On the target machine:"
echo    "    chmod +x ezHealthKonnect-Setup-Linux-x64"
echo    "    sudo ./ezHealthKonnect-Setup-Linux-x64"
echo ""
echo    "  Fast rebuild (JS/Node/SQL only):"
echo    "    ./build-linux-release.sh --skip-go"
echo -e "  \033[32m=================================================\033[0m"
echo ""
