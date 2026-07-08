#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
RUN_DIR="${HOME}/.openbrain/run"
SESSION_PIDFILE="${RUN_DIR}/openbrain-run-dev.pid"
MAIN_PIDFILE="${RUN_DIR}/openbrain-run-dev.main.pid"
RENDERER_PIDFILE="${RUN_DIR}/openbrain-run-dev.renderer.pid"
ELECTRON_PIDFILE="${RUN_DIR}/openbrain-run-dev.electron.pid"
VITE_PORT="5173"
VITE_READY_PATH="/@vite/client"
VITE_READY_ATTEMPTS="60"
VITE_READY_INTERVAL_SECS="0.2"
VITE_READY_CURL_MAX_TIME_SECS="1"

DEV_MAIN_PID=""
DEV_RENDERER_PID=""
ELECTRON_PID=""
DEV_SERVER_URL=""

VITE_CANDIDATE_URLS=(
  "http://127.0.0.1:${VITE_PORT}"
  "http://[::1]:${VITE_PORT}"
  "http://localhost:${VITE_PORT}"
)

read_pidfile() {
  local pidfile="$1"
  if [ ! -f "${pidfile}" ]; then
    return 1
  fi
  local pid
  pid="$(<"${pidfile}" 2>/dev/null || true)"
  if [[ -z "${pid}" || ! "${pid}" =~ ^[0-9]+$ ]]; then
    rm -f "${pidfile}"
    return 1
  fi
  printf '%s' "${pid}"
}

stop_pid() {
  local pid="$1"
  local label="$2"
  if [[ "${pid}" == "$$" ]]; then
    return
  fi
  if ! kill -0 "${pid}" 2>/dev/null; then
    return
  fi
  echo "Stopping ${label} (pid ${pid})..."
  kill -TERM "${pid}" 2>/dev/null || true
  for _ in {1..20}; do
    if ! kill -0 "${pid}" 2>/dev/null; then
      return
    fi
    sleep 0.2
  done
  echo "${label} did not exit, force killing..."
  kill -KILL "${pid}" 2>/dev/null || true
}

stop_pidfile() {
  local pidfile="$1"
  local label="$2"
  local pid
  if ! pid="$(read_pidfile "${pidfile}")"; then
    return 0
  fi
  rm -f "${pidfile}"
  stop_pid "${pid}" "${label}"
}

clear_pidfile_if_matches() {
  local pidfile="$1"
  local expected_pid="$2"
  local current_pid
  current_pid="$(read_pidfile "${pidfile}")" || return 0
  if [[ "${current_pid}" == "${expected_pid}" ]]; then
    rm -f "${pidfile}"
  fi
}

stop_matching_processes() {
  local pattern="$1"
  local label="$2"
  local pids
  pids="$(pgrep -f "${pattern}" || true)"
  for pid in ${pids}; do
    stop_pid "${pid}" "${label}"
  done
}

stop_orphans() {
  # Fallback cleanup for stale runs where pidfiles are missing/corrupted.
  stop_matching_processes "${ROOT_DIR}/node_modules/.bin/../electron/cli.js \\.?$" "stale electron cli"
  stop_matching_processes "${ROOT_DIR}/node_modules/.pnpm/electron@.*/node_modules/electron/dist/Electron.app/Contents/MacOS/Electron ${ROOT_DIR}$" "stale Electron main"
  stop_matching_processes "${ROOT_DIR}/node_modules/electron/dist/Electron.app/Contents/MacOS/Electron ${ROOT_DIR}$" "stale Electron main"
  stop_matching_processes "${ROOT_DIR}/node_modules/.bin/../vite/bin/vite.js --port ${VITE_PORT} --strictPort$" "stale renderer watcher"
  stop_matching_processes "node ${ROOT_DIR}/node_modules/\\.bin/vite --port ${VITE_PORT} --strictPort$" "stale renderer watcher"
  stop_matching_processes "${ROOT_DIR}/node_modules/.bin/../typescript/bin/tsc -p tsconfig.main.json --watch$" "stale main watcher"
  stop_matching_processes "node ${ROOT_DIR}/node_modules/\\.bin/tsc -p tsconfig.main.json --watch$" "stale main watcher"
}

stop_existing() {
  # Try to stop the previous coordinator process first; it may clean up children itself.
  stop_pidfile "${SESSION_PIDFILE}" "existing dev session"

  # Then clean up stale child processes from pidfiles (e.g. previous hard-kill).
  stop_pidfile "${ELECTRON_PIDFILE}" "existing Electron"
  stop_pidfile "${RENDERER_PIDFILE}" "existing renderer watcher"
  stop_pidfile "${MAIN_PIDFILE}" "existing main watcher"
  stop_orphans
}

