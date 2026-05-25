#!/usr/bin/env bash
# installer/build.sh — Build the ezHealthKonnect installer binaries using Docker
#
# Usage:
#   bash installer/build.sh                  # build all platforms
#   bash installer/build.sh --windows-only   # build .exe only
#   bash installer/build.sh --linux-only     # build linux binary only
#   bash installer/build.sh --version v1.2.0 # embed version string
#
# Output:  installer/dist/
#   ezhealthkonnect-installer.exe      (Windows amd64)
#   ezhealthkonnect-installer-linux    (Linux amd64)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DIST_DIR="${SCRIPT_DIR}/dist"

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
ok()   { echo -e "${GREEN}  ✔ ${NC}${*}"; }
info() { echo -e "${CYAN}  → ${NC}${*}"; }
die()  { echo -e "${RED}  ✖ ${NC}${*}" >&2; exit 1; }

# ── Args ──────────────────────────────────────────────────────────────────────
BUILD_WINDOWS=true
BUILD_LINUX=true
VERSION="dev"

while [[ $# -gt 0 ]]; do
  case $1 in
    --windows-only) BUILD_LINUX=false;   shift ;;
    --linux-only)   BUILD_WINDOWS=false; shift ;;
    --version)      VERSION="$2";        shift 2 ;;
    *) die "Unknown option: $1" ;;
  esac
done

# ── Sanity ────────────────────────────────────────────────────────────────────
command -v docker &>/dev/null || die "Docker is required to build the installer"
docker info &>/dev/null       || die "Docker daemon is not running"

echo -e "${BOLD}${CYAN}ezHealthKonnect Installer Build${NC}"
echo -e "  Version: ${VERSION}"
echo -e "  Source:  ${SCRIPT_DIR}"
echo -e "  Output:  ${DIST_DIR}"
echo ""

mkdir -p "$DIST_DIR"

# ── Build image with Go compiler ──────────────────────────────────────────────
GO_IMAGE="golang:1.21-alpine"
info "Pulling ${GO_IMAGE}..."
docker pull -q "$GO_IMAGE"

# ── Build Windows .exe ────────────────────────────────────────────────────────
if [[ "$BUILD_WINDOWS" == "true" ]]; then
  info "Building Windows installer (amd64)..."
  docker run --rm \
    -v "${SCRIPT_DIR}:/src:ro" \
    -v "${DIST_DIR}:/dist" \
    -w /src \
    -e GOOS=windows \
    -e GOARCH=amd64 \
    -e CGO_ENABLED=0 \
    "$GO_IMAGE" \
    go build \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /dist/ezhealthkonnect-installer.exe \
      .
  ok "ezhealthkonnect-installer.exe  ($(du -sh "${DIST_DIR}/ezhealthkonnect-installer.exe" | cut -f1))"
fi

# ── Build Linux binary ────────────────────────────────────────────────────────
if [[ "$BUILD_LINUX" == "true" ]]; then
  info "Building Linux installer (amd64)..."
  docker run --rm \
    -v "${SCRIPT_DIR}:/src:ro" \
    -v "${DIST_DIR}:/dist" \
    -w /src \
    -e GOOS=linux \
    -e GOARCH=amd64 \
    -e CGO_ENABLED=0 \
    "$GO_IMAGE" \
    go build \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /dist/ezhealthkonnect-installer-linux \
      .
  chmod +x "${DIST_DIR}/ezhealthkonnect-installer-linux"
  ok "ezhealthkonnect-installer-linux  ($(du -sh "${DIST_DIR}/ezhealthkonnect-installer-linux" | cut -f1))"
fi

# ── Copy install.sh ───────────────────────────────────────────────────────────
cp "${SCRIPT_DIR}/install.sh" "${DIST_DIR}/install.sh"
chmod +x "${DIST_DIR}/install.sh"
ok "install.sh"

echo ""
echo -e "${GREEN}${BOLD}Build complete → ${DIST_DIR}${NC}"
ls -lh "${DIST_DIR}"
