#!/bin/bash
#
# Lite Node Installer - Docker-based setup
# https://github.com/qubic/core-lite
#
# Usage:
#   Interactive:  ./lite.sh
#   CLI:          ./lite.sh install --seed <seed> --alias <alias>
#
# Commands:
#   install       Install and start Lite node
#   uninstall     Remove Lite node
#   status        Show container status
#   logs          Show live logs
#   stop          Stop container
#   start         Start container
#   restart       Restart container
#   reconfigure   Change seed/alias and restart
#   reset         Wipe node data and restart fresh
#   update        Update this script to latest version
#

set -e

# Resolve script path before any cd
SCRIPT_PATH=$(realpath "$0" 2>/dev/null || readlink -f "$0" 2>/dev/null || echo "$0")

# --- Config ---
CONTAINER_NAME="qubic-lite"
DOCKER_IMAGE="qubiccore/lite"
DATA_DIR="/opt/qubic-lite"

# Default ports
P2P_PORT=21841
HTTP_PORT=41841

# Public RPC
NETWORK_RPC="https://rpc.qubic.org/v1/tick-info"

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
    echo "  reconfigure   Change seed/alias and restart"
    echo "  reset         Wipe node data and restart fresh"
    echo "  update        Update this script to latest version"
    echo "  watch         Live snapshot download progress"
    echo ""
    echo "Install/Reconfigure options:"
    echo "  --seed <seed>         Operator seed [REQUIRED]"
    echo "  --alias <alias>       Operator alias [REQUIRED]"
    echo "  --p2p-port <port>     P2P port (default: 21841)"
    echo "  --http-port <port>    HTTP port (default: 41841)"
    echo "  --data-dir <path>     Data directory (default: /opt/qubic-lite)"
    echo "  --image <image>       Override Docker image (default: qubiccore/lite:latest)"
    echo ""
    echo "Examples:"
    echo "  $0 install --seed myseed --alias mynode"
    echo "  $0 logs"
    echo "  $0 status"
}

