#!/bin/bash
#
# Bob Node Installer - Docker-based setup
# https://github.com/qubic/core-bob
#
# Usage:
#   Interactive:  ./bob.sh
#   CLI:          ./bob.sh install --seed <seed> --alias <alias>
#
# Commands:
#   install       Install and start Bob node
#   uninstall     Remove Bob node
#   status        Show container status
#   logs          Show live logs
#   stop          Stop container
#   start         Start container
#   restart       Restart container
#   reconfigure   Change seed/alias and restart
#   reset         Wipe node data and restart fresh
#   set-mem       Set KeyDB maxmemory (e.g. 12gb) — live + persisted
#   migrate       Migrate from named volumes to bind mounts
#   update        Update this script to latest version
#

set -e

# Resolve script path before any cd
SCRIPT_PATH=$(realpath "$0" 2>/dev/null || readlink -f "$0" 2>/dev/null || echo "$0")

# --- Config ---
CONTAINER_NAME="qubic-bob"
DOCKER_IMAGE="qubiccore/bob"
DATA_DIR="/opt/qubic-bob"

# Default ports
P2P_PORT=21842
API_PORT=40420

# Public RPC
NETWORK_RPC="https://rpc.qubic.org/v1/tick-info"

# Self-update / Guardian dashboard
BOB_SH_URL="https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/bob.sh"
GUARDIAN_PY_URL="https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/bob-guardian.py"

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
    echo "Bob Node Installer"
    echo ""
    echo "Usage:"
    echo "  Interactive:  $0"
    echo "  CLI:          $0 <command> [options]"
    echo ""
    echo "Default (no command): opens the Guardian dashboard once a node is installed."
    echo ""
    echo "Commands:"
    echo "  dashboard     Open the Guardian dashboard (bob-guardian.py)"
    echo "  old           Open the classic text menu (fallback)"
    echo "  install       Install and start Bob node"
    echo "  uninstall     Remove Bob node and data"
    echo "  status        Show container status"
    echo "  info          Show node info (tick, epoch, identity)"
    echo "  logs          Show live logs (Ctrl+C to exit)"
    echo "  stop          Stop container"
    echo "  start         Start container"
    echo "  restart       Restart container"
    echo "  reconfigure   Change seed/alias and restart"
    echo "  reset         Wipe node data and restart fresh"
    echo "  set-mem <sz>  Set KeyDB maxmemory (e.g. 12gb) — live + persisted"
    echo "  migrate       Migrate from named volumes to bind mounts"
    echo "  update        Update this script to latest version"
    echo ""
    echo "Install/Reconfigure options:"
    echo "  --seed <seed>       Node seed (55 lowercase letters) [REQUIRED]"
    echo "  --alias <alias>     Node alias name [REQUIRED]"
    echo "  --p2p-port <port>   P2P port (default: 21842)"
    echo "  --api-port <port>   API port (default: 40420)"
    echo "  --data-dir <path>   Data directory (default: /opt/qubic-bob)"
    echo ""
    echo "Examples:"
    echo "  $0 install --seed abcde...xyz --alias mynode"
    echo "  $0 logs"
    echo "  $0 status"
}

