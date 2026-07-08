#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
DESKTOP_ROOT="${REPO_ROOT}/desktop"
SERVER_ROOT="${REPO_ROOT}/server"
OP_HOME="${OPENBRAIN_HOME:-${OP_HOME:-${HOME}/.openbrain}}"
OPENBRAIN_RUNTIME_SOURCE_ROOT="${OPENBRAIN_RUNTIME_SOURCE_ROOT:-${REPO_ROOT}/opagent-runtime}"
OPENBRAIN_CODER_AGENT_SOURCE_ROOT="${OPENBRAIN_CODER_AGENT_SOURCE_ROOT:-${REPO_ROOT}/agents/coder}"
OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT="${OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT:-${REPO_ROOT}/agents/simple-memory}"
OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT="${OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT:-${REPO_ROOT}/agents/gbrain}"
OPENBRAIN_GBRAIN_SOURCE_ROOT="${OPENBRAIN_GBRAIN_SOURCE_ROOT:-${REPO_ROOT}/../gbrain}"
OPENBRAIN_TOOLS_SOURCE_ROOT="${OPENBRAIN_TOOLS_SOURCE_ROOT:-${REPO_ROOT}/tools}"
OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT="${OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT:-${REPO_ROOT}/skills/openbrain-cloud-sync}"
RUN_DIR="${OP_HOME}/run"
RUNTIME_BIN_DIR="${OP_HOME}/bin"
RUNTIME_BIN="${RUNTIME_BIN_DIR}/opagent-runtime"
GBRAIN_BIN="${RUNTIME_BIN_DIR}/gbrain"
RUNTIME_LOG_DIR="${OP_HOME}/logs/opagent-runtime"
RUNTIME_LOG_FILE="${RUNTIME_LOG_DIR}/opagent-runtime.log"
RUNTIME_PIDFILE="${RUN_DIR}/opagent-runtime.pid"
LATEST_VERSION_FILE="${RUN_DIR}/latest.version"
RUNTIME_HEALTH_URL="http://127.0.0.1:19530/health"
RUNTIME_WAIT_ATTEMPTS="${OPENBRAIN_RUNTIME_WAIT_ATTEMPTS:-60}"
RUNTIME_WAIT_INTERVAL_SECS="${OPENBRAIN_RUNTIME_WAIT_INTERVAL_SECS:-0.5}"
RUNTIME_CURL_MAX_TIME_SECS="${OPENBRAIN_RUNTIME_CURL_MAX_TIME_SECS:-2}"

SERVER_PLATFORM="${OPENBRAIN_SERVER_PLATFORM:-}"
SERVER_DIST_BASE="${SERVER_ROOT}/dist"
SERVER_AGENT_DIR="${OP_HOME}/agents/opagent-server"
LEGACY_OPENBRAIN_SERVER_AGENT_DIR="${OP_HOME}/agents/openbrain-server"
SERVER_AGENT_MD="${SERVER_AGENT_DIR}/.agent/AGENT.md"
SERVER_BIN="${SERVER_AGENT_DIR}/.agent/bin/openbrain-server"
CODER_AGENT_DIR="${OP_HOME}/agents/coder"
CODER_AGENT_BIN="${CODER_AGENT_DIR}/.agent/bin/coder"
CODER_AGENT_MD="${CODER_AGENT_DIR}/.agent/AGENT.md"
LEGACY_OPAGENT_AGENT_DIR="${OP_HOME}/agents/opagent"
SIMPLE_MEMORY_DIR="${OP_HOME}/agents/simple-memory"
SIMPLE_MEMORY_MD="${SIMPLE_MEMORY_DIR}/.agent/AGENT.md"
GBRAIN_AGENT_DIR="${OP_HOME}/agents/gbrain"
GBRAIN_AGENT_MD="${GBRAIN_AGENT_DIR}/.agent/AGENT.md"
CLOUD_SYNC_SKILL_DIR="${OP_HOME}/skills/openbrain-cloud-sync"
CLOUD_SYNC_SKILL_BIN="${CLOUD_SYNC_SKILL_DIR}/bin/openbrain-cloud-sync-helper"
CLOUD_SYNC_SKILL_MD="${CLOUD_SYNC_SKILL_DIR}/SKILL.md"
DEFAULT_WORKSPACE_DIR="${OP_HOME}/workspace"
DEFAULT_WORKSPACE_AGENT_MD="${DEFAULT_WORKSPACE_DIR}/.agent/AGENT.md"

