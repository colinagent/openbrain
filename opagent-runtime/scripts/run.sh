#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "${ROOT_DIR}/.." && pwd)"
BUILD_AGENT_SCRIPT="${REPO_ROOT}/scripts/build-agent.sh"
OP_HOME="${HOME}/.openbrain"
CONFIG_SRC="${ROOT_DIR}/configs"
CONFIG_DEST="${OP_HOME}/configs"
BIN_DIR="${OP_HOME}/bin"
BIN_PATH="${BIN_DIR}/opagent-runtime"
TMP_BIN_PATH="${BIN_PATH}.tmp"
LOG_FILE="${OP_HOME}/logs/opagent-runtime/opagent-runtime.log"
HEALTH_URL="http://127.0.0.1:19530/health"
HEALTH_WAIT_ATTEMPTS=60
HEALTH_WAIT_INTERVAL_SECS=0.5

mkdir -p "${CONFIG_DEST}" "${BIN_DIR}" "${OP_HOME}/logs/opagent-runtime" "${OP_HOME}/agents" "${OP_HOME}/data"

cp "${CONFIG_SRC}/config.json" "${CONFIG_DEST}/config.json"
if [ -f "${CONFIG_SRC}/secrets.env.example" ]; then
  cp "${CONFIG_SRC}/secrets.env.example" "${CONFIG_DEST}/secrets.env.example"
fi

if [ ! -f "${CONFIG_DEST}/secrets.env" ]; then
  echo "missing ${CONFIG_DEST}/secrets.env; copy secrets.env.example and fill your keys" >&2
fi

(
  cd "${REPO_ROOT}"
  rm -f "${TMP_BIN_PATH}"
  go build -o "${TMP_BIN_PATH}" ./opagent-runtime/cmd/opagent-runtime
  chmod +x "${TMP_BIN_PATH}"
  mv -f "${TMP_BIN_PATH}" "${BIN_PATH}"
)

if [ -x "${BUILD_AGENT_SCRIPT}" ]; then
  bash "${BUILD_AGENT_SCRIPT}" --agent opagent-server
fi

