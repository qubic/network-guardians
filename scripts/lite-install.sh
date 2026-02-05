#!/bin/bash
#
# Lite Node Installer - Docker-based setup
# https://github.com/qubic/core-lite
#
# Usage:
#   Interactive:  ./lite-install.sh
#   CLI:          ./lite-install.sh install --seed <seed> --alias <alias>
#
# Commands:
#   install       Install and start Lite node
#   uninstall     Remove Lite node
#   status        Show container status
#   logs          Show live logs
#   stop          Stop container
#   start         Start container
#   restart       Restart container
#

set -e

# --- Config ---
CONTAINER_NAME="qubic-lite"
DOCKER_IMAGE="qubiccore/lite"
DATA_DIR="/opt/qubic-lite"

# Default ports
P2P_PORT=21841
HTTP_PORT=41841

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[*]${NC} $1"; }
log_ok()    { echo -e "${GREEN}[+]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[-]${NC} $1"; }

# --- Functions ---

print_usage() {
    echo "Lite Node Installer"
    echo ""
    echo "Usage:"
    echo "  Interactive:  $0"
    echo "  CLI:          $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  install       Install and start Lite node"
    echo "  uninstall     Remove Lite node and data"
    echo "  status        Show container status"
    echo "  info          Show node info (tick, epoch, operator)"
    echo "  logs          Show live logs (Ctrl+C to exit)"
    echo "  stop          Stop container"
    echo "  start         Start container"
    echo "  restart       Restart container"
    echo ""
    echo "Install options:"
    echo "  --seed <seed>         Operator seed [REQUIRED]"
    echo "  --alias <alias>       Operator alias [REQUIRED]"
    echo "  --p2p-port <port>     P2P port (default: 21841)"
    echo "  --http-port <port>    HTTP port (default: 41841)"
    echo "  --data-dir <path>     Data directory (default: /opt/qubic-lite)"
    echo ""
    echo "Examples:"
    echo "  $0 install --seed myseed --alias mynode"
    echo "  $0 logs"
    echo "  $0 status"
}

print_security_warning() {
    echo ""
    log_warn "SECURITY TIP: To prevent your seed from being saved in shell history:"
    echo "      - Add a SPACE before the command:  ' ./lite-install.sh install ...'"
    echo "      - Or use interactive mode:  ./lite-install.sh"
    echo "      - Or set: export HISTCONTROL=ignorespace"
    echo ""
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        log_warn "Docker not found. Installing..."
        curl -fsSL https://get.docker.com | sh
        if ! command -v docker &> /dev/null; then
            log_error "Docker installation failed"
            exit 1
        fi
        log_ok "Docker installed"
    fi
    log_ok "Docker: $(docker --version | cut -d' ' -f3 | tr -d ',')"
}

check_system() {
    log_info "Checking system..."

    local ram_kb ram_gb min_ram=64
    ram_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    ram_gb=$((ram_kb / 1024 / 1024))

    if [ "$ram_gb" -lt "$min_ram" ]; then
        log_warn "RAM: ${ram_gb}GB (recommended: ${min_ram}GB+)"
    else
        log_ok "RAM: ${ram_gb}GB"
    fi

    if grep -q avx2 /proc/cpuinfo; then
        log_ok "AVX2: supported"
    else
        log_warn "AVX2: not detected (required for mainnet)"
    fi
}

container_exists() {
    docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"
}

container_running() {
    docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"
}

do_install() {
    log_info "Installing Lite node..."

    check_docker
    check_system

    # Validate inputs
    if [ -z "$OPERATOR_SEED" ]; then
        log_error "--seed is required"
        exit 1
    fi

    if [ -z "$OPERATOR_ALIAS" ]; then
        log_error "--alias is required"
        exit 1
    fi

    # Stop existing containers
    if container_exists; then
        log_info "Removing existing container..."
        docker rm -f "$CONTAINER_NAME" &>/dev/null || true
    fi
    docker rm -f watchtower &>/dev/null || true

    # Create directory
    mkdir -p "${DATA_DIR}"

    # Copy script for management
    cp "$0" "${DATA_DIR}/lite-install.sh" 2>/dev/null || true
    chmod +x "${DATA_DIR}/lite-install.sh" 2>/dev/null || true

    # Pull image
    log_info "Pulling image from Docker Hub..."
    docker pull "${DOCKER_IMAGE}:latest"
    log_ok "Image ready"

    # Create .env file with sensitive data
    cat > "${DATA_DIR}/.env" <<EOF
QUBIC_OPERATOR_SEED=${OPERATOR_SEED}
QUBIC_OPERATOR_ALIAS=${OPERATOR_ALIAS}
EOF
    chmod 600 "${DATA_DIR}/.env"
    log_ok "Config: ${DATA_DIR}/.env"

    # Create docker-compose.yml
    cat > "${DATA_DIR}/docker-compose.yml" <<EOF
services:
  qubic-lite:
    image: ${DOCKER_IMAGE}:latest
    container_name: ${CONTAINER_NAME}
    restart: unless-stopped
    ports:
      - "${P2P_PORT}:21841"
      - "${HTTP_PORT}:41841"
    env_file:
      - .env
    volumes:
      - qubic-lite-data:/qubic

  watchtower:
    image: containrrr/watchtower
    container_name: watchtower
    restart: unless-stopped
    environment:
      DOCKER_API_VERSION: "1.44"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    command: --interval 300 ${CONTAINER_NAME}

volumes:
  qubic-lite-data:
EOF

    # Start containers
    log_info "Starting containers..."
    cd "${DATA_DIR}" && docker compose up -d

    log_ok "Lite node started!"
    echo ""
    echo "  Container:   $CONTAINER_NAME"
    echo "  Config:      ${DATA_DIR}/.env"
    echo "  P2P:         port ${P2P_PORT}"
    echo "  HTTP:        http://localhost:${HTTP_PORT}"
    echo "  Auto-Update: enabled (Watchtower)"
    echo ""
    echo "  View logs:   ./lite-install.sh logs"
    echo "  Status:      ./lite-install.sh status"
    echo ""

    cd "${DATA_DIR}"
}