probe_vite_url() {
  local base_url="$1"
  local probe_url="${base_url}${VITE_READY_PATH}"

  curl \
    --silent \
    --show-error \
    --fail \
    --max-time "${VITE_READY_CURL_MAX_TIME_SECS}" \
    "${probe_url}" \
    >/dev/null 2>&1
}

wait_for_vite() {
  if ! command -v curl >/dev/null 2>&1; then
    echo "Warning: curl not found, defaulting dev server URL to http://localhost:${VITE_PORT}."
    DEV_SERVER_URL="http://localhost:${VITE_PORT}"
    return 0
  fi

  echo "Waiting for Vite dev server..."
  echo "  candidates:"
  for candidate in "${VITE_CANDIDATE_URLS[@]}"; do
    echo "    - ${candidate}${VITE_READY_PATH}"
  done

  for ((attempt = 1; attempt <= VITE_READY_ATTEMPTS; attempt += 1)); do
    for candidate in "${VITE_CANDIDATE_URLS[@]}"; do
      if probe_vite_url "${candidate}"; then
        DEV_SERVER_URL="${candidate}"
        echo "Detected Vite dev server URL: ${DEV_SERVER_URL}"
        return 0
      fi
    done
    sleep "${VITE_READY_INTERVAL_SECS}"
  done

  echo "ERROR: Timed out waiting for Vite dev server after ${VITE_READY_ATTEMPTS} attempts." >&2
  echo "Probed URLs:" >&2
  for candidate in "${VITE_CANDIDATE_URLS[@]}"; do
    echo "  - ${candidate}${VITE_READY_PATH}" >&2
  done
  return 1
}

cleanup() {
  local exit_code=$?
  trap - INT TERM EXIT

  if [ -n "${ELECTRON_PID}" ]; then
    stop_pid "${ELECTRON_PID}" "Electron"
  fi
  if [ -n "${DEV_RENDERER_PID}" ]; then
    stop_pid "${DEV_RENDERER_PID}" "renderer watcher"
  fi
  if [ -n "${DEV_MAIN_PID}" ]; then
    stop_pid "${DEV_MAIN_PID}" "main watcher"
  fi

  stop_pidfile "${ELECTRON_PIDFILE}" "Electron"
  stop_pidfile "${RENDERER_PIDFILE}" "renderer watcher"
  stop_pidfile "${MAIN_PIDFILE}" "main watcher"
  stop_orphans
  clear_pidfile_if_matches "${ELECTRON_PIDFILE}" "${ELECTRON_PID}"
  clear_pidfile_if_matches "${RENDERER_PIDFILE}" "${DEV_RENDERER_PID}"
  clear_pidfile_if_matches "${MAIN_PIDFILE}" "${DEV_MAIN_PID}"
  clear_pidfile_if_matches "${SESSION_PIDFILE}" "$$"
  exit "${exit_code}"
}

trap cleanup INT TERM EXIT

mkdir -p "${RUN_DIR}"
stop_existing
echo "$$" > "${SESSION_PIDFILE}"

echo "Syncing settings..."
bash "${SCRIPT_DIR}/sync-settings.sh"

cd "${ROOT_DIR}"

TSC_BIN="${ROOT_DIR}/node_modules/.bin/tsc"
VITE_BIN="${ROOT_DIR}/node_modules/.bin/vite"
if [ ! -x "${TSC_BIN}" ] || [ ! -x "${VITE_BIN}" ]; then
  echo "Missing node_modules binaries. Run pnpm install in ${ROOT_DIR} first."
  exit 1
fi

ELECTRON_BIN="$(node -e "process.stdout.write(require('electron'))")"
if [ -z "${ELECTRON_BIN}" ] || [ ! -x "${ELECTRON_BIN}" ]; then
  echo "Cannot resolve Electron binary from local dependencies."
  exit 1
fi

echo "Building main (tsc)..."
"${TSC_BIN}" -p tsconfig.main.json

echo "Starting dev:main (tsc --watch)..."
"${TSC_BIN}" -p tsconfig.main.json --watch &
DEV_MAIN_PID=$!
echo "${DEV_MAIN_PID}" > "${MAIN_PIDFILE}"

echo "Starting dev:renderer (vite on ${VITE_PORT})..."
"${VITE_BIN}" --host 127.0.0.1 --port "${VITE_PORT}" --strictPort &
DEV_RENDERER_PID=$!
echo "${DEV_RENDERER_PID}" > "${RENDERER_PIDFILE}"

wait_for_vite

echo "Starting Electron..."
OPENBRAIN_DEV_SERVER_URL="${DEV_SERVER_URL}" "${ELECTRON_BIN}" "${ROOT_DIR}" &
ELECTRON_PID=$!
echo "${ELECTRON_PID}" > "${ELECTRON_PIDFILE}"

wait "${ELECTRON_PID}"
