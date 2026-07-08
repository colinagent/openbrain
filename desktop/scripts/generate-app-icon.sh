#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"
ICON_SVG="${BUILD_DIR}/icon.svg"
MASTER_PNG="${BUILD_DIR}/icon.png"
ICONSET_DIR="${BUILD_DIR}/icon.iconset"
ICNS_FILE="${BUILD_DIR}/icon.icns"

render_png() {
  local size="$1"
  local input="$2"
  local output="$3"

  if command -v rsvg-convert >/dev/null 2>&1; then
    rsvg-convert -w "${size}" -h "${size}" "${input}" -o "${output}"
    return 0
  fi

  if command -v sips >/dev/null 2>&1; then
    sips -s format png -z "${size}" "${size}" "${input}" --out "${output}" >/dev/null
    return 0
  fi

  echo "Error: rsvg-convert or sips is required to build macOS app icons" >&2
  exit 1
}

if ! command -v iconutil >/dev/null 2>&1; then
  echo "Error: iconutil is required to build ${ICNS_FILE}" >&2
  exit 1
fi

if [[ ! -f "${ICON_SVG}" ]]; then
  echo "Error: ${ICON_SVG} not found" >&2
  exit 1
fi

mkdir -p "${BUILD_DIR}"
rm -rf "${ICONSET_DIR}"
mkdir -p "${ICONSET_DIR}"
rm -f "${MASTER_PNG}" "${ICNS_FILE}"

# Render master 1024x1024 PNG from SVG
render_png 1024 "${ICON_SVG}" "${MASTER_PNG}"

# Generate iconset at all required sizes
render_icon() {
  local size="$1"
  local filename="$2"
  render_png "${size}" "${ICON_SVG}" "${ICONSET_DIR}/${filename}"
}

render_icon 16 icon_16x16.png
render_icon 32 icon_16x16@2x.png
render_icon 32 icon_32x32.png
render_icon 64 icon_32x32@2x.png
render_icon 128 icon_128x128.png
render_icon 256 icon_128x128@2x.png
render_icon 256 icon_256x256.png
render_icon 512 icon_256x256@2x.png
render_icon 512 icon_512x512.png
render_icon 1024 icon_512x512@2x.png

iconutil -c icns "${ICONSET_DIR}" -o "${ICNS_FILE}"

echo "Generated macOS app icon assets:"
echo "  SVG:   ${ICON_SVG}"
echo "  PNG:   ${MASTER_PNG}"
echo "  ICNS:  ${ICNS_FILE}"
echo "  SET:   ${ICONSET_DIR}"