print_security_warning() {
    echo ""
    log_warn "SECURITY TIP: To prevent your seed from being saved in shell history:"
    echo "      - Add a SPACE before the command:  ' ./lite.sh install ...'"
    echo "      - Or use interactive mode:  ./lite.sh"
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

get_network_tick() {
    local resp
    resp=$(curl -sf --max-time 5 "$NETWORK_RPC" 2>/dev/null || true)
    [ -n "$resp" ] && echo "$resp" | grep -oP '"tick":\K[0-9]+' | head -1
}

get_local_tick() {
    local resp
    resp=$(curl -sf --max-time 5 "http://localhost:${HTTP_PORT}/tick-info" 2>/dev/null || true)
    [ -n "$resp" ] && echo "$resp" | grep -oP '"tick":\K[0-9]+'
}

format_number() {
    printf "%'d" "$1" 2>/dev/null || echo "$1"
}

format_eta() {
    local seconds=$1
    if [ "$seconds" -lt 60 ]; then
        echo "< 1 min"
    elif [ "$seconds" -lt 3600 ]; then
        echo "~$((seconds / 60)) min"
    elif [ "$seconds" -lt 86400 ]; then
        local h=$((seconds / 3600)) m=$(( (seconds % 3600) / 60 ))
        echo "~${h}h ${m}m"
    else
        local d=$((seconds / 86400)) h=$(( (seconds % 86400) / 3600 ))
        echo "~${d}d ${h}h"
    fi
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
    docker rm -f watchtower-lite &>/dev/null || true

    # Create directory
    mkdir -p "${DATA_DIR}"

    # Copy script for management
    cp "$0" "${DATA_DIR}/lite.sh" 2>/dev/null || true
    chmod +x "${DATA_DIR}/lite.sh" 2>/dev/null || true

    # Pull image
    log_info "Pulling ${DOCKER_IMAGE}:latest..."
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
    container_name: watchtower-lite
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
    echo "  View logs:   ./lite.sh logs"
    echo "  Status:      ./lite.sh status"
    echo ""

    # Remove original script if not in DATA_DIR
    if [ "$SCRIPT_PATH" != "${DATA_DIR}/lite.sh" ] && [ -f "$SCRIPT_PATH" ]; then
        rm -f "$SCRIPT_PATH"
        log_ok "Removed installer from download location"
    fi

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
        docker rm -f watchtower-lite &>/dev/null || true
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

get_snapshot_total_size() {
    local snap_url
    snap_url=$(docker logs "$CONTAINER_NAME" 2>&1 | grep -oP 'Downloading \Khttps?://[^ "]+' | tail -1)
    if [ -n "$snap_url" ]; then
        curl -sI "$snap_url" 2>/dev/null | grep -i content-length | awk '{print $2}' | tr -d '\r'
    fi
}

check_snapshot_progress() {
    local volume_path snap_file snap_size total_size pct bar filled empty
    volume_path=$(docker volume inspect qubic-lite_qubic-lite-data --format '{{.Mountpoint}}' 2>/dev/null || true)
    [ -z "$volume_path" ] && return 1

    snap_file="${volume_path}/snapshot.tar.zst"
    [ ! -f "$snap_file" ] && return 1

    snap_size=$(stat -c%s "$snap_file" 2>/dev/null || echo 0)
    [ "$snap_size" -eq 0 ] && return 1

    total_size=$(get_snapshot_total_size)

    if [ -z "$total_size" ] || [ "$total_size" -eq 0 ] 2>/dev/null; then
        local snap_mb=$((snap_size / 1024 / 1024))
        echo -e "  ${YELLOW}Downloading snapshot... ${snap_mb} MB downloaded${NC}"
        return 0
    fi

    pct=$((snap_size * 100 / total_size))
    [ "$pct" -gt 100 ] && pct=100

    local snap_gb total_gb
    snap_gb=$(awk "BEGIN {printf \"%.1f\", $snap_size / 1024 / 1024 / 1024}")
    total_gb=$(awk "BEGIN {printf \"%.1f\", $total_size / 1024 / 1024 / 1024}")

    # Progress bar (30 chars wide)
    filled=$((pct * 30 / 100))
    empty=$((30 - filled))
    bar=$(printf '%0.s█' $(seq 1 $filled 2>/dev/null) || true)
    bar+=$(printf '%0.s░' $(seq 1 $empty 2>/dev/null) || true)

    echo -e "  ${YELLOW}Downloading snapshot...${NC}"
    echo -e "  ${CYAN}${bar}${NC} ${pct}%  (${snap_gb} / ${total_gb} GB)"
    return 0
}

watch_snapshot_progress() {
    local volume_path snap_file total_size
    volume_path=$(docker volume inspect qubic-lite_qubic-lite-data --format '{{.Mountpoint}}' 2>/dev/null || true)
    [ -z "$volume_path" ] && { log_error "Volume not found"; return 1; }

    snap_file="${volume_path}/snapshot.tar.zst"
    if [ ! -f "$snap_file" ]; then
        log_info "No snapshot download in progress"
        return 1
    fi

    log_info "Fetching snapshot size..."
    total_size=$(get_snapshot_total_size)

    echo ""
    echo -e "  ${YELLOW}Snapshot download (Ctrl+C to stop)${NC}"
    echo ""

    while [ -f "$snap_file" ]; do
        local snap_size pct snap_gb total_gb filled empty bar
        snap_size=$(stat -c%s "$snap_file" 2>/dev/null || echo 0)

        if [ -n "$total_size" ] && [ "$total_size" -gt 0 ] 2>/dev/null; then
            pct=$((snap_size * 100 / total_size))
            [ "$pct" -gt 100 ] && pct=100
            snap_gb=$(awk "BEGIN {printf \"%.1f\", $snap_size / 1024 / 1024 / 1024}")
            total_gb=$(awk "BEGIN {printf \"%.1f\", $total_size / 1024 / 1024 / 1024}")

            filled=$((pct * 30 / 100))
            empty=$((30 - filled))
            bar=$(printf '%0.s█' $(seq 1 $filled 2>/dev/null) || true)
            bar+=$(printf '%0.s░' $(seq 1 $empty 2>/dev/null) || true)

            printf "\r  ${CYAN}${bar}${NC} ${pct}%%  (${snap_gb} / ${total_gb} GB)  "
        else
            local snap_mb=$((snap_size / 1024 / 1024))
            printf "\r  ${YELLOW}Downloading... ${snap_mb} MB${NC}  "
        fi

        # Check if download finished (file removed after extraction)
        sleep 3
    done

    echo ""
    echo ""
    log_ok "Snapshot download complete!"
}

do_status() {
    if container_running; then
        trap 'echo ""; return 0' INT

        local tick_prev=""
        while true; do
            clear
            print_logo
            log_ok "Lite node is running"
            echo ""
            docker ps --filter "name=${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
            docker ps --filter "name=watchtower-lite" --format "table {{.Names}}\t{{.Status}}" 2>/dev/null || true

            local tick_now
            tick_now=$(get_local_tick)

            if [ -z "$tick_now" ]; then
                echo ""
                if ! check_snapshot_progress; then
                    log_warn "HTTP not responding on port ${HTTP_PORT}"
                fi
                echo ""
                echo -e "  ${BLUE}Refreshing every 3s... (Ctrl+C to exit)${NC}"
                tick_prev=""
                sleep 3
                continue
            fi

            local net_tick
            net_tick=$(get_network_tick)

            echo ""
            echo -e "  ${GREEN}=== Node Health ===${NC}"
            echo ""

            # Determine if ticking (compare with previous reading)
            local ticking=false
            if [ -n "$tick_prev" ] && [ "$tick_now" -gt "$tick_prev" ] 2>/dev/null; then
                ticking=true
            fi

            if [ -n "$net_tick" ] && [ "$tick_now" -ge "$net_tick" ] 2>/dev/null; then
                echo -e "  Status:    ${GREEN}● SYNCED${NC}"
            elif [ -z "$tick_prev" ]; then
                echo -e "  Status:    ${YELLOW}● CHECKING...${NC}"
            elif [ "$ticking" = true ]; then
                echo -e "  Status:    ${YELLOW}● SYNCING${NC} (ticking)"
            else
                echo -e "  Status:    ${RED}● NOT TICKING${NC}"
            fi

            echo -e "  Node Tick: ${CYAN}$(format_number "$tick_now")${NC}"

            if [ -n "$net_tick" ]; then
                echo -e "  Net Tick:  $(format_number "$net_tick")"

                local behind=$((net_tick - tick_now))
                if [ "$behind" -gt 0 ]; then
                    local pct
                    pct=$(awk "BEGIN {printf \"%.1f\", $tick_now * 100 / $net_tick}")
                    echo -e "  Behind:    $(format_number "$behind") ticks (${pct}% synced)"

                    # ETA based on measured tick rate
                    if [ "$ticking" = true ] && [ -n "$tick_prev" ]; then
                        local rate=$((tick_now - tick_prev))
                        if [ "$rate" -gt 0 ]; then
                            local eta_sec=$((behind * 3 / rate))
                            echo -e "  ETA:       ${CYAN}$(format_eta "$eta_sec")${NC}"
                        fi
                    fi
                fi
            else
                log_warn "Could not reach network RPC"
            fi

            echo ""
            echo -e "  ${BLUE}Refreshing every 3s... (Ctrl+C to exit)${NC}"

            tick_prev="$tick_now"
            sleep 3
        done
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
        echo ""
        if check_snapshot_progress; then
            echo ""
            log_info "Node will be available after snapshot download completes"
            return 0
        fi
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

do_reconfigure() {
    if [ ! -f "${DATA_DIR}/.env" ]; then
        log_error "No config found. Run install first."
        return 1
    fi

    # Show current config
    echo ""
    log_info "Current config:"
    local current_seed current_alias
    current_seed=$(grep -oP 'QUBIC_OPERATOR_SEED=\K.*' "${DATA_DIR}/.env" 2>/dev/null)
    current_alias=$(grep -oP 'QUBIC_OPERATOR_ALIAS=\K.*' "${DATA_DIR}/.env" 2>/dev/null)
    echo "  Seed:  ${current_seed:0:8}...${current_seed: -4}"
    echo "  Alias: ${current_alias}"
    echo ""

    # Get new values (Enter to keep current)
    local new_seed new_alias
    read -rp "New seed (Enter to keep current): " new_seed
    read -rp "New alias (Enter to keep current): " new_alias

    new_seed="${new_seed:-$current_seed}"
    new_alias="${new_alias:-$current_alias}"

    if [ "$new_seed" = "$current_seed" ] && [ "$new_alias" = "$current_alias" ]; then
        log_info "No changes made"
        return 0
    fi

    # Update .env
    cat > "${DATA_DIR}/.env" <<EOF
QUBIC_OPERATOR_SEED=${new_seed}
QUBIC_OPERATOR_ALIAS=${new_alias}
EOF
    chmod 600 "${DATA_DIR}/.env"
    log_ok "Config updated"

    # Restart with volume reset
    log_info "Restarting with fresh data..."
    cd "${DATA_DIR}" && docker compose down -v && docker compose up -d
    log_ok "Reconfigured and restarted!"
}

do_reset() {
    if ! container_exists; then
        log_error "Lite node is not installed"
        return 1
    fi

    echo ""
    log_warn "This will DELETE all node data and restart with a fresh state."
    log_warn "Config (seed/alias) will be kept."
    read -rp "Are you sure? [y/N] " confirm
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        log_info "Cancelled"
        return 0
    fi

    log_info "Wiping data and restarting..."
    cd "${DATA_DIR}" && docker compose down -v && docker compose up -d
    log_ok "Node reset complete! Starting with fresh state."
}

do_update() {
    local update_url tmp_file
    update_url="https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/lite.sh"
    tmp_file=$(mktemp)

    log_info "Checking for updates..."

    if ! curl -sfL --max-time 15 -o "$tmp_file" "$update_url"; then
        rm -f "$tmp_file"
        log_error "Failed to download update"
        return 1
    fi

    # Verify download is a valid script
    if ! head -1 "$tmp_file" | grep -q '^#!/bin/bash'; then
        rm -f "$tmp_file"
        log_error "Downloaded file is not a valid script"
        return 1
    fi

    # Check if there are changes
    if cmp -s "$SCRIPT_PATH" "$tmp_file"; then
        rm -f "$tmp_file"
        log_ok "Already up to date"
        return 0
    fi

    # Apply update
    chmod +x "$tmp_file"
    mv "$tmp_file" "$SCRIPT_PATH"
    log_ok "Updated successfully!"
    log_info "Restart the script to use the new version"
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
    echo -e "${CYAN}"
    cat << 'EOF'
            ██████  ██    ██ ██████  ██  ██████
            ██    ██ ██    ██ ██   ██ ██ ██
            ██    ██ ██    ██ ██████  ██ ██
            ██ ▄▄ ██ ██    ██ ██   ██ ██ ██
             ██████   ██████  ██████  ██  ██████
                ▀▀
EOF
    echo -e "${NC}"
    echo ""
    echo -e "                  ${GREEN}Qubic Lite Node Installer${NC}"
    echo -e "                  ${CYAN}─────────────────────────${NC}"
    echo ""
}

interactive_menu() {
    set +e  # Disable exit on error for interactive mode
    while true; do
        echo ""
        print_logo

        echo -e "         ${CYAN}┌────────────────────────────────────────┐${NC}"
        echo -e "         ${CYAN}│${NC} ${GREEN}INSTALL${NC}                                ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   1) install       setup lite node     ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   2) uninstall     remove lite node    ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}                                        ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC} ${GREEN}MANAGE${NC}                                 ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   3) status    4) info       5) logs   ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   6) stop      7) start      8) restart${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   9) reconfigure  change seed/alias    ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}  10) watch      live download progress ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}  11) reset      wipe data & restart    ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}                                        ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC} ${GREEN}OTHER${NC}                                  ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}  12) update     update client script   ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}                                        ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   0) exit                              ${CYAN}│${NC}"
        echo -e "         ${CYAN}└────────────────────────────────────────┘${NC}"
        echo ""
        read -rp "         Select [0-12]: " choice

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
            9) do_reconfigure || true ;;
            10) watch_snapshot_progress || true ;;
            11) do_reset || true ;;
            12) do_update || true ;;
            *) log_error "Invalid choice" ;;
        esac

        echo ""
        read -rp "         Press Enter to continue..." _
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
        --image)       DOCKER_IMAGE="$2"; shift 2 ;;
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
    restart)      do_restart ;;
    reconfigure)  do_reconfigure ;;
    reset)        do_reset ;;
    update)       do_update ;;
    watch)        watch_snapshot_progress ;;
    help|--help|-h) print_usage ;;
    *)          log_error "Unknown command: $COMMAND"; print_usage; exit 1 ;;
esac