DESKTOP_SESSION_PIDFILE="${RUN_DIR}/openbrain-run-dev.pid"
DESKTOP_MAIN_PIDFILE="${RUN_DIR}/openbrain-run-dev.main.pid"
DESKTOP_RENDERER_PIDFILE="${RUN_DIR}/openbrain-run-dev.renderer.pid"
DESKTOP_ELECTRON_PIDFILE="${RUN_DIR}/openbrain-run-dev.electron.pid"

VITE_PORT="${OPENBRAIN_VITE_PORT:-5173}"
VITE_READY_PATH="/@vite/client"
VITE_READY_ATTEMPTS="${OPENBRAIN_VITE_READY_ATTEMPTS:-60}"
VITE_READY_INTERVAL_SECS="${OPENBRAIN_VITE_READY_INTERVAL_SECS:-0.2}"
VITE_READY_CURL_MAX_TIME_SECS="${OPENBRAIN_VITE_READY_CURL_MAX_TIME_SECS:-1}"
VITE_CANDIDATE_URLS=(
  "http://127.0.0.1:${VITE_PORT}"
  "http://[::1]:${VITE_PORT}"
  "http://localhost:${VITE_PORT}"
)

DEV_MAIN_PID=""
DEV_RENDERER_PID=""
ELECTRON_PID=""
DEV_SERVER_URL=""

log() {
  printf '[openbrain-run-dev] %s\n' "$*"
}

