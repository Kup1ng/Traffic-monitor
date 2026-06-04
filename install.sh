#!/usr/bin/env bash
#
# Traffic Monitor installer for Ubuntu/Debian (systemd).
#
# Place the built binary at /tmp/Traffic-monitor-amd64 and this script at
# /root/install.sh, then run one of:
#
#   bash install.sh install          # interactive install
#   bash install.sh update           # replace binary only (keeps data + config)
#   bash install.sh uninstall        # remove service (optionally delete data)
#   bash install.sh reset-password   # set a new admin password
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

# Options that may be set via flags to skip the matching prompt.
OPT_IFACE=""
OPT_BIND="0.0.0.0"
OPT_PORT=""
OPT_PASSWORD=""

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

write_unit() {
  local port="$1" caps=""
  if [ "$port" -lt 1024 ] 2>/dev/null; then
    caps="AmbientCapabilities=CAP_NET_BIND_SERVICE"
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
${caps}

[Install]
WantedBy=multi-user.target
EOF
}

cmd_install() {
  require_root
  [ -f "$BIN_SRC" ] || die "binary not found at ${BIN_SRC} — copy the built ${BIN_NAME} there first"

  local def; def="$(default_iface || true)"
  if [ -z "$OPT_IFACE" ]; then
    echo "Available interfaces:"
    while read -r n; do
      if [ "$n" = "$def" ]; then echo "  ${n}  (default route)"; else echo "  ${n}"; fi
    done < <(list_ifaces)
    read -r -p "Interface to monitor [${def:-eth0}]: " OPT_IFACE || true
    OPT_IFACE="${OPT_IFACE:-${def:-eth0}}"
  fi

  if [ -z "$OPT_PORT" ]; then
    read -r -p "Listen port [8088]: " OPT_PORT || true
    OPT_PORT="${OPT_PORT:-8088}"
  fi

  [ -n "$OPT_PASSWORD" ] || OPT_PASSWORD="$(prompt_password)"

  info "Creating service user '${SVC_USER}'"
  id -u "$SVC_USER" >/dev/null 2>&1 || \
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SVC_USER"

  info "Creating directories"
  mkdir -p "$DATA_DIR" "$CONFIG_DIR"
  chown "$SVC_USER:$SVC_USER" "$DATA_DIR"
  chmod 750 "$DATA_DIR"

  info "Installing binary -> ${BIN_DST}"
  install -m 0755 "$BIN_SRC" "$BIN_DST"

  info "Writing config -> ${ENV_FILE}"
  cat > "$ENV_FILE" <<EOF
# Traffic Monitor configuration (loaded by systemd).
TM_INTERFACE=${OPT_IFACE}
TM_LISTEN=${OPT_BIND}:${OPT_PORT}
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
  systemctl enable --now "$SERVICE"

  echo
  ok "Installed and started."
  systemctl --no-pager --full status "$SERVICE" 2>/dev/null | head -n 5 || true
  local ip; ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  echo
  ok "Open the panel at: http://${ip:-<server-ip>}:${OPT_PORT}"
  warn "The panel has no TLS. Expose it only on a trusted/LAN/VPN network, or put it behind a reverse proxy with HTTPS."
}

cmd_update() {
  require_root
  [ -f "$BIN_SRC" ] || die "binary not found at ${BIN_SRC} — copy the new ${BIN_NAME} there first"
  [ -f "$UNIT_FILE" ] || warn "service unit not found — has it been installed?"

  info "Stopping service"
  systemctl stop "$SERVICE" 2>/dev/null || true
  info "Replacing binary (database and config are left untouched)"
  install -m 0755 "$BIN_SRC" "$BIN_DST"
  info "Starting service"
  systemctl start "$SERVICE"

  echo
  ok "Updated. All history and settings were preserved."
  systemctl --no-pager --full status "$SERVICE" 2>/dev/null | head -n 5 || true
}

cmd_uninstall() {
  require_root
  info "Stopping and disabling service"
  systemctl disable --now "$SERVICE" 2>/dev/null || true
  rm -f "$UNIT_FILE"
  systemctl daemon-reload
  rm -f "$BIN_DST"
  ok "Service, unit, and binary removed."

  local ans
  read -r -p "Also delete the database and config (${DATA_DIR}, ${CONFIG_DIR})? [y/N]: " ans || true
  if [[ "${ans:-}" =~ ^[Yy]$ ]]; then
    rm -rf "$DATA_DIR" "$CONFIG_DIR"
    userdel "$SVC_USER" 2>/dev/null || true
    ok "Database, config, and service user removed."
  else
    info "Kept ${DATA_DIR} and ${CONFIG_DIR} — history and settings are preserved."
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

menu() {
  echo "${C_BOLD}Traffic Monitor — installer${C_RESET}"
  echo "  1) Install"
  echo "  2) Update binary (keeps data + config)"
  echo "  3) Uninstall"
  echo "  4) Reset admin password"
  echo "  5) Exit"
  local choice
  read -r -p "Choose [1-5]: " choice || true
  case "${choice:-}" in
    1) cmd_install ;;
    2) cmd_update ;;
    3) cmd_uninstall ;;
    4) cmd_reset_password ;;
    5) exit 0 ;;
    *) die "invalid choice" ;;
  esac
}

usage() {
  cat <<EOF
Traffic Monitor installer

Usage: install.sh <command> [options]

Commands:
  install            Install the service (interactive; options below skip prompts)
  update             Replace the binary from ${BIN_SRC} and restart (keeps data/config)
  uninstall          Stop and remove the service (optionally delete data/config)
  reset-password     Set a new admin web-panel password
  (no command)       Show an interactive menu

Install options:
  --interface NAME   Interface to monitor (default: the default-route interface)
  --bind ADDR        Listen address (default: 0.0.0.0)
  --port N           Listen port (default: 8088)
  --password PASS    Admin password (prefer the interactive prompt on shared shells)

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
      --password)  OPT_PASSWORD="${2:-}"; shift 2 ;;
      -h|--help)   usage; exit 0 ;;
      *) die "unknown option: $1" ;;
    esac
  done

  case "$sub" in
    install)        cmd_install ;;
    update)         cmd_update ;;
    uninstall)      cmd_uninstall ;;
    reset-password) cmd_reset_password ;;
    ""|menu)        menu ;;
    -h|--help|help) usage ;;
    *) die "unknown command: ${sub} (use install | update | uninstall | reset-password)" ;;
  esac
}

main "$@"