print_security_warning() {
    echo ""
    log_warn "SECURITY TIP: To prevent your seed from being saved in shell history:"
    echo "      - Add a SPACE before the command:  ' ./bob.sh install ...'"
    echo "      - Or use interactive mode:  ./bob.sh"
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

# Resolve the first reachable Bob API base into $BOB_API_BASE (cached).
# The published port can be unreachable via localhost on hardened nodes where the
# firewall blocks host->bridge forwarding (route_localnet off): the loopback DNAT
# stalls and only the host's primary IP — which hits docker-proxy on 0.0.0.0 —
# answers. Try 127.0.0.1 -> primary host IP -> localhost, keep the first that works.
# Call this from PARENT context (not inside $()), so the cache survives.
BOB_API_BASE=""
host_primary_ip() {
    ip route get 8.8.8.8 2>/dev/null | grep -oP 'src \K[0-9.]+' | head -1
}
resolve_bob_api() {
    # fast path: cached base still answering
    if [ -n "$BOB_API_BASE" ] && \
       curl -sf --max-time 3 -o /dev/null "${BOB_API_BASE}/status" 2>/dev/null; then
        return 0
    fi
    BOB_API_BASE=""
    local hip host
    hip=$(host_primary_ip)
    for host in 127.0.0.1 "$hip" localhost; do
        [ -z "$host" ] && continue
        if curl -sf --max-time 3 -o /dev/null "http://${host}:${API_PORT}/status" 2>/dev/null; then
            BOB_API_BASE="http://${host}:${API_PORT}"
            return 0
        fi
    done
    return 1
}

get_local_tick() {
    [ -z "$BOB_API_BASE" ] && resolve_bob_api
    [ -z "$BOB_API_BASE" ] && return
    local resp
    resp=$(curl -sf --max-time 5 "${BOB_API_BASE}/status" 2>/dev/null || true)
    [ -n "$resp" ] && echo "$resp" | grep -oP '"currentFetchingTick":\K[0-9]+'
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

# Check if the current compose file still uses named volumes
uses_named_volumes() {
    [ -f "${DATA_DIR}/docker-compose.yml" ] && grep -q "qubic-bob-data:/data" "${DATA_DIR}/docker-compose.yml"
}

# Wipe bind-mounted data directories and recreate them
wipe_data_dirs() {
    rm -rf "${DATA_DIR}/data/kvrocks" "${DATA_DIR}/data/redis" "${DATA_DIR}/data/bob"
    mkdir -p "${DATA_DIR}/data/kvrocks" "${DATA_DIR}/data/redis" "${DATA_DIR}/data/bob"
}

# Stop containers and wipe all node data, handling both old (named volumes) and new (bind mounts) setups
stop_and_wipe() {
    cd "${DATA_DIR}"
    if uses_named_volumes; then
        docker compose down -v
    else
        docker compose down
        wipe_data_dirs
    fi
}

# Confirm the node container actually reached (and held) a running state after a
# start/restart/reset. `docker compose up -d` returns 0 as soon as the container
# is created, but one that crashes on boot (port clash, bad config, data wiped
# mid-write) flips to Exited a moment later. The old code printed success
# regardless — and from the classic menu (which runs with `set +e`) that left an
# operator staring at "reset complete" over a dead node. Poll until it's up, then
# re-check after a beat so a boot-crash loop doesn't read as healthy. On failure
# surface the real state + last log lines and return non-zero.
verify_running() {
    local i
    for i in $(seq 1 8); do
        container_running && break
        sleep 1
    done
    if container_running; then
        sleep 2
        container_running && return 0
    fi
    log_error "Container '${CONTAINER_NAME}' is not running after start"
    local state
    state=$(docker ps -a --filter "name=^${CONTAINER_NAME}$" --format '{{.Status}}' 2>/dev/null)
    [ -n "$state" ] && log_warn "State: ${state}"
    log_warn "Recent logs:"
    docker logs --tail 20 "$CONTAINER_NAME" 2>&1 | sed 's/^/    /' || true
    log_warn "Retry with: cd ${DATA_DIR} && docker compose up -d"
    return 1
}

do_install() {
    log_info "Installing Bob node..."

    check_docker

    # Validate inputs
    if [ -z "$NODE_SEED" ]; then
        log_error "--seed is required"
        exit 1
    fi

    if [ ${#NODE_SEED} -ne 55 ]; then
        log_warn "Seed should be 55 characters (got ${#NODE_SEED})"
    fi

    if [ -z "$NODE_ALIAS" ]; then
        log_error "--alias is required"
        exit 1
    fi

    # Stop existing containers
    if container_exists; then
        log_info "Removing existing container..."
        docker rm -f "$CONTAINER_NAME" &>/dev/null || true
    fi
    docker rm -f watchtower-bob &>/dev/null || true

    # Create directories
    mkdir -p "${DATA_DIR}/data/kvrocks" "${DATA_DIR}/data/redis" "${DATA_DIR}/data/bob"

    # Copy script for management
    cp "$0" "${DATA_DIR}/bob.sh" 2>/dev/null || true
    chmod +x "${DATA_DIR}/bob.sh" 2>/dev/null || true

    # Pull image
    log_info "Pulling image from Docker Hub..."
    docker pull "${DOCKER_IMAGE}:latest"
    log_ok "Image ready"

    # Create .env file with sensitive data
    cat > "${DATA_DIR}/.env" <<EOF
NODE_SEED=${NODE_SEED}
NODE_ALIAS=${NODE_ALIAS}
EOF
    chmod 600 "${DATA_DIR}/.env"
    log_ok "Config: ${DATA_DIR}/.env"

    # Create docker-compose.yml
    cat > "${DATA_DIR}/docker-compose.yml" <<EOF
services:
  qubic-bob:
    image: ${DOCKER_IMAGE}:latest
    container_name: ${CONTAINER_NAME}
    restart: unless-stopped
    ports:
      - "${P2P_PORT}:21842"
      - "${API_PORT}:40420"
    env_file:
      - .env
    volumes:
      - ./data/kvrocks:/data/kvrocks
      - ./data/redis:/data/redis
      - ./data/bob:/data/bob

  watchtower:
    image: containrrr/watchtower
    container_name: watchtower-bob
    restart: unless-stopped
    environment:
      DOCKER_API_VERSION: "1.44"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    command: --interval 300 ${CONTAINER_NAME}
EOF

    # Start containers
    log_info "Starting containers..."
    cd "${DATA_DIR}" && docker compose up -d
    verify_running || return 1

    log_ok "Bob node started!"
    echo ""
    echo "  Container:   $CONTAINER_NAME"
    echo "  Config:      ${DATA_DIR}/.env"
    echo "  Data:        ${DATA_DIR}/data/"
    echo "  P2P:         port ${P2P_PORT}"
    echo "  API:         http://localhost:${API_PORT}"
    echo "  Auto-Update: enabled (Watchtower)"
    echo ""
    echo "  View logs:   ./bob.sh logs"
    echo "  Status:      ./bob.sh status"
    echo ""

    # Remove original script if not in DATA_DIR
    if [ "$SCRIPT_PATH" != "${DATA_DIR}/bob.sh" ] && [ -f "$SCRIPT_PATH" ]; then
        rm -f "$SCRIPT_PATH"
        log_ok "Removed installer from download location"
    fi

    cd "${DATA_DIR}"
}

do_uninstall() {
    log_info "Uninstalling Bob node..."

    # Stop containers
    if [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        docker compose -f "${DATA_DIR}/docker-compose.yml" down 2>/dev/null || true
        log_ok "Containers stopped"
    elif container_exists; then
        docker rm -f "$CONTAINER_NAME" &>/dev/null || true
        docker rm -f watchtower-bob &>/dev/null || true
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

    # Clean up any leftover named volumes from old installations
    docker volume rm "qubic-bob_qubic-bob-data" 2>/dev/null || true
    docker volume prune -f 2>/dev/null || true

    log_ok "Uninstall complete"

    # Return to home if data dir was removed
    if [ "$data_removed" = true ]; then
        cd ~ && exec bash
    fi
}

do_status() {
    if container_running; then
        trap 'echo ""; return 0' INT

        local tick_prev=""
        while true; do
            clear
            print_logo
            log_ok "Bob node is running"
            echo ""
            docker ps --filter "name=${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
            docker ps --filter "name=watchtower-bob" --format "table {{.Names}}\t{{.Status}}" 2>/dev/null || true

            # resolve in parent context so the cached base survives across polls
            resolve_bob_api || true

            local tick_now
            tick_now=$(get_local_tick)

            if [ -z "$tick_now" ]; then
                echo ""
                log_warn "API not responding on port ${API_PORT}"
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
        log_warn "Bob node is stopped"
        docker ps -a --filter "name=${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}"
    else
        log_info "Bob node is not installed"
    fi
}

do_info() {
    if ! container_running; then
        log_error "Bob node is not running"
        return 1
    fi

    log_info "Fetching node info..."
    resolve_bob_api || true
    local response=""
    [ -n "$BOB_API_BASE" ] && response=$(curl -sf --max-time 10 "${BOB_API_BASE}/status" 2>/dev/null || true)

    if [ -z "$response" ]; then
        log_error "Could not fetch status from port ${API_PORT}"
        return 1
    fi

    echo ""
    echo -e "${GREEN}=== Bob Node Info ===${NC}"
    echo ""

    local epoch tick alias operator version uptime
    epoch=$(echo "$response" | grep -oP '"currentProcessingEpoch":\K[0-9]+')
    tick=$(echo "$response" | grep -oP '"currentFetchingTick":\K[0-9]+')
    alias=$(echo "$response" | grep -oP '"alias":"[^"]*"' | cut -d'"' -f4)
    operator=$(echo "$response" | grep -oP '"operator":"[^"]*"' | cut -d'"' -f4)
    version=$(echo "$response" | grep -oP '"bobVersion":\s*"[^"]*"' | cut -d'"' -f4)
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
        verify_running || return 1
        log_ok "Started"
    elif container_exists; then
        docker start "$CONTAINER_NAME"
        verify_running || return 1
        log_ok "Started"
    else
        log_error "Container not found. Run: $0 install"
        return 1
    fi
}

do_restart() {
    if [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        cd "${DATA_DIR}" && docker compose up -d --force-recreate
        verify_running || return 1
        log_ok "Restarted"
    elif container_exists; then
        docker restart "$CONTAINER_NAME"
        verify_running || return 1
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
    current_seed=$(grep -oP 'NODE_SEED=\K.*' "${DATA_DIR}/.env" 2>/dev/null)
    current_alias=$(grep -oP 'NODE_ALIAS=\K.*' "${DATA_DIR}/.env" 2>/dev/null)
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
NODE_SEED=${new_seed}
NODE_ALIAS=${new_alias}
EOF
    chmod 600 "${DATA_DIR}/.env"
    log_ok "Config updated"

    # Restart with data wipe
    log_info "Restarting with fresh data..."
    stop_and_wipe
    cd "${DATA_DIR}" && docker compose up -d
    verify_running || return 1
    log_ok "Reconfigured and restarted!"
}

do_reset() {
    if ! container_exists; then
        log_error "Bob node is not installed"
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
    stop_and_wipe
    cd "${DATA_DIR}" && docker compose up -d
    verify_running || return 1
    log_ok "Node reset complete! Starting with fresh state."
}

do_migrate() {
    log_info "Checking if migration is needed..."

    if [ ! -f "${DATA_DIR}/docker-compose.yml" ]; then
        log_error "No docker-compose.yml found at ${DATA_DIR}."
        log_error "Nothing to migrate. Run install first."
        return 1
    fi

    if ! uses_named_volumes; then
        log_ok "Already using bind mounts. No migration needed."
        return 0
    fi

    if ! container_exists; then
        log_error "Container '$CONTAINER_NAME' not found."
        return 1
    fi

    echo ""
    log_info "This will migrate from Docker named volumes to local bind mounts."
    log_info "Data will be stored under ${DATA_DIR}/data/"
    echo ""
    read -rp "Continue? [y/N] " confirm
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        log_info "Cancelled"
        return 0
    fi

    # Step 1: Stop internal services
    if container_running; then
        echo ""
        log_info "Stopping internal services (redis/kvrocks)..."
        docker exec "$CONTAINER_NAME" /bin/sh -c '
            if command -v supervisorctl > /dev/null 2>&1; then
                supervisorctl stop all 2>/dev/null || true
                sleep 3
            else
                kill $(pgrep -f redis-server 2>/dev/null) 2>/dev/null || true
                kill $(pgrep -f kvrocks 2>/dev/null) 2>/dev/null || true
                sleep 3
            fi
        ' 2>/dev/null || log_warn "Could not stop internal services. Continuing anyway."
        log_ok "Internal services stopped"
    fi

    # Step 2: Create data directories
    log_info "Creating data directories..."
    mkdir -p "${DATA_DIR}/data/kvrocks" "${DATA_DIR}/data/redis" "${DATA_DIR}/data/bob"

    # Step 3: Copy data from container
    log_info "Copying data from container..."
    for subdir in kvrocks redis bob; do
        echo "  -> /data/${subdir}"
        docker cp "$CONTAINER_NAME":/data/${subdir}/. "${DATA_DIR}/data/${subdir}/" 2>/dev/null || \
            log_warn "Could not copy /data/${subdir} (may be empty)"
    done

    echo ""
    log_info "Data sizes:"
    du -sh "${DATA_DIR}/data"/* 2>/dev/null || echo "  (empty)"
    echo ""

    # Step 4: Backup compose file
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    cp "${DATA_DIR}/docker-compose.yml" "${DATA_DIR}/docker-compose.yml.backup.${timestamp}"
    log_ok "Compose backup: docker-compose.yml.backup.${timestamp}"

    # Step 5: Stop containers
    log_info "Stopping containers..."
    cd "${DATA_DIR}" && docker compose down

    # Step 6: Patch docker-compose.yml
    log_info "Patching docker-compose.yml..."

    # Get the indentation of the existing volume line
    local indent
    indent=$(grep "qubic-bob-data:/data" "${DATA_DIR}/docker-compose.yml" | sed 's/\(-.*\)//')

    # Replace the named volume mount with three bind mounts
    sed -i "s|^.*- qubic-bob-data:/data.*$|${indent}- ./data/kvrocks:/data/kvrocks\n${indent}- ./data/redis:/data/redis\n${indent}- ./data/bob:/data/bob|" "${DATA_DIR}/docker-compose.yml"

    # Remove the top-level volumes block and the qubic-bob-data entry
    sed -i '/^volumes:/,/^[^ ]/{/^volumes:/d; /^[[:space:]]*qubic-bob-data:/d; /^[[:space:]]*$/d}' "${DATA_DIR}/docker-compose.yml"

    # Clean up trailing blank lines
    sed -i -e :a -e '/^\n*$/{$d;N;ba}' "${DATA_DIR}/docker-compose.yml"

    echo ""
    log_info "Changes applied:"
    diff "${DATA_DIR}/docker-compose.yml.backup.${timestamp}" "${DATA_DIR}/docker-compose.yml" || true
    echo ""

    # Step 7: Start containers
    log_info "Starting containers..."
    docker compose up -d

    # Step 8: Clean up old volumes
    log_info "Cleaning up old Docker volumes..."
    docker volume rm "qubic-bob_qubic-bob-data" 2>/dev/null && \
        log_ok "Removed named volume qubic-bob_qubic-bob-data" || true
    docker volume prune -f 2>/dev/null || true

    # Verify
    sleep 3
    if container_running; then
        echo ""
        log_ok "Migration complete!"
        echo ""
        echo "  Data is now at: ${DATA_DIR}/data/"
        docker inspect "$CONTAINER_NAME" --format '{{range .Mounts}}  {{.Type}}  {{.Source}} -> {{.Destination}}{{"\n"}}{{end}}'
        echo ""
    else
        log_error "Container is not running after migration!"
        log_error "Check logs: docker compose logs"
        log_info "Your backup: ${DATA_DIR}/docker-compose.yml.backup.${timestamp}"
        log_info "Your data:   ${DATA_DIR}/data/"
    fi
}

do_update() {
    local tmp_file
    tmp_file=$(mktemp)

    log_info "Checking for updates..."

    if ! curl -sfL --max-time 15 -o "$tmp_file" "$BOB_SH_URL"; then
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

    # Apply bob.sh update (if changed)
    if cmp -s "$SCRIPT_PATH" "$tmp_file"; then
        rm -f "$tmp_file"
        log_ok "bob.sh already up to date"
    else
        chmod +x "$tmp_file"
        # Only claim success if the replace truly happened. The old code printed
        # "updated" unconditionally, so a failed mv (read-only fs, no perms,
        # wrong path) left users re-running the OLD file in a loop, fooled into
        # thinking it worked. Report the real result and how to recover.
        if mv -f "$tmp_file" "$SCRIPT_PATH" 2>/dev/null; then
            log_ok "bob.sh updated"
        else
            rm -f "$tmp_file"
            log_error "Update could NOT replace this script (permissions / read-only fs?)."
            log_warn  "Recover manually, then restart:"
            log_warn  "  sudo curl -fsSL '$BOB_SH_URL' -o '$SCRIPT_PATH' && sudo chmod +x '$SCRIPT_PATH'"
            return 1
        fi
    fi

    # Install/refresh the Guardian dashboard: fetch bob-guardian.py from git,
    # chmod +x, and set up the python venv + deps — everything it needs.
    install_guardian || true

    log_info "Restart the script to use the new version"
}

# --- Guardian dashboard (bob-guardian.py) ---

guardian_paths() {
    SCRIPT_HOME=$(dirname "$SCRIPT_PATH")
    GUARDIAN_PY="${SCRIPT_HOME}/bob-guardian.py"
    GUARDIAN_VENV="${SCRIPT_HOME}/.venv"
}

# Download bob-guardian.py from git + chmod +x. Returns 1 on failure.
download_guardian_py() {
    guardian_paths
    local tmp
    tmp=$(mktemp)
    if curl -sfL --max-time 20 -o "$tmp" "$GUARDIAN_PY_URL" && head -1 "$tmp" | grep -q 'python'; then
        if cmp -s "$GUARDIAN_PY" "$tmp" 2>/dev/null; then
            rm -f "$tmp"
            log_ok "Dashboard already up to date"
        else
            mv "$tmp" "$GUARDIAN_PY"
            chmod u+x "$GUARDIAN_PY" 2>/dev/null || true
            log_ok "Dashboard installed (bob-guardian.py)"
        fi
        return 0
    fi
    rm -f "$tmp"
    log_warn "Could not fetch dashboard (bob-guardian.py) from git"
    return 1
}

# Create/refresh the python venv with the dashboard deps. Returns 1 on failure.
ensure_venv() {
    guardian_paths
    if ! command -v python3 >/dev/null 2>&1; then
        log_warn "python3 not found (needed for the dashboard)"
        return 1
    fi
    if [ -x "${GUARDIAN_VENV}/bin/python" ] && "${GUARDIAN_VENV}/bin/python" -c 'import textual' 2>/dev/null; then
        return 0
    fi
    log_info "Setting up dashboard environment (~30s)..."
    if ! python3 -m venv "$GUARDIAN_VENV" 2>/dev/null; then
        if command -v apt-get >/dev/null 2>&1; then
            apt-get update -y >/dev/null 2>&1 || true
            apt-get install -y python3-venv >/dev/null 2>&1 || true
        fi
        python3 -m venv "$GUARDIAN_VENV" || { log_warn "venv creation failed"; return 1; }
    fi
    "${GUARDIAN_VENV}/bin/pip" install --quiet --upgrade pip >/dev/null 2>&1 || true
    if ! "${GUARDIAN_VENV}/bin/pip" install --quiet 'textual>=0.80,<1' rich; then
        log_warn "Failed to install dashboard dependencies"
        return 1
    fi
    return 0
}

# Full install used by 'update': dashboard + deps + chmod.
install_guardian() {
    download_guardian_py || return 1
    ensure_venv || return 1
    log_ok "Dashboard ready — next './bob.sh' opens it"
}

launch_guardian() {
    guardian_paths
    # Not installed yet -> classic menu. 'update' installs it.
    if [ ! -f "$GUARDIAN_PY" ]; then
        log_info "New dashboard not installed yet — opening the classic menu."
        log_info "Run 'update' to install the dashboard, then restart."
        interactive_menu
        return
    fi
    if ! ensure_venv; then
        log_warn "Dashboard dependencies missing — opening the classic menu."
        log_info "Run 'update' to (re)install, then restart."
        interactive_menu
        return
    fi
    exec "${GUARDIAN_VENV}/bin/python" "$GUARDIAN_PY" \
        --bob-script "$SCRIPT_PATH" --api-port "$API_PORT"
}

interactive_install() {
    echo ""
    echo "=== Bob Node Installer ==="
    echo ""

    print_security_warning

    # Get seed
    while [ -z "$NODE_SEED" ]; do
        read -rp "Node seed (55 characters): " NODE_SEED
        if [ -z "$NODE_SEED" ]; then
            echo "  Seed is required."
        elif [ ${#NODE_SEED} -ne 55 ]; then
            log_warn "Seed should be 55 characters (got ${#NODE_SEED})"
            read -rp "  Continue anyway? [y/N] " confirm
            [[ ! "$confirm" =~ ^[Yy]$ ]] && NODE_SEED=""
        fi
    done

    # Get alias
    while [ -z "$NODE_ALIAS" ]; do
        read -rp "Node alias: " NODE_ALIAS
        [ -z "$NODE_ALIAS" ] && echo "  Alias is required."
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
    echo -e "                  ${GREEN}Qubic Bob Node Installer${NC}"
    echo -e "                  ${CYAN}────────────────────────${NC}"
    echo ""
}

interactive_menu() {
    set +e  # Disable exit on error for interactive mode
    while true; do
        echo ""
        print_logo

        local needs_migrate=false
        uses_named_volumes && needs_migrate=true

        echo -e "         ${CYAN}┌────────────────────────────────────────┐${NC}"
        echo -e "         ${CYAN}│${NC} ${GREEN}INSTALL${NC}                                ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   1) install       setup bob node      ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   2) uninstall     remove bob node     ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}                                        ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC} ${GREEN}MANAGE${NC}                                 ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   3) status    4) info      5) logs    ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   6) stop      7) start     8) restart ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   9) reconfigure  change seed/alias    ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}                                        ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}  10) reset      wipe data & restart    ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}                                        ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC} ${GREEN}OTHER${NC}                                  ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}  11) update     update client script   ${CYAN}│${NC}"
        if [ "$needs_migrate" = true ]; then
        echo -e "         ${CYAN}│${NC}  12) ${YELLOW}migrate${NC}    volumes → bind mounts  ${CYAN}│${NC}"
        fi
        echo -e "         ${CYAN}│${NC}                                        ${CYAN}│${NC}"
        echo -e "         ${CYAN}│${NC}   0) exit                              ${CYAN}│${NC}"
        echo -e "         ${CYAN}└────────────────────────────────────────┘${NC}"
        echo ""
        read -rp "         Select: " choice

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
            10) do_reset || true ;;
            11) do_update || true ;;
            12) if [ "$needs_migrate" = true ]; then do_migrate || true; else log_error "Invalid choice"; fi ;;
            *) log_error "Invalid choice" ;;
        esac

        echo ""
        read -rp "         Press Enter to continue..." _
    done
}

do_set_mem() {
    local val="$1"
    if [ -z "$val" ]; then
        log_error "Usage: $0 set-mem <size>   (e.g. 8gb, 12gb, 8192mb)"
        exit 1
    fi
    # redis memory value: digits + optional k/m/g (+ optional b), case-insensitive
    if [[ ! "$val" =~ ^[0-9]+([kKmMgG][bB]?)?$ ]]; then
        log_error "Invalid memory value: '$val' (use e.g. 8gb, 12gb, 8192mb)"
        exit 1
    fi
    local env_file="${DATA_DIR}/.env"
    if [ ! -f "$env_file" ]; then
        log_error "Node not installed (${env_file} missing)"
        exit 1
    fi
    # Persist so the limit survives container recreation / Watchtower updates: the
    # bob image entrypoint writes REDIS_MAXMEMORY into redis.conf on every start.
    if grep -q "^REDIS_MAXMEMORY=" "$env_file"; then
        sed -i "s/^REDIS_MAXMEMORY=.*/REDIS_MAXMEMORY=${val}/" "$env_file"
    else
        echo "REDIS_MAXMEMORY=${val}" >> "$env_file"
    fi
    log_ok "Persisted REDIS_MAXMEMORY=${val} in ${env_file}"
    # Apply immediately without a restart (KeyDB CONFIG SET takes effect live).
    if container_running; then
        if docker exec "$CONTAINER_NAME" redis-cli CONFIG SET maxmemory "$val" 2>/dev/null | grep -q OK; then
            log_ok "Applied live: KeyDB maxmemory = ${val} (no restart needed)"
        else
            log_warn "Live apply failed — takes effect on next 'restart'"
        fi
    else
        log_info "Container not running — applies on next start"
    fi
}

# --- Main ---

# When sourced (e.g. for the dashboard) only load the functions/config
# above; skip the CLI/menu below.
if [ "${BASH_SOURCE[0]}" != "${0}" ]; then
    return 0 2>/dev/null || true
fi

NODE_SEED=""
NODE_ALIAS=""

# Parse arguments
if [ $# -eq 0 ]; then
    # Installed node  -> Guardian dashboard
    # Fresh machine    -> classic install menu (familiar first-run flow)
    if container_exists || [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        launch_guardian
    else
        interactive_menu
    fi
    exit 0
fi

COMMAND="$1"
shift

# set-mem takes a positional value (new KeyDB maxmemory) before flag parsing
MEM_VALUE=""
if [ "$COMMAND" = "set-mem" ]; then
    MEM_VALUE="$1"
    [ $# -gt 0 ] && shift
fi

while [ $# -gt 0 ]; do
    case "$1" in
        --seed)      NODE_SEED="$2"; shift 2 ;;
        --alias)     NODE_ALIAS="$2"; shift 2 ;;
        --p2p-port)  P2P_PORT="$2"; shift 2 ;;
        --api-port)  API_PORT="$2"; shift 2 ;;
        --data-dir)  DATA_DIR="$2"; shift 2 ;;
        --help|-h)   print_usage; exit 0 ;;
        *)           log_error "Unknown option: $1"; print_usage; exit 1 ;;
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
    set-mem)      do_set_mem "$MEM_VALUE" ;;
    migrate)      do_migrate ;;
    update)       do_update ;;
    dashboard|guardian|ui) launch_guardian ;;
    old|-old|--old|menu)   interactive_menu ;;
    help|--help|-h) print_usage ;;
    *)          log_error "Unknown command: $COMMAND"; print_usage; exit 1 ;;
esac