die() {
  printf '[openbrain-run-dev] ERROR: %s\n' "$*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
usage: scripts/openbrain/openbrain-run-dev.sh

Starts the full local OpenBrain development stack:
  1. build/install the local OpAgent runtime from ./opagent-runtime
  2. build/install the local coder and simple-memory agents
  3. create ~/.openbrain/workspace when missing and bind it to coder
  4. build/install the current OpenBrain server agent
  5. start the runtime and wait for :19530 health
  6. start desktop main watcher, Vite, and Electron

Environment:
  OPENBRAIN_HOME                  Runtime base dir. Default: ~/.openbrain
  OPENBRAIN_VITE_PORT             Desktop Vite port. Default: 5173
  OPENBRAIN_RUNTIME_SOURCE_ROOT   Local runtime source root. Default: ./opagent-runtime
  OPENBRAIN_CODER_AGENT_SOURCE_ROOT
                                  Local coder agent source root. Default: ./agents/coder
  OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT
                                  Local simple-memory agent source root. Default: ./agents/simple-memory
  OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT
                                  Local gbrain agent source root. Default: ./agents/gbrain
  OPENBRAIN_GBRAIN_SOURCE_ROOT    Local colinagent/gbrain fork checkout. Default: ../gbrain
  OPENBRAIN_TOOLS_SOURCE_ROOT     Local tools source root. Default: ./tools
  OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT
                                  Local OpenBrain Cloud Sync skill source root. Default: ./skills/openbrain-cloud-sync
  OPENBRAIN_ELECTRON_ARGS         Extra whitespace-separated args passed to Electron.
                                  Example: --remote-debugging-port=9222
EOF
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

require_cmd() {
  have_cmd "$1" || die "required command not found: $1"
}

read_pidfile() {
  local pidfile="$1"
  [[ -f "$pidfile" ]] || return 1
  local pid
  pid="$(<"$pidfile" 2>/dev/null || true)"
  if [[ -z "$pid" || ! "$pid" =~ ^[0-9]+$ ]]; then
    rm -f "$pidfile"
    return 1
  fi
  printf '%s' "$pid"
}

stop_pid() {
  local pid="$1"
  local label="$2"
  [[ "$pid" == "$$" ]] && return 0
  kill -0 "$pid" 2>/dev/null || return 0
  log "Stopping ${label} (pid ${pid})"
  kill -TERM "$pid" 2>/dev/null || true
  for _ in {1..20}; do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 0.2
  done
  log "${label} did not exit, force killing"
  kill -KILL "$pid" 2>/dev/null || true
}

stop_pidfile() {
  local pidfile="$1"
  local label="$2"
  local pid
  pid="$(read_pidfile "$pidfile")" || return 0
  rm -f "$pidfile"
  stop_pid "$pid" "$label"
}

clear_pidfile_if_matches() {
  local pidfile="$1"
  local expected_pid="$2"
  local current_pid
  current_pid="$(read_pidfile "$pidfile")" || return 0
  [[ "$current_pid" == "$expected_pid" ]] && rm -f "$pidfile"
}

stop_matching_processes() {
  local pattern="$1"
  local label="$2"
  local pids pid
  have_cmd pgrep || return 0
  pids="$(pgrep -f "$pattern" || true)"
  for pid in $pids; do
    stop_pid "$pid" "$label"
  done
}

stop_parent_processes_matching() {
  local pattern="$1"
  local label="$2"
  local pids pid parent_pid
  have_cmd pgrep || return 0
  pids="$(pgrep -f "$pattern" || true)"
  for pid in $pids; do
    parent_pid="$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d '[:space:]' || true)"
    if [[ -n "$parent_pid" && "$parent_pid" =~ ^[0-9]+$ && "$parent_pid" != '1' ]]; then
      stop_pid "$parent_pid" "$label"
    fi
  done
}

stop_desktop_orphans() {
  stop_matching_processes "${DESKTOP_ROOT}/node_modules/.bin/../electron/cli.js \\.?$" 'stale electron cli'
  stop_matching_processes "${DESKTOP_ROOT}/node_modules/.pnpm/electron@.*/node_modules/electron/dist/Electron.app/Contents/MacOS/Electron ${DESKTOP_ROOT}$" 'stale Electron main'
  stop_matching_processes "${DESKTOP_ROOT}/node_modules/electron/dist/Electron.app/Contents/MacOS/Electron ${DESKTOP_ROOT}$" 'stale Electron main'
  stop_parent_processes_matching "Electron Helper .*--app-path=${DESKTOP_ROOT}( |$)" 'stale Electron main'
  stop_matching_processes "${DESKTOP_ROOT}/node_modules/.bin/../vite/bin/vite.js( --host [^ ]+)? --port ${VITE_PORT} --strictPort$" 'stale renderer watcher'
  stop_matching_processes "node ${DESKTOP_ROOT}/node_modules/\\.bin/vite( --host [^ ]+)? --port ${VITE_PORT} --strictPort$" 'stale renderer watcher'
  stop_matching_processes "${DESKTOP_ROOT}/node_modules/.bin/../typescript/bin/tsc -p tsconfig.main.json --watch$" 'stale main watcher'
  stop_matching_processes "node ${DESKTOP_ROOT}/node_modules/\\.bin/tsc -p tsconfig.main.json --watch$" 'stale main watcher'
}

stop_existing_desktop_session() {
  stop_pidfile "$DESKTOP_SESSION_PIDFILE" 'existing OpenBrain dev session'
  stop_pidfile "$DESKTOP_ELECTRON_PIDFILE" 'existing Electron'
  stop_pidfile "$DESKTOP_RENDERER_PIDFILE" 'existing renderer watcher'
  stop_pidfile "$DESKTOP_MAIN_PIDFILE" 'existing main watcher'
  stop_desktop_orphans
}

stop_existing_runtime() {
  stop_pidfile "$RUNTIME_PIDFILE" 'existing opagent-runtime'
  rm -f \
    "${RUN_DIR}/opagent-server.pid" "${RUN_DIR}/opagent-server.lock" \
    "${RUN_DIR}/openbrain-server.pid" "${RUN_DIR}/openbrain-server.lock" \
    "${RUN_DIR}/systool.pid" "${RUN_DIR}/systool.lock"

  stop_matching_processes "^${RUNTIME_BIN//\//\\/} --base-dir ${OP_HOME//\//\\/}$" 'stray opagent-runtime'
  stop_matching_processes "^${SERVER_AGENT_DIR//\//\\/}/.agent/bin/openbrain-server --host 127\\.0\\.0\\.1 --port 19530$" 'stray openbrain-server'
  stop_matching_processes "^${SERVER_AGENT_DIR//\//\\/}/bin/openbrain-server --host 127\\.0\\.0\\.1 --port 19530$" 'stray legacy opagent-server'
  stop_matching_processes "^${LEGACY_OPENBRAIN_SERVER_AGENT_DIR//\//\\/}/bin/openbrain-server --host 127\\.0\\.0\\.1 --port 19530$" 'stray legacy openbrain-server'
  if have_cmd lsof; then
    local listener_pids
    listener_pids="$(lsof -tiTCP:19530 -sTCP:LISTEN 2>/dev/null || true)"
    if [[ -n "$listener_pids" ]]; then
      log "Stopping existing listeners on 127.0.0.1:19530: ${listener_pids}"
      kill -TERM $listener_pids 2>/dev/null || true
      sleep 1
      kill -KILL $listener_pids 2>/dev/null || true
    fi
  fi
}

platform_key() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"
  case "${os}:${arch}" in
    darwin:arm64|darwin:aarch64) printf 'darwin-arm64' ;;
    darwin:x86_64|darwin:amd64) printf 'darwin-amd64' ;;
    linux:x86_64|linux:amd64) printf 'linux-amd64' ;;
    windows:*|mingw*:*) printf 'windows-amd64' ;;
    *) die "unsupported runtime platform: ${os}/${arch}" ;;
  esac
}