cleanup_old() {
  local run_dir="${OP_HOME}/run"
  local pidfile="${run_dir}/opagent-runtime.pid"
  if [ -f "${pidfile}" ]; then
    local old_pid
    old_pid="$(cat "${pidfile}" 2>/dev/null || true)"
    if [[ -n "${old_pid}" && "${old_pid}" =~ ^[0-9]+$ ]]; then
      if kill -0 "${old_pid}" 2>/dev/null; then
        echo "Stopping old opagent-runtime (pid ${old_pid})..."
        kill -TERM "${old_pid}" 2>/dev/null || true
        for i in $(seq 1 20); do
          kill -0 "${old_pid}" 2>/dev/null || break
          sleep 0.2
        done
        kill -9 "${old_pid}" 2>/dev/null || true
      fi
    fi
    rm -f "${pidfile}"
  fi

  rm -f \
    "${run_dir}/opagent-server.pid" "${run_dir}/opagent-server.lock" \
    "${run_dir}/systool.pid" "${run_dir}/systool.lock"

  expand_pid_tree() {
    local pid="$1"
    if [[ -z "${pid}" || ! "${pid}" =~ ^[0-9]+$ ]]; then
      return 0
    fi
    printf '%s\n' "${pid}"
    if ! command -v pgrep >/dev/null 2>&1; then
      return 0
    fi
    local child_pid
    while IFS= read -r child_pid; do
      if [[ -z "${child_pid}" || ! "${child_pid}" =~ ^[0-9]+$ ]]; then
        continue
      fi
      expand_pid_tree "${child_pid}"
    done < <(pgrep -P "${pid}" 2>/dev/null || true)
  }

  stop_pid_group() {
    local label="$1"
    shift
    local root_pid
    local expanded_pids=""
    for root_pid in "$@"; do
      if [[ -z "${root_pid}" || ! "${root_pid}" =~ ^[0-9]+$ ]]; then
        continue
      fi
      expanded_pids="${expanded_pids}"$'\n'"$(expand_pid_tree "${root_pid}")"
    done
    local target_pids
    target_pids="$(printf '%s\n' "${expanded_pids}" | awk '/^[0-9]+$/ { if (!seen[$0]++) print $0 }')"
    if [ -z "${target_pids}" ]; then
      return 0
    fi
    echo "Stopping stray ${label} pids: $(echo "${target_pids}" | tr '\n' ' ')"
    kill -TERM ${target_pids} 2>/dev/null || true
    sleep 1
    kill -KILL ${target_pids} 2>/dev/null || true
  }

  # The pidfile is not enough when older manual runs or crashed children linger.
  if command -v pgrep >/dev/null 2>&1; then
    local host_pids server_pids agent_pids
    host_pids="$(pgrep -f "^${BIN_PATH//\//\\/} --base-dir ${OP_HOME//\//\\/}$" || true)"
    server_pids="$(pgrep -f "^${OP_HOME//\//\\/}/agents/opagent-server/.agent/bin/openbrain-server --host 127.0.0.1 --port 19530$" || true)"
    server_pids="${server_pids}"$'\n'"$(pgrep -f "^${OP_HOME//\//\\/}/agents/opagent-server/bin/openbrain-server --host 127.0.0.1 --port 19530$" || true)"
    agent_pids="$(pgrep -f "^${OP_HOME//\//\\/}/agents/coder/.agent/bin/coder( |$)" || true)"
    if [ -n "${host_pids}" ]; then
      stop_pid_group "opagent-runtime" ${host_pids}
    fi
    if [ -n "${server_pids}" ]; then
      stop_pid_group "opagent-server" ${server_pids}
    fi
    if [ -n "${agent_pids}" ]; then
      stop_pid_group "coder agent" ${agent_pids}
    fi
  fi

  if command -v lsof >/dev/null 2>&1; then
    local listener_pids
    listener_pids="$(lsof -tiTCP:19530 -sTCP:LISTEN 2>/dev/null || true)"
    if [ -n "${listener_pids}" ]; then
      echo "Stopping existing listeners on 127.0.0.1:19530: ${listener_pids}"
      kill -TERM ${listener_pids} 2>/dev/null || true
      sleep 1
      kill -KILL ${listener_pids} 2>/dev/null || true
    fi
  fi
}

start_runtime_detached() {
  local launcher=""
  local -a launch_cmd=()

  if command -v setsid >/dev/null 2>&1; then
    launcher="setsid"
    launch_cmd=(setsid "${BIN_PATH}" --base-dir "${OP_HOME}")
  elif command -v perl >/dev/null 2>&1; then
    launcher="perl-setsid"
    launch_cmd=(
      perl
      -MPOSIX=setsid
      -e
      'setsid() or die "setsid failed: $!"; exec @ARGV or die "exec failed: $!";'
      "${BIN_PATH}"
      --base-dir
      "${OP_HOME}"
    )
  else
    launcher="nohup"
    launch_cmd=("${BIN_PATH}" --base-dir "${OP_HOME}")
  fi

  echo "Starting opagent-runtime in background (${launcher})..."
  nohup "${launch_cmd[@]}" </dev/null >>"${LOG_FILE}" 2>&1 &
  local host_pid=$!
  disown "${host_pid}" 2>/dev/null || true
  echo "opagent-runtime started (pid ${host_pid}), log: ${LOG_FILE}"
}

wait_for_health() {
  if ! command -v curl >/dev/null 2>&1; then
    return 0
  fi
  for _ in $(seq 1 "${HEALTH_WAIT_ATTEMPTS}"); do
    if curl --silent --show-error --fail --max-time 2 "${HEALTH_URL}" >/dev/null 2>&1; then
      echo "opagent-runtime health check passed: ${HEALTH_URL}"
      return 0
    fi
    sleep "${HEALTH_WAIT_INTERVAL_SECS}"
  done
  echo "opagent-runtime failed to become healthy at ${HEALTH_URL}" >&2
  tail -n 80 "${LOG_FILE}" 2>/dev/null || true
  return 1
}

cleanup_old
start_runtime_detached
wait_for_health