do_uninstall() {
    log_info "Uninstalling Lite node..."

    # Stop containers
    if [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        docker compose -f "${DATA_DIR}/docker-compose.yml" down -v 2>/dev/null || true
        log_ok "Containers stopped"
    elif container_exists; then
        docker rm -f "$CONTAINER_NAME" &>/dev/null || true
        docker rm -f watchtower &>/dev/null || true
        log_ok "Containers removed"
    fi

    # Ask before removing data
    local data_removed=false
    if [ -d "$DATA_DIR" ]; then
        echo ""
        read -rp "Remove data directory ${DATA_DIR}? [y/N] " confirm
        if [[ "$confirm" =~ ^[Yy]$ ]]; then
            rm -rf "$DATA_DIR"
            log_ok "Data removed"
            data_removed=true
        else
            log_info "Data kept at ${DATA_DIR}"
        fi
    fi

    log_ok "Uninstall complete"

    # Return to home if data dir was removed
    if [ "$data_removed" = true ]; then
        cd ~ && exec bash
    fi
}

do_status() {
    if container_running; then
        log_ok "Lite node is running"
        echo ""
        docker ps --filter "name=${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
        docker ps --filter "name=watchtower" --format "table {{.Names}}\t{{.Status}}" 2>/dev/null || true
        echo ""
        log_info "Checking HTTP endpoint..."
        local response
        response=$(curl -sf --max-time 5 "http://localhost:${HTTP_PORT}/live/v1" 2>/dev/null || true)
        if [ -n "$response" ]; then
            echo "$response" | head -c 500
            echo ""
        else
            log_warn "HTTP not responding on port ${HTTP_PORT}"
        fi
    elif container_exists; then
        log_warn "Lite node is stopped"
        docker ps -a --filter "name=${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}"
    else
        log_info "Lite node is not installed"
    fi
}

do_info() {
    if ! container_running; then
        log_error "Lite node is not running"
        return 1
    fi

    log_info "Fetching node info..."
    local response
    response=$(curl -sf --max-time 10 "http://localhost:${HTTP_PORT}/tick-info" 2>/dev/null || true)

    if [ -z "$response" ]; then
        log_error "Could not fetch tick-info from port ${HTTP_PORT}"
        return 1
    fi

    echo ""
    echo -e "${GREEN}=== Lite Node Info ===${NC}"
    echo ""

    local epoch tick alias operator version uptime
    epoch=$(echo "$response" | grep -oP '"epoch":\K[0-9]+' | head -1)
    tick=$(echo "$response" | grep -oP '"tick":\K[0-9]+')
    alias=$(echo "$response" | grep -oP '"alias":"[^"]*"' | head -1 | cut -d'"' -f4)
    operator=$(echo "$response" | grep -oP '"operator":"[^"]*"' | head -1 | cut -d'"' -f4)
    version=$(echo "$response" | grep -oP '"version":"[^"]*"' | cut -d'"' -f4)
    uptime=$(echo "$response" | grep -oP '"uptime":\K[0-9]+')

    [ -n "$alias" ] && echo -e "  Alias:     ${CYAN}${alias}${NC}"
    [ -n "$operator" ] && echo -e "  Operator:  ${CYAN}${operator}${NC}"
    [ -n "$epoch" ] && echo -e "  Epoch:     ${epoch}"
    [ -n "$tick" ] && echo -e "  Tick:      ${tick}"
    [ -n "$version" ] && echo -e "  Version:   ${version}"
    [ -n "$uptime" ] && echo -e "  Uptime:    ${uptime}s"
    echo ""

    log_info "Raw response:"
    echo "$response" | head -c 1000
    echo ""
}

do_logs() {
    if ! container_exists; then
        log_error "Container not found"
        return 1
    fi
    log_info "Showing logs (Ctrl+C to exit)..."
    docker logs -f "$CONTAINER_NAME"
}

do_stop() {
    if container_running; then
        docker stop "$CONTAINER_NAME"
        log_ok "Stopped"
    else
        log_info "Already stopped"
    fi
}

do_start() {
    if container_running; then
        log_info "Already running"
        return
    fi

    if [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        cd "${DATA_DIR}" && docker compose up -d
        log_ok "Started"
    elif container_exists; then
        docker start "$CONTAINER_NAME"
        log_ok "Started"
    else
        log_error "Container not found. Run: $0 install"
        return 1
    fi
}

do_restart() {
    if [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        cd "${DATA_DIR}" && docker compose up -d --force-recreate
        log_ok "Restarted"
    elif container_exists; then
        docker restart "$CONTAINER_NAME"
        log_ok "Restarted"
    else
        log_error "Container not found. Run: $0 install"
        return 1
    fi
}

interactive_install() {
    echo ""
    echo "=== Lite Node Installer ==="
    echo ""

    print_security_warning

    # Get seed
    while [ -z "$OPERATOR_SEED" ]; do
        read -rp "Operator seed: " OPERATOR_SEED
        [ -z "$OPERATOR_SEED" ] && echo "  Seed is required."
    done

    # Get alias
    while [ -z "$OPERATOR_ALIAS" ]; do
        read -rp "Operator alias: " OPERATOR_ALIAS
        [ -z "$OPERATOR_ALIAS" ] && echo "  Alias is required."
    done

    echo ""
    do_install
}

print_logo() {
    echo ""
    echo -e "${CYAN}              @@@@@@ @@@@@@${NC}"
    echo -e "${CYAN}              @@@@@@ @@@@@@${NC}"
    echo -e "${CYAN}              @@@@@@ @@@@@@${NC}"
    echo -e "${CYAN}              @@@@@@ @@@@@@${NC}"
    echo -e "${CYAN}              @@@@@@ @@@@@@${NC}"
    echo -e "${CYAN}              @@@@@@ @@@@@@${NC}"
    echo -e "${CYAN}              @@@@@@ @@@@@@${NC}"
    echo -e "${CYAN}                     @@@@@@${NC}"
    echo -e "${CYAN}                     @@@@@@${NC}"
    echo ""
    echo -e "${GREEN}      Qubic Lite Node Installer${NC}"
    echo -e "      ──────────────────────────"
    echo ""
}

interactive_menu() {
    set +e  # Disable exit on error for interactive mode
    while true; do
        echo ""
        print_logo

        echo -e "${CYAN}┌─────────────────────────────────────────────────┐${NC}"
        echo -e "${CYAN}│${NC}  ${GREEN}INSTALL${NC}                                        ${CYAN}│${NC}"
        echo -e "${CYAN}│${NC}    1) install      setup lite node              ${CYAN}│${NC}"
        echo -e "${CYAN}│${NC}    2) uninstall    remove lite node             ${CYAN}│${NC}"
        echo -e "${CYAN}├─────────────────────────────────────────────────┤${NC}"
        echo -e "${CYAN}│${NC}  ${GREEN}MANAGE${NC}                                         ${CYAN}│${NC}"
        echo -e "${CYAN}│${NC}    3) status    4) info      5) logs            ${CYAN}│${NC}"
        echo -e "${CYAN}│${NC}    6) stop      7) start     8) restart         ${CYAN}│${NC}"
        echo -e "${CYAN}├─────────────────────────────────────────────────┤${NC}"
        echo -e "${CYAN}│${NC}    0) exit                                      ${CYAN}│${NC}"
        echo -e "${CYAN}└─────────────────────────────────────────────────┘${NC}"
        echo ""
        read -rp "  Select [0-8]: " choice

        case "$choice" in
            0) echo ""; log_info "Goodbye!"; exit 0 ;;
            1) interactive_install || true ;;
            2) do_uninstall || true ;;
            3) do_status || true ;;
            4) do_info || true ;;
            5) do_logs || true ;;
            6) do_stop || true ;;
            7) do_start || true ;;
            8) do_restart || true ;;
            *) log_error "Invalid choice" ;;
        esac

        echo ""
        read -rp "  Press Enter to continue..." _
    done
}

# --- Main ---

OPERATOR_SEED=""
OPERATOR_ALIAS=""

# Parse arguments
if [ $# -eq 0 ]; then
    interactive_menu
    exit 0
fi

COMMAND="$1"
shift

while [ $# -gt 0 ]; do
    case "$1" in
        --seed)        OPERATOR_SEED="$2"; shift 2 ;;
        --alias)       OPERATOR_ALIAS="$2"; shift 2 ;;
        --p2p-port)    P2P_PORT="$2"; shift 2 ;;
        --http-port)   HTTP_PORT="$2"; shift 2 ;;
        --data-dir)    DATA_DIR="$2"; shift 2 ;;
        --help|-h)     print_usage; exit 0 ;;
        *)             log_error "Unknown option: $1"; print_usage; exit 1 ;;
    esac
done

case "$COMMAND" in
    install)    do_install ;;
    uninstall)  do_uninstall ;;
    status)     do_status ;;
    info)       do_info ;;
    logs)       do_logs ;;
    stop)       do_stop ;;
    start)      do_start ;;
    restart)    do_restart ;;
    help|--help|-h) print_usage ;;
    *)          log_error "Unknown command: $COMMAND"; print_usage; exit 1 ;;
esac
