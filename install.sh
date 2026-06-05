#!/usr/bin/env bash
#
# Traffic Monitor installer for Ubuntu/Debian (systemd).
#
# Place the built binary at /tmp/Traffic-monitor-amd64 and this script at
# /root/install.sh, then run one of:
#
#   bash install.sh install          # interactive install (random port + secret path)
#   bash install.sh update           # replace binary only (keeps data + config)
#   bash install.sh uninstall        # remove service (optionally delete data)
#   bash install.sh reset-password   # set a new admin password
#   bash install.sh set-web-path [P] # change/regenerate the secret web path
#   bash install.sh                  # interactive menu
#
set -euo pipefail

BIN_NAME="Traffic-monitor-amd64"
BIN_SRC="/tmp/${BIN_NAME}"
BIN_DST="/usr/local/bin/${BIN_NAME}"
SERVICE="traffic-monitor"
SVC_USER="traffic-monitor"
DATA_DIR="/var/lib/traffic-monitor"
DB_PATH="${DATA_DIR}/traffic.db"
CONFIG_DIR="/etc/traffic-monitor"
ENV_FILE="${CONFIG_DIR}/traffic-monitor.env"
UNIT_FILE="/etc/systemd/system/${SERVICE}.service"

# Options that may be set via flags to skip the matching prompt/auto-pick.
OPT_IFACE=""
OPT_BIND=""
OPT_PORT=""
OPT_PASSWORD=""
OPT_WEB_PATH=""
POSITIONAL=()

if [ -t 1 ]; then
  C_RESET=$'\e[0m'; C_INFO=$'\e[36m'; C_OK=$'\e[32m'; C_WARN=$'\e[33m'; C_ERR=$'\e[31m'; C_BOLD=$'\e[1m'
else
  C_RESET=""; C_INFO=""; C_OK=""; C_WARN=""; C_ERR=""; C_BOLD=""
fi
info() { printf '%s==>%s %s\n' "$C_INFO" "$C_RESET" "$*"; }
ok()   { printf '%s[ok]%s %s\n' "$C_OK" "$C_RESET" "$*"; }
warn() { printf '%s[warn]%s %s\n' "$C_WARN" "$C_RESET" "$*" >&2; }
die()  { printf '%serror:%s %s\n' "$C_ERR" "$C_RESET" "$*" >&2; exit 1; }

require_root() { [ "$(id -u)" -eq 0 ] || die "this command must be run as root (use sudo)"; }

run_as_user() {
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$SVC_USER" -- "$@"
  else
    sudo -u "$SVC_USER" -- "$@"
  fi
}

default_iface() { ip route show default 2>/dev/null | awk '/default/ {print $5; exit}'; }