server_platform_key() {
  local platform="$1"
  case "$platform" in
    darwin-arm64|darwin-amd64|linux-amd64|windows-amd64) printf '%s' "$platform" ;;
    linux-arm64) printf 'linux-amd64' ;;
    *) die "unsupported server platform: $platform" ;;
  esac
}

build_and_install_local_runtime() {
  local tmp_bin
  tmp_bin="${RUNTIME_BIN}.tmp"

  [[ -d "${OPENBRAIN_RUNTIME_SOURCE_ROOT}/cmd/opagent-runtime" ]] || {
    die "local runtime source not found: ${OPENBRAIN_RUNTIME_SOURCE_ROOT}/cmd/opagent-runtime"
  }

  mkdir -p "$RUNTIME_BIN_DIR" "$RUN_DIR"
  rm -f "$tmp_bin"

  log "Stage: build local OpAgent runtime (${OPENBRAIN_RUNTIME_SOURCE_ROOT})"
  (
    cd "$OPENBRAIN_RUNTIME_SOURCE_ROOT"
    go build -trimpath -o "$tmp_bin" ./cmd/opagent-runtime
  )
  chmod +x "$tmp_bin"
  mv -f "$tmp_bin" "$RUNTIME_BIN"
  printf 'local\n' > "$LATEST_VERSION_FILE"
}

ensure_runtime_config() {
  local config_dest="${OP_HOME}/configs/config.json"
  local template="${OPENBRAIN_RUNTIME_SOURCE_ROOT}/configs/config.json"
  mkdir -p "${OP_HOME}/configs"
  if [[ -f "$config_dest" ]]; then
    return 0
  fi
  [[ -f "$template" ]] || die "runtime config missing and template not found: ${template}"
  log "Installing runtime config template: ${config_dest}"
  cp -f "$template" "$config_dest"
}

build_and_install_gbrain() {
  local tmp_bin
  tmp_bin="${GBRAIN_BIN}.tmp"

  require_cmd bun
  [[ -f "${OPENBRAIN_GBRAIN_SOURCE_ROOT}/src/cli.ts" ]] || {
    die "local gbrain CLI source not found: ${OPENBRAIN_GBRAIN_SOURCE_ROOT}/src/cli.ts; set OPENBRAIN_GBRAIN_SOURCE_ROOT or clone https://github.com/colinagent/gbrain next to this repo"
  }
  mkdir -p "$RUNTIME_BIN_DIR"
  rm -f "$tmp_bin"

  log "Stage: build local gbrain CLI (${OPENBRAIN_GBRAIN_SOURCE_ROOT})"
  (
    cd "$OPENBRAIN_GBRAIN_SOURCE_ROOT"
    bun build --compile --outfile "$tmp_bin" src/cli.ts
  )
  chmod +x "$tmp_bin"
  mv -f "$tmp_bin" "$GBRAIN_BIN"
}

manifest_has_system_tag() {
  local manifest="$1"
  awk '
    BEGIN { in_tags = 0; found = 0 }
    /^[^[:space:]]/ {
      if ($0 ~ /^tags:[[:space:]]*/) {
        line = $0
        sub(/^tags:[[:space:]]*/, "", line)
        if (line ~ /(^|[^[:alnum:]_-])system([^[:alnum:]_-]|$)/) {
          found = 1
        }
        in_tags = (line == "")
        next
      }
      in_tags = 0
    }
    in_tags && /^[[:space:]]*-[[:space:]]*/ {
      line = $0
      sub(/^[[:space:]]*-[[:space:]]*/, "", line)
      gsub(/["'\'' ,]/, "", line)
      if (line == "system") {
        found = 1
      }
    }
    END { exit found ? 0 : 1 }
  ' "$manifest"
}

project_system_tool_bins() {
  local manifest tool_dir bin_dir copied
  copied=0
  mkdir -p "$RUNTIME_BIN_DIR"
  for manifest in "${OP_HOME}/tools"/*/TOOL.md; do
    [[ -f "$manifest" ]] || continue
    manifest_has_system_tag "$manifest" || continue
    tool_dir="$(dirname "$manifest")"
    bin_dir="${tool_dir}/bin"
    [[ -d "$bin_dir" ]] || continue
    while IFS= read -r -d '' file; do
      cp -f "$file" "${RUNTIME_BIN_DIR}/$(basename "$file")"
      chmod +x "${RUNTIME_BIN_DIR}/$(basename "$file")" 2>/dev/null || true
      copied=1
    done < <(find "$bin_dir" -maxdepth 1 -type f -print0)
  done
  if [[ "$copied" == "1" ]]; then
    log "Stage: projected system tool bins to ${RUNTIME_BIN_DIR}"
  fi
}

install_builtin_tools() {
  [[ -d "$OPENBRAIN_TOOLS_SOURCE_ROOT" ]] || {
    die "local tools source root not found: ${OPENBRAIN_TOOLS_SOURCE_ROOT}"
  }

  local tool_src tool_name tool_dest
  mkdir -p "${OP_HOME}/tools"
  for tool_src in "${OPENBRAIN_TOOLS_SOURCE_ROOT}"/*; do
    [[ -d "$tool_src" && -f "${tool_src}/TOOL.md" ]] || continue
    tool_name="$(basename "$tool_src")"
    tool_dest="${OP_HOME}/tools/${tool_name}"
    log "Stage: install tool ${tool_name} (${tool_src})"
    rm -rf "$tool_dest"
    cp -R "$tool_src" "$tool_dest"
  done
  project_system_tool_bins
}

build_and_install_coder_agent() {
  local tmp_bin
  tmp_bin="${CODER_AGENT_BIN}.tmp"

  [[ -d "${OPENBRAIN_CODER_AGENT_SOURCE_ROOT}/cmd/coder" ]] || {
    die "local coder source not found: ${OPENBRAIN_CODER_AGENT_SOURCE_ROOT}/cmd/coder"
  }
  [[ -f "${OPENBRAIN_CODER_AGENT_SOURCE_ROOT}/.agent/AGENT.md" ]] || {
    die "local coder manifest not found: ${OPENBRAIN_CODER_AGENT_SOURCE_ROOT}/.agent/AGENT.md"
  }

  rm -rf "${CODER_AGENT_DIR}/bin"
  mkdir -p "${CODER_AGENT_DIR}/.agent/bin"
  rm -f "$tmp_bin"

  log "Stage: build local coder agent (${OPENBRAIN_CODER_AGENT_SOURCE_ROOT})"
  (
    cd "$OPENBRAIN_CODER_AGENT_SOURCE_ROOT"
    go build -trimpath -o "$tmp_bin" ./cmd/coder
  )
  chmod +x "$tmp_bin"
  mv -f "$tmp_bin" "$CODER_AGENT_BIN"
  cp -f "${OPENBRAIN_CODER_AGENT_SOURCE_ROOT}/.agent/AGENT.md" "$CODER_AGENT_MD"
}

remove_legacy_opagent_agent() {
  if [[ -d "$LEGACY_OPAGENT_AGENT_DIR" ]]; then
    log "Stage: remove legacy opagent agent (${LEGACY_OPAGENT_AGENT_DIR})"
    rm -rf "$LEGACY_OPAGENT_AGENT_DIR"
  fi
}

install_simple_memory_agent() {
  [[ -f "${OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT}/.agent/AGENT.md" ]] || {
    die "local simple-memory manifest not found: ${OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT}/.agent/AGENT.md"
  }
  rm -rf "$SIMPLE_MEMORY_DIR"
  mkdir -p "${SIMPLE_MEMORY_DIR}/.agent"
  cp -f "${OPENBRAIN_SIMPLE_MEMORY_SOURCE_ROOT}/.agent/AGENT.md" "$SIMPLE_MEMORY_MD"
}

build_and_install_cloud_sync_skill() {
  local tmp_bin
  tmp_bin="${CLOUD_SYNC_SKILL_BIN}.tmp"

  [[ -d "${OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT}/cmd/openbrain-cloud-sync-helper" ]] || {
    die "local OpenBrain Cloud Sync helper source not found: ${OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT}/cmd/openbrain-cloud-sync-helper"
  }
  [[ -f "${OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT}/SKILL.md" ]] || {
    die "local OpenBrain Cloud Sync skill not found: ${OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT}/SKILL.md"
  }

  mkdir -p "${CLOUD_SYNC_SKILL_DIR}/bin"
  rm -f "$tmp_bin"

  log "Stage: build local OpenBrain Cloud Sync skill (${OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT})"
  (
    cd "$OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT"
    go build -trimpath -o "$tmp_bin" ./cmd/openbrain-cloud-sync-helper
  )
  chmod +x "$tmp_bin"
  mv -f "$tmp_bin" "$CLOUD_SYNC_SKILL_BIN"
  cp -f "${OPENBRAIN_CLOUD_SYNC_SKILL_SOURCE_ROOT}/SKILL.md" "$CLOUD_SYNC_SKILL_MD"
  rm -rf "${OP_HOME}/agents/openbrain-sync"
}

sync_gbrain_agent() {
  [[ -f "${OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT}/.agent/AGENT.md" ]] || {
    die "local gbrain agent manifest not found: ${OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT}/.agent/AGENT.md"
  }

  log "Stage: sync gbrain agent (${OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT})"
  rm -rf "${OP_HOME}/agents/brain"
  rm -rf "$GBRAIN_AGENT_DIR"
  mkdir -p "$(dirname "$GBRAIN_AGENT_DIR")"
  cp -R "$OPENBRAIN_GBRAIN_AGENT_SOURCE_ROOT" "$GBRAIN_AGENT_DIR"
  [[ -f "$GBRAIN_AGENT_MD" ]] || die "gbrain agent install missing manifest: ${GBRAIN_AGENT_MD}"
}

ensure_default_workspace() {
  mkdir -p "${DEFAULT_WORKSPACE_DIR}/.agent"
  if [[ -f "$DEFAULT_WORKSPACE_AGENT_MD" ]]; then
    return 0
  fi

  log "Stage: initialize default workspace (${DEFAULT_WORKSPACE_DIR})"
  cat > "$DEFAULT_WORKSPACE_AGENT_MD" <<'EOF_AGENT'
---
bind: @agent-coder
---
EOF_AGENT
}

build_and_install_openbrain_server() {
  local platform dist_root built_bin exe_suffix
  platform="${SERVER_PLATFORM:-$(server_platform_key "$(platform_key)")}"
  exe_suffix=""
  if [[ "$platform" == windows-* ]]; then
    exe_suffix=".exe"
    SERVER_BIN="${SERVER_AGENT_DIR}/.agent/bin/openbrain-server.exe"
  fi
  dist_root="${SERVER_DIST_BASE}/${platform}"
  built_bin="${dist_root}/agents/openbrain-server/bin/openbrain-server${exe_suffix}"

  log "Stage: build OpenBrain server (${platform})"
  OPENBRAIN_SERVER_DIST_ROOT="$dist_root" bash "${SERVER_ROOT}/scripts/build.sh" "$platform"
  [[ -x "$built_bin" ]] || die "server build did not produce ${built_bin}"

  rm -rf "${SERVER_AGENT_DIR}/bin"
  mkdir -p "${SERVER_AGENT_DIR}/.agent/bin"
  rm -rf "$LEGACY_OPENBRAIN_SERVER_AGENT_DIR"
  cp -f "$built_bin" "$SERVER_BIN"
  chmod +x "$SERVER_BIN" 2>/dev/null || true
  cat > "$SERVER_AGENT_MD" <<EOF_AGENT
---
id: agent-openbrain-server
name: openbrain-server
description: openbrain server (ws + chat) for workspace access
tags: builtin,server,system
opcodes: system/started, notify/message, system/config/get
run:
  command: ["./bin/openbrain-server${exe_suffix}", "--host", "127.0.0.1", "--port", "19530"]
  daemon: true
---
EOF_AGENT
}

start_runtime_detached() {
  mkdir -p "$RUN_DIR" "$RUNTIME_LOG_DIR"
  stop_existing_runtime

  log "Stage: start opagent-runtime"
  nohup "$RUNTIME_BIN" --base-dir "$OP_HOME" </dev/null >>"$RUNTIME_LOG_FILE" 2>&1 &
  local pid=$!
  printf '%s\n' "$pid" > "$RUNTIME_PIDFILE"
  disown "$pid" 2>/dev/null || true
  log "opagent-runtime started (pid ${pid}), log: ${RUNTIME_LOG_FILE}"
}

runtime_healthy() {
  curl --silent --show-error --fail --max-time "$RUNTIME_CURL_MAX_TIME_SECS" "$RUNTIME_HEALTH_URL" >/dev/null 2>&1
}

wait_for_runtime_health() {
  log "Stage: runtime health check"
  log "Health URL: ${RUNTIME_HEALTH_URL}"
  for ((attempt = 1; attempt <= RUNTIME_WAIT_ATTEMPTS; attempt += 1)); do
    if runtime_healthy; then
      log "Runtime is healthy"
      return 0
    fi
    sleep "$RUNTIME_WAIT_INTERVAL_SECS"
  done
  tail -n 120 "$RUNTIME_LOG_FILE" 2>/dev/null || true
  die "runtime did not become healthy after ${RUNTIME_WAIT_ATTEMPTS} attempts"
}

ensure_desktop_dependencies() {
  [[ -d "$DESKTOP_ROOT" ]] || die "desktop directory not found: ${DESKTOP_ROOT}"
  if [[ -x "${DESKTOP_ROOT}/node_modules/.bin/tsc" && -x "${DESKTOP_ROOT}/node_modules/.bin/vite" ]]; then
    return 0
  fi
  require_cmd npm
  log "Stage: install desktop dependencies"
  (cd "$DESKTOP_ROOT" && npm ci)
}

sync_desktop_settings() {
  log "Stage: sync desktop settings"
  bash "${DESKTOP_ROOT}/scripts/sync-settings.sh"
}

ensure_desktop_app_icon() {
  local icon_svg="${DESKTOP_ROOT}/build/icon.svg"
  local icon_png="${DESKTOP_ROOT}/build/icon.png"
  local icon_script="${DESKTOP_ROOT}/scripts/generate-app-icon.sh"

  [[ -f "$icon_svg" && -x "$icon_script" ]] || return 0
  if [[ ! -f "$icon_png" || "$icon_svg" -nt "$icon_png" ]]; then
    log "Stage: regenerate desktop app icon"
    bash "$icon_script"
  fi
}

probe_vite_url() {
  local base_url="$1"
  curl --silent --show-error --fail --max-time "$VITE_READY_CURL_MAX_TIME_SECS" "${base_url}${VITE_READY_PATH}" >/dev/null 2>&1
}

wait_for_vite() {
  log "Waiting for Vite dev server"
  for ((attempt = 1; attempt <= VITE_READY_ATTEMPTS; attempt += 1)); do
    local candidate
    for candidate in "${VITE_CANDIDATE_URLS[@]}"; do
      if probe_vite_url "$candidate"; then
        DEV_SERVER_URL="$candidate"
        log "Detected Vite dev server URL: ${DEV_SERVER_URL}"
        return 0
      fi
    done
    sleep "$VITE_READY_INTERVAL_SECS"
  done
  die "timed out waiting for Vite dev server after ${VITE_READY_ATTEMPTS} attempts"
}

start_desktop_watchers_and_electron() {
  local tsc_bin vite_bin electron_bin
  local electron_extra_args=()
  mkdir -p "$RUN_DIR"
  stop_existing_desktop_session
  printf '%s\n' "$$" > "$DESKTOP_SESSION_PIDFILE"
  sync_desktop_settings
  ensure_desktop_app_icon

  cd "$DESKTOP_ROOT"
  tsc_bin="${DESKTOP_ROOT}/node_modules/.bin/tsc"
  vite_bin="${DESKTOP_ROOT}/node_modules/.bin/vite"
  [[ -x "$tsc_bin" && -x "$vite_bin" ]] || die "missing desktop node_modules binaries"
  electron_bin="$(node -e "process.stdout.write(require('electron'))")"
  [[ -n "$electron_bin" && -x "$electron_bin" ]] || die "cannot resolve Electron binary from local dependencies"

  log "Stage: build desktop main"
  "$tsc_bin" -p tsconfig.main.json

  log "Starting dev:main (tsc --watch)"
  "$tsc_bin" -p tsconfig.main.json --watch &
  DEV_MAIN_PID=$!
  printf '%s\n' "$DEV_MAIN_PID" > "$DESKTOP_MAIN_PIDFILE"

  log "Starting dev:renderer (vite on 127.0.0.1:${VITE_PORT})"
  "$vite_bin" --host 127.0.0.1 --port "$VITE_PORT" --strictPort &
  DEV_RENDERER_PID=$!
  printf '%s\n' "$DEV_RENDERER_PID" > "$DESKTOP_RENDERER_PIDFILE"

  wait_for_vite

  log "Starting Electron"
  if [[ -n "${OPENBRAIN_ELECTRON_ARGS:-}" ]]; then
    # shellcheck disable=SC2206
    electron_extra_args=(${OPENBRAIN_ELECTRON_ARGS})
    log "Electron extra args: ${OPENBRAIN_ELECTRON_ARGS}"
    OPENBRAIN_DEV_SERVER_URL="$DEV_SERVER_URL" "$electron_bin" "${electron_extra_args[@]}" "$DESKTOP_ROOT" &
  else
    OPENBRAIN_DEV_SERVER_URL="$DEV_SERVER_URL" "$electron_bin" "$DESKTOP_ROOT" &
  fi
  ELECTRON_PID=$!
  printf '%s\n' "$ELECTRON_PID" > "$DESKTOP_ELECTRON_PIDFILE"

  wait "$ELECTRON_PID"
}

cleanup() {
  local exit_code=$?
  trap - INT TERM EXIT
  [[ -n "$ELECTRON_PID" ]] && stop_pid "$ELECTRON_PID" 'Electron'
  [[ -n "$DEV_RENDERER_PID" ]] && stop_pid "$DEV_RENDERER_PID" 'renderer watcher'
  [[ -n "$DEV_MAIN_PID" ]] && stop_pid "$DEV_MAIN_PID" 'main watcher'
  stop_pidfile "$DESKTOP_ELECTRON_PIDFILE" 'Electron'
  stop_pidfile "$DESKTOP_RENDERER_PIDFILE" 'renderer watcher'
  stop_pidfile "$DESKTOP_MAIN_PIDFILE" 'main watcher'
  stop_desktop_orphans
  clear_pidfile_if_matches "$DESKTOP_ELECTRON_PIDFILE" "$ELECTRON_PID"
  clear_pidfile_if_matches "$DESKTOP_RENDERER_PIDFILE" "$DEV_RENDERER_PID"
  clear_pidfile_if_matches "$DESKTOP_MAIN_PIDFILE" "$DEV_MAIN_PID"
  clear_pidfile_if_matches "$DESKTOP_SESSION_PIDFILE" "$$"
  exit "$exit_code"
}

main() {
  if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi

  require_cmd curl
  require_cmd go
  require_cmd node

  trap cleanup INT TERM EXIT
  build_and_install_local_runtime
  ensure_runtime_config
  build_and_install_gbrain
  install_builtin_tools
  build_and_install_coder_agent
  remove_legacy_opagent_agent
  install_simple_memory_agent
  build_and_install_cloud_sync_skill
  sync_gbrain_agent
  ensure_default_workspace
  build_and_install_openbrain_server
  start_runtime_detached
  wait_for_runtime_health
  ensure_desktop_dependencies
  start_desktop_watchers_and_electron
}

main "$@"