list_ifaces() {
  local d n
  for d in /sys/class/net/*; do
    n="$(basename "$d")"
    [ "$n" = "lo" ] && continue
    echo "$n"
  done
}

# --- config helpers ---------------------------------------------------------

get_config_value() {
  local key="$1"
  [ -f "$ENV_FILE" ] || return 1
  sed -n "s/^${key}=//p" "$ENV_FILE" | head -n1
}

set_config_value() {
  local key="$1" val="$2"
  [ -f "$ENV_FILE" ] || die "config not found: ${ENV_FILE}"
  if grep -qE "^${key}=" "$ENV_FILE"; then
    sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
  else
    printf '%s=%s\n' "$key" "$val" >> "$ENV_FILE"
  fi
}

# --- port / web-path helpers ------------------------------------------------

port_in_use() {
  local p="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltnH 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${p}$"
  elif command -v netstat >/dev/null 2>&1; then
    netstat -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${p}$"
  else
    return 1 # cannot check; assume free
  fi
}

pick_free_port() {
  local p tries=0
  while [ "$tries" -lt 200 ]; do
    if command -v shuf >/dev/null 2>&1; then
      p="$(shuf -i 10000-65535 -n 1)"
    else
      p=$(( (RANDOM * 32768 + RANDOM) % 55536 + 10000 ))
    fi
    if ! port_in_use "$p"; then
      echo "$p"
      return 0
    fi
    tries=$((tries + 1))
  done
  die "could not find a free port after many attempts"
}

gen_web_path() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 8
  elif [ -r /dev/urandom ]; then
    # Read a FINITE block first (so the producer gets EOF, not SIGPIPE which would
    # abort under `set -o pipefail`), filter, then take 16 chars with cut (cut
    # reads all input, so it never closes the pipe early either).
    head -c 512 /dev/urandom | LC_ALL=C tr -dc 'a-z0-9' | cut -c1-16
  else
    printf 'p%s%s' "$(date +%s)" "$$"
  fi
}

sanitize_web_path() {
  # Keep a single URL-safe path segment.
  printf '%s' "$1" | tr -cd 'A-Za-z0-9._~-'
}

prompt_password() {
  local p1 p2
  while :; do
    read -r -s -p "Admin password: " p1; echo >&2
    [ -n "$p1" ] || { warn "password must not be empty"; continue; }
    read -r -s -p "Confirm password: " p2; echo >&2
    [ "$p1" = "$p2" ] || { warn "passwords do not match — try again"; continue; }
    printf '%s' "$p1"
    return 0
  done
}

print_url() {
  local listen path port ip seg=""
  listen="$(get_config_value TM_LISTEN || echo '')"
  path="$(get_config_value TM_BASE_PATH || echo '')"
  port="${listen##*:}"
  [ -n "$path" ] && seg="${path}/"
  ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  ok "Open: http://${ip:-<server-ip>}:${port}/${seg}"
}

write_unit() {
  local port="$1" netbind=""
  # Low ports also need CAP_NET_BIND_SERVICE; traffic shaping always needs
  # CAP_NET_ADMIN (to run tc/ip on the monitored interface).
  if [ "$port" -lt 1024 ] 2>/dev/null; then
    netbind=" CAP_NET_BIND_SERVICE"
  fi
  cat > "$UNIT_FILE" <<EOF
[Unit]
Description=Traffic Monitor
Documentation=https://github.com/Kup1ng/Traffic-monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SVC_USER}
Group=${SVC_USER}
EnvironmentFile=${ENV_FILE}
# Load the ifb module (used for ingress shaping) as root before privileges are
# dropped; "-+" runs it privileged and tolerates a built-in/absent module.
ExecStartPre=-+/sbin/modprobe ifb
ExecStart=${BIN_DST} serve
Restart=on-failure
RestartSec=3
# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectControlGroups=true
ReadWritePaths=${DATA_DIR}
# CAP_NET_ADMIN: apply the bandwidth limit via tc/ip on the monitored interface.
AmbientCapabilities=CAP_NET_ADMIN${netbind}
CapabilityBoundingSet=CAP_NET_ADMIN${netbind}

[Install]
WantedBy=multi-user.target
EOF
}

cmd_install() {
  require_root
  [ -f "$BIN_SRC" ] || die "binary not found at ${BIN_SRC} — copy the built ${BIN_NAME} there first"

  # Stop a running instance first so the new binary/config and set-password are
  # applied cleanly (and the database is never open by two processes at once).
  systemctl stop "$SERVICE" 2>/dev/null || true

  # Reuse an existing install's port, secret path, and interface so reinstall is
  # stable.
  local ex_listen ex_path ex_iface
  ex_listen="$(get_config_value TM_LISTEN || echo '')"
  ex_path="$(get_config_value TM_BASE_PATH || echo '')"
  ex_iface="$(get_config_value TM_INTERFACE || echo '')"

  # Interface (default to the existing one on reinstall — history is tied to it).
  local def def_iface
  def="$(default_iface || true)"
  def_iface="${ex_iface:-${def:-eth0}}"
  if [ -z "$OPT_IFACE" ]; then
    echo "Available interfaces:"
    while read -r n; do
      if [ "$n" = "$def" ]; then echo "  ${n}  (default route)"; else echo "  ${n}"; fi
    done < <(list_ifaces)
    read -r -p "Interface to monitor [${def_iface}]: " OPT_IFACE || true
    OPT_IFACE="${OPT_IFACE:-${def_iface}}"
  fi
  # Changing the monitored interface while keeping history would crash-loop the
  # service (the stored counter state is tied to the original interface).
  if [ -n "$ex_iface" ] && [ "$OPT_IFACE" != "$ex_iface" ] && [ -f "$DB_PATH" ]; then
    die "this install monitors '${ex_iface}' and has existing history; changing it to '${OPT_IFACE}' would crash-loop. Keep '${ex_iface}', or run 'uninstall' (deleting data) first."
  fi

  # Bind address.
  if [ -z "$OPT_BIND" ]; then
    if [ -n "$ex_listen" ]; then OPT_BIND="${ex_listen%:*}"; else OPT_BIND="0.0.0.0"; fi
  fi

  # Port: flag > existing (persistent) > random free 5-digit.
  if [ -z "$OPT_PORT" ]; then
    if [ -n "$ex_listen" ]; then
      OPT_PORT="${ex_listen##*:}"
      info "Reusing existing port ${OPT_PORT}"
    else
      OPT_PORT="$(pick_free_port)"
      info "Selected random free port ${OPT_PORT}"
    fi
  fi

  # Secret web path: flag > existing (persistent) > generated.
  if [ -z "$OPT_WEB_PATH" ]; then
    if [ -n "$ex_path" ]; then
      OPT_WEB_PATH="$ex_path"
      info "Reusing existing secret web path"
    else
      OPT_WEB_PATH="$(gen_web_path)"
      info "Generated secret web path"
    fi
  fi
  OPT_WEB_PATH="$(sanitize_web_path "$OPT_WEB_PATH")"
  printf '%s' "$OPT_WEB_PATH" | grep -q '[A-Za-z0-9]' || \
    die "secret web path must contain letters or digits (got '${OPT_WEB_PATH}')"

  # Password.
  [ -n "$OPT_PASSWORD" ] || OPT_PASSWORD="$(prompt_password)"

  info "Creating service user '${SVC_USER}'"
  id -u "$SVC_USER" >/dev/null 2>&1 || \
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SVC_USER"

  info "Creating directories"
  mkdir -p "$DATA_DIR" "$CONFIG_DIR"
  chown "$SVC_USER:$SVC_USER" "$DATA_DIR"
  chmod 750 "$DATA_DIR"

  # The ingress half of the bandwidth limit redirects traffic to an ifb device,
  # so the ifb kernel module must be available. Load it at every boot and now.
  info "Enabling the ifb kernel module (for ingress shaping)"
  mkdir -p /etc/modules-load.d
  echo "ifb" > /etc/modules-load.d/traffic-monitor.conf
  modprobe ifb 2>/dev/null || true

  info "Installing binary -> ${BIN_DST}"
  install -m 0755 "$BIN_SRC" "$BIN_DST"

  info "Writing config -> ${ENV_FILE}"
  cat > "$ENV_FILE" <<EOF
# Traffic Monitor configuration (loaded by systemd).
TM_INTERFACE=${OPT_IFACE}
TM_LISTEN=${OPT_BIND}:${OPT_PORT}
TM_BASE_PATH=${OPT_WEB_PATH}
TM_DB=${DB_PATH}
# TM_TZ=
# TM_POLL_INTERVAL=2s
# TM_FLUSH_INTERVAL=60s
# TM_SESSION_TTL=168h
# TM_COOKIE_SECURE=auto
EOF
  chown "root:${SVC_USER}" "$ENV_FILE"
  chmod 640 "$ENV_FILE"

  info "Setting admin password"
  printf '%s' "$OPT_PASSWORD" | run_as_user "$BIN_DST" set-password --db "$DB_PATH" --stdin >/dev/null

  info "Writing systemd unit -> ${UNIT_FILE}"
  write_unit "$OPT_PORT"
  systemctl daemon-reload
  systemctl enable "$SERVICE"
  # restart (not `enable --now`): start if stopped, and re-exec with the new
  # binary/config if it was already running — `start` on an active unit is a no-op.
  systemctl restart "$SERVICE"

  echo
  ok "Installed and started."
  systemctl --no-pager --full status "$SERVICE" 2>/dev/null | head -n 5 || true
  echo
  ok "Random port:     ${OPT_PORT}"
  ok "Secret web path: ${OPT_WEB_PATH}"
  print_url
  ok "Set a hard bandwidth limit (Mbps) for ${OPT_IFACE} anytime from the dashboard footer."
  warn "The whole app lives ONLY under that secret path; the root URL returns 404."
  warn "There is no TLS — expose it only on a trusted/LAN/VPN network, or behind an HTTPS reverse proxy."
}

cmd_update() {
  require_root
  [ -f "$BIN_SRC" ] || die "binary not found at ${BIN_SRC} — copy the new ${BIN_NAME} there first"
  [ -f "$UNIT_FILE" ] || warn "service unit not found — has it been installed?"

  info "Stopping service"
  systemctl stop "$SERVICE" 2>/dev/null || true
  info "Replacing binary (database, port, and secret path are left untouched)"
  install -m 0755 "$BIN_SRC" "$BIN_DST"
  info "Starting service"
  systemctl start "$SERVICE"

  echo
  ok "Updated. All history and settings (including port and secret path) were preserved."
  systemctl --no-pager --full status "$SERVICE" 2>/dev/null | head -n 5 || true
  print_url
}

cmd_uninstall() {
  require_root
  info "Stopping and disabling service"
  systemctl disable --now "$SERVICE" 2>/dev/null || true

  # The service clears its own tc rules on a clean stop, but a crash/SIGKILL
  # could leave them behind — and we're about to delete the binary that knows
  # how to undo them. Tear any leftovers down explicitly (best-effort) using the
  # interface from the config (still present at this point).
  local un_iface
  un_iface="$(get_config_value TM_INTERFACE 2>/dev/null || echo '')"
  if [ -n "$un_iface" ]; then
    tc qdisc del dev "$un_iface" root 2>/dev/null || true
    tc qdisc del dev "$un_iface" ingress 2>/dev/null || true
  fi
  ip link del tm-ifb0 2>/dev/null || true

  rm -f "$UNIT_FILE"
  rm -f /etc/modules-load.d/traffic-monitor.conf
  systemctl daemon-reload
  rm -f "$BIN_DST"
  ok "Service, unit, and binary removed (interface returned to unshaped)."

  local ans
  read -r -p "Also delete the database and config (${DATA_DIR}, ${CONFIG_DIR})? [y/N]: " ans || true
  if [[ "${ans:-}" =~ ^[Yy]$ ]]; then
    rm -rf "$DATA_DIR" "$CONFIG_DIR"
    userdel "$SVC_USER" 2>/dev/null || true
    ok "Database, config, and service user removed."
  else
    info "Kept ${DATA_DIR} and ${CONFIG_DIR} — history, port, and secret path are preserved."
  fi
}

cmd_reset_password() {
  require_root
  [ -x "$BIN_DST" ] || die "binary not found at ${BIN_DST} — install first"
  [ -f "$DB_PATH" ] || die "database not found at ${DB_PATH} — install first"
  local pw; pw="$(prompt_password)"
  printf '%s' "$pw" | run_as_user "$BIN_DST" set-password --db "$DB_PATH" --stdin >/dev/null
  ok "Admin password updated."
}

cmd_set_web_path() {
  require_root
  [ -f "$ENV_FILE" ] || die "not installed (config not found at ${ENV_FILE})"
  local newpath="${OPT_WEB_PATH:-${POSITIONAL[0]:-}}"
  if [ -z "$newpath" ]; then
    newpath="$(gen_web_path)"
  fi
  newpath="$(sanitize_web_path "$newpath")"
  printf '%s' "$newpath" | grep -q '[A-Za-z0-9]' || \
    die "web path must contain letters or digits (got '${newpath}')"
  set_config_value TM_BASE_PATH "$newpath"
  # Rotate the session secret so previously-issued tokens are truly invalidated
  # (the cookie Path change alone does not invalidate a captured/replayed token).
  if run_as_user "$BIN_DST" rotate-secret --db "$DB_PATH" >/dev/null 2>&1; then
    local rotated=1
  else
    local rotated=0
    warn "could not rotate the session secret; existing tokens stay valid until they expire"
  fi
  systemctl restart "$SERVICE" 2>/dev/null || true
  ok "Secret web path updated to: ${newpath}"
  [ "$rotated" = 1 ] && ok "Session secret rotated — all existing sessions are now invalid."
  print_url
}

menu() {
  echo "${C_BOLD}Traffic Monitor — installer${C_RESET}"
  echo "  1) Install"
  echo "  2) Update binary (keeps data + config)"
  echo "  3) Uninstall"
  echo "  4) Reset admin password"
  echo "  5) Change/regenerate secret web path"
  echo "  6) Exit"
  local choice
  read -r -p "Choose [1-6]: " choice || true
  case "${choice:-}" in
    1) cmd_install ;;
    2) cmd_update ;;
    3) cmd_uninstall ;;
    4) cmd_reset_password ;;
    5) cmd_set_web_path ;;
    6) exit 0 ;;
    *) die "invalid choice" ;;
  esac
}

usage() {
  cat <<EOF
Traffic Monitor installer

Usage: install.sh <command> [options]

Commands:
  install            Install the service (auto-picks a random free 5-digit port
                     and a secret web path; interface and password are prompted)
  update             Replace the binary from ${BIN_SRC} and restart
                     (keeps the database, port, and secret path)
  uninstall          Stop and remove the service (optionally delete data/config)
  reset-password     Set a new admin web-panel password
  set-web-path [P]   Change the secret web path to P (or regenerate if omitted)
  (no command)       Show an interactive menu

Install options (skip the matching prompt/auto-pick):
  --interface NAME   Interface to monitor (default: the default-route interface)
  --bind ADDR        Listen address (default: 0.0.0.0)
  --port N           Listen port (default: a random free 5-digit port)
  --web-path PATH    Secret base path (default: randomly generated)
  --password PASS    Admin password (prefer the interactive prompt on shared shells)

The whole app (UI, API, login, SSE) is served only under the secret web path:
  http://<server-ip>:<port>/<web-path>/   — the root URL returns 404.

The built binary must be present at ${BIN_SRC} before install/update.
EOF
}

main() {
  local sub="${1:-}"
  [ $# -gt 0 ] && shift || true
  while [ $# -gt 0 ]; do
    case "$1" in
      --interface) OPT_IFACE="${2:-}"; shift 2 ;;
      --bind)      OPT_BIND="${2:-}"; shift 2 ;;
      --port)      OPT_PORT="${2:-}"; shift 2 ;;
      --web-path)  OPT_WEB_PATH="${2:-}"; shift 2 ;;
      --password)  OPT_PASSWORD="${2:-}"; shift 2 ;;
      -h|--help)   usage; exit 0 ;;
      --*)         die "unknown option: $1" ;;
      *)           POSITIONAL+=("$1"); shift ;;
    esac
  done

  case "$sub" in
    install)        cmd_install ;;
    update)         cmd_update ;;
    uninstall)      cmd_uninstall ;;
    reset-password) cmd_reset_password ;;
    set-web-path)   cmd_set_web_path ;;
    ""|menu)        menu ;;
    -h|--help|help) usage ;;
    *) die "unknown command: ${sub} (use install | update | uninstall | reset-password | set-web-path)" ;;
  esac
}

main "$@"
