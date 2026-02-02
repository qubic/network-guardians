#!/bin/bash
# bob-install.sh - Qubic Bob Node installer & manager
#
# Usage:
#   Interactive:  ./bob-install.sh
#   CLI:          ./bob-install.sh <mode> [options]
#
# Install modes:
#   docker              run via docker (recommended)
#   uninstall           remove bob node completely
#
# Management modes:
#   status              show container status
#   logs                show live logs (Ctrl+C to exit)
#   stop                stop containers
#   start               start containers
#   restart             restart containers
#   update              pull latest image + restart
#
# Options:
#   --node-seed <seed>      node identity seed (required for install)
#   --node-alias <alias>    node alias name (required for install)
#   --peers <ip:port,...>   peers to sync from
#   --threads <n>           max threads (0=auto)
#   --rpc-port <port>       REST API port (default: 40420)
#   --server-port <port>    P2P port (default: 21842)
#   --data-dir <path>       install dir (default: /opt/qubic-bob)

set -e

# resolve own path before any cd changes the working directory
SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"

# defaults
MODE=""
PEERS=""
BM_PEERS=""
MAX_THREADS=0
RPC_PORT=40420
SERVER_PORT=21842
DATA_DIR="/opt/qubic-bob"
DOCKER_IMAGE="qubiccore/bob"
ARBITRATOR_ID="AFZPUAIYVPNUYGJRQVLUKOPPVLHAZQTGLYAAUUNBXFTVTAMSBKQBLEIEPCVJ"
NODE_SEED=""
NODE_ALIAS=""

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

log_info()  { echo -e "${CYAN}[*]${NC} $1"; }
log_ok()    { echo -e "${GREEN}[+]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[-]${NC} $1"; }

validate_node_params() {
    # Validate NODE_SEED and NODE_ALIAS don't contain characters that break JSON
    local param_name="$1"
    local param_value="$2"

    if [ -z "$param_value" ]; then
        return 0
    fi

    # Check for quotes and backslashes that would break JSON
    case "$param_value" in
        *'"'*|*\\*)
            log_error "${param_name} contains invalid characters (quotes or backslashes)"
            exit 1
            ;;
    esac
}

print_usage() {
    echo "Usage:"
    echo "  Interactive:  $0"
    echo "  CLI:          $0 <mode> --node-seed <seed> --node-alias <alias> [options]"
    echo ""
    echo "Modes (install):"
    echo "  docker              run via docker (recommended)"
    echo "  uninstall           remove bob node completely"
    echo ""
    echo "Modes (manage):"
    echo "  status              show container status"
    echo "  logs                show live logs (Ctrl+C to exit)"
    echo "  stop                stop containers"
    echo "  start               start containers"
    echo "  restart             restart containers"
    echo "  update              pull latest image + restart"
    echo ""
    echo "Options:"
    echo "  --node-seed <seed>     node identity seed (REQUIRED for install)"
    echo "  --node-alias <alias>   node alias name (REQUIRED for install)"
    echo "  --peers <ip:port,...>  peers to sync from"
    echo "  --threads <n>          max threads (0=auto)"
    echo "  --rpc-port <port>      REST API port (default: 40420)"
    echo "  --server-port <port>   P2P port (default: 21842)"
    echo "  --data-dir <path>      install dir (default: /opt/qubic-bob)"
    echo ""
    echo "Security note:"
    echo "  Prefix CLI commands with a space to prevent seed from being saved in bash history:"
    echo "    [space]$0 docker --node-seed <seed> --node-alias <alias>"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "run as root"
        exit 1
    fi
}

check_system() {
    log_info "checking system..."

    if [ ! -f /etc/os-release ]; then
        log_error "needs Ubuntu/Debian"
        exit 1
    fi

    local ram_kb ram_gb cores avail_gb
    ram_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    ram_gb=$((ram_kb / 1024 / 1024))
    cores=$(nproc)
    avail_gb=$(df -BG / | tail -1 | awk '{print $4}' | tr -d 'G')

    [ "$ram_gb" -lt 14 ] && log_warn "RAM: ${ram_gb}GB (need 16GB)" || log_ok "RAM: ${ram_gb}GB"
    [ "$cores" -lt 4 ] && log_warn "CPU: ${cores} cores (need 4)" || log_ok "CPU: ${cores} cores"
    grep -q avx2 /proc/cpuinfo && log_ok "AVX2: yes" || log_warn "AVX2: not detected"
    [ "$avail_gb" -lt 100 ] && log_warn "Disk: ${avail_gb}GB (need 100GB)" || log_ok "Disk: ${avail_gb}GB"
}

# --- peer discovery ---

PEER_LIST_URL="https://app.qubic.li/network/live"

fetch_default_peers() {
    # fetch peers from qubic.global API when none provided
    if [ -n "$PEERS" ]; then
        # user provided manual peers - parse them into BM category
        parse_manual_peers
        return
    fi
    log_info "fetching peers from qubic.global API..."
    local resp
    resp=$(curl -sSf --max-time 10 "https://api.qubic.global/random-peers?service=bobNode&litePeers=6" 2>/dev/null) || {
        log_error "could not reach qubic.global API"
        log_warn "please provide peers manually with --peers or select from:"
        log_warn "  ${PEER_LIST_URL}"
        log_warn ""
        log_warn "example: --peers 1.2.3.4,5.6.7.8"
        exit 1
    }

    # Extract litePeers (these become BM/trusted-node peers)
    # Extract bobPeers (these provide actual tick data)
    local api_lite_peers api_bob_peers

    if command -v jq &> /dev/null; then
        # Use jq for reliable JSON parsing
        api_lite_peers=$(echo "$resp" | jq -r '.litePeers[]? // empty' 2>/dev/null || true)
        api_bob_peers=$(echo "$resp" | jq -r '.bobPeers[]? // empty' 2>/dev/null || true)
    else
        # Fallback to grep (less reliable but avoids jq dependency)
        api_lite_peers=$(echo "$resp" | grep -oP '"litePeers"\s*:\s*\[([^\]]*)\]' | grep -oP '"[^"]+\.\d+"' | tr -d '"' || true)
        api_bob_peers=$(echo "$resp" | grep -oP '"bobPeers"\s*:\s*\[([^\]]*)\]' | grep -oP '"[^"]+\.\d+"' | tr -d '"' || true)
    fi

    # Build peer lists
    BM_PEERS=""
    for ip in $api_lite_peers; do BM_PEERS="${BM_PEERS:+$BM_PEERS,}BM:${ip}:21841:0-0-0-0"; done

    BOB_PEERS=""
    for ip in $api_bob_peers; do BOB_PEERS="${BOB_PEERS:+$BOB_PEERS,}bob:${ip}:21842"; done

    if [ -z "$BM_PEERS" ] && [ -z "$BOB_PEERS" ]; then
        log_error "API returned no peers"
        log_warn "please provide peers manually with --peers or select from:"
        log_warn "  ${PEER_LIST_URL}"
        exit 1
    fi

    [ -n "$BM_PEERS" ] && log_ok "peers (BM): ${BM_PEERS}"
    [ -n "$BOB_PEERS" ] && log_ok "peers (bob): ${BOB_PEERS}"
}

parse_manual_peers() {
    # Parse user-provided PEERS string into BM and bob peers
    # Supports formats:
    #   BM:ip:port:pass        -> BM peer as-is
    #   BM:ip:port             -> BM peer, add :0-0-0-0 suffix
    #   bob:ip:port            -> bob peer as-is
    #   bob:ip                 -> bob peer, add :21842 port
    #   ip:port                -> BM peer, add BM: prefix and :0-0-0-0 suffix
    #   ip                     -> BM peer, add BM: prefix, :21841 port, and :0-0-0-0 suffix
    BM_PEERS=""
    BOB_PEERS=""
    local IFS=','
    for peer in $PEERS; do
        peer=$(echo "$peer" | xargs)  # trim whitespace
        if [[ "$peer" == BM:* ]]; then
            # BM peer
            if [[ "$peer" =~ ^BM:[0-9.]+:[0-9]+:[0-9-]+$ ]]; then
                BM_PEERS="${BM_PEERS:+$BM_PEERS,}$peer"
            elif [[ "$peer" =~ ^BM:[0-9.]+:[0-9]+$ ]]; then
                BM_PEERS="${BM_PEERS:+$BM_PEERS,}${peer}:0-0-0-0"
            else
                log_warn "skipping invalid BM peer format: $peer"
            fi
        elif [[ "$peer" == bob:* ]]; then
            # bob peer
            if [[ "$peer" =~ ^bob:[0-9.]+:[0-9]+$ ]]; then
                BOB_PEERS="${BOB_PEERS:+$BOB_PEERS,}$peer"
            elif [[ "$peer" =~ ^bob:[0-9.]+$ ]]; then
                local ip="${peer#bob:}"
                BOB_PEERS="${BOB_PEERS:+$BOB_PEERS,}bob:${ip}:21842"
            else
                log_warn "skipping invalid bob peer format: $peer"
            fi
        elif [[ "$peer" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:[0-9]+$ ]]; then
            # ip:port format -> add BM: prefix and passcode
            BM_PEERS="${BM_PEERS:+$BM_PEERS,}BM:${peer}:0-0-0-0"
        elif [[ "$peer" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            # ip only -> add BM: prefix, default port, and passcode
            BM_PEERS="${BM_PEERS:+$BM_PEERS,}BM:${peer}:21841:0-0-0-0"
        else
            log_warn "skipping invalid peer format: $peer"
        fi
    done

    if [ -z "$BM_PEERS" ] && [ -z "$BOB_PEERS" ]; then
        log_error "no valid peers found in: $PEERS"
        log_warn "please provide valid peer IPs, e.g.: --peers 1.2.3.4,5.6.7.8"
        log_warn "find peers at: ${PEER_LIST_URL}"
        exit 1
    fi

    [ -n "$BM_PEERS" ] && log_ok "peers (BM): ${BM_PEERS}"
    [ -n "$BOB_PEERS" ] && log_ok "peers (bob): ${BOB_PEERS}"
}

# --- config generation ---

generate_config() {
    local keydb_host="$1" kvrocks_host="$2" config_path="$3"

    # Combine BM and bob peers for trusted-node
    local all_peers=""
    [ -n "$BM_PEERS" ] && all_peers="$BM_PEERS"
    [ -n "$BOB_PEERS" ] && all_peers="${all_peers:+$all_peers,}$BOB_PEERS"

    # Build JSON array for trusted-node
    local trusted_json="[]"
    if [ -n "$all_peers" ]; then
        trusted_json=$(echo "$all_peers" | tr ',' '\n' | awk '{printf "\"%s\",", $0}' | sed 's/,$//' | awk '{print "["$0"]"}')
    fi

    cat > "$config_path" <<CONFIGEOF
{
  "p2p-node": [],
  "trusted-node": ${trusted_json},
  "request-cycle-ms": 100,
  "request-logging-cycle-ms": 30,
  "future-offset": 3,
  "log-level": "info",
  "keydb-url": "tcp://${keydb_host}:6379",
  "run-server": true,
  "server-port": ${SERVER_PORT},
  "rpc-port": ${RPC_PORT},
  "arbitrator-identity": "${ARBITRATOR_ID}",
  "tick-storage-mode": "kvrocks",
  "kvrocks-url": "tcp://${kvrocks_host}:6666",
  "tx-storage-mode": "kvrocks",
  "tx_tick_to_live": 10000,
  "max-thread": ${MAX_THREADS},
  "spam-qu-threshold": 100,
  "node-seed": "${NODE_SEED}",
  "node-alias": "${NODE_ALIAS}"
}
CONFIGEOF

    log_ok "config -> ${config_path}"
}

# --- docker install ---

install_docker() {
    log_info "setting up bob node..."

    install_docker_engine
    mkdir -p "${DATA_DIR}/data" && cd "${DATA_DIR}"

    fetch_default_peers
    generate_config "127.0.0.1" "127.0.0.1" "${DATA_DIR}/bob.json"

    cat > "${DATA_DIR}/docker-compose.yml" <<COMPOSEEOF
services:
  qubic-bob:
    image: ${DOCKER_IMAGE}:latest
    restart: unless-stopped
    ports:
      - "21842:21842"
      - "40420:40420"
    volumes:
      - ./bob.json:/app/bob.json:ro
      - qubic-bob-redis:/data/redis
      - qubic-bob-kvrocks:/data/kvrocks
      - ./data:/data/bob

volumes:
  qubic-bob-redis:
  qubic-bob-kvrocks:
COMPOSEEOF

    sed -i "s/\"21842:21842\"/\"${SERVER_PORT}:21842\"/" "${DATA_DIR}/docker-compose.yml"
    sed -i "s/\"40420:40420\"/\"${RPC_PORT}:40420\"/" "${DATA_DIR}/docker-compose.yml"

    log_info "starting containers..."
    docker compose up -d

    log_ok "done!"
    print_status_docker
}

# --- component installers ---

install_docker_engine() {
    if command -v docker &> /dev/null; then
        log_ok "docker: $(docker --version)"
    else
        log_info "installing docker..."
        curl -fsSL https://get.docker.com | sh
        systemctl enable docker && systemctl start docker
        log_ok "docker installed"
    fi
}

# --- status output ---

print_status_docker() {
    echo ""
    echo -e "${GREEN}--- bob node ready ---${NC}"
    echo "  dir:     ${DATA_DIR}"
    echo "  config:  ${DATA_DIR}/bob.json"
    echo "  P2P:     ${SERVER_PORT}"
    echo "  API:     http://localhost:${RPC_PORT}"
    echo ""
    echo "  docker compose -f ${DATA_DIR}/docker-compose.yml ps       # status"
    echo "  docker compose -f ${DATA_DIR}/docker-compose.yml logs -f  # logs"
    echo "  docker compose -f ${DATA_DIR}/docker-compose.yml restart  # restart"
    echo ""
}

# --- uninstall ---

do_uninstall() {
    log_info "uninstalling bob node..."

    # stop and remove docker containers
    if [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        log_info "stopping docker containers..."
        docker compose -f "${DATA_DIR}/docker-compose.yml" down -v 2>/dev/null || true
        log_ok "containers stopped and volumes removed"
    fi

    # stop systemd service if exists
    if systemctl is-active --quiet qubic-bob 2>/dev/null; then
        log_info "stopping systemd service..."
        systemctl stop qubic-bob
        systemctl disable qubic-bob
        rm -f /etc/systemd/system/qubic-bob.service
        systemctl daemon-reload
        log_ok "service removed"
    fi

    # remove install directory
    if [ -d "${DATA_DIR}" ]; then
        log_info "removing ${DATA_DIR}..."
        rm -rf "${DATA_DIR}"
        log_ok "directory removed"
    fi

    echo ""
    log_ok "bob node uninstalled"
}

# --- management commands ---

check_installed() {
    if [ ! -f "${DATA_DIR}/docker-compose.yml" ]; then
        log_error "bob node not installed in ${DATA_DIR}"
        log_info "run '$0 docker' to install"
        exit 1
    fi
}

cmd_status() {
    check_installed
    echo ""
    log_info "container status:"
    docker compose -f "${DATA_DIR}/docker-compose.yml" ps
    echo ""
    log_info "checking API..."
    local api_response
    api_response=$(curl -sf --max-time 5 "http://localhost:${RPC_PORT}/status" 2>/dev/null) && {
        echo "$api_response" | head -c 500
        echo ""
    } || log_warn "API not responding on port ${RPC_PORT}"
}

cmd_logs() {
    check_installed
    log_info "showing live logs (Ctrl+C to exit)..."
    docker compose -f "${DATA_DIR}/docker-compose.yml" logs -f
}

cmd_stop() {
    check_installed
    log_info "stopping containers..."
    docker compose -f "${DATA_DIR}/docker-compose.yml" stop
    log_ok "containers stopped"
}

cmd_start() {
    check_installed
    log_info "starting containers..."
    docker compose -f "${DATA_DIR}/docker-compose.yml" start
    log_ok "containers started"
}

cmd_restart() {
    check_installed
    log_info "restarting containers..."
    docker compose -f "${DATA_DIR}/docker-compose.yml" restart
    log_ok "containers restarted"
}

cmd_update() {
    check_installed
    log_info "pulling latest images..."
    docker compose -f "${DATA_DIR}/docker-compose.yml" pull
    log_info "restarting containers..."
    docker compose -f "${DATA_DIR}/docker-compose.yml" up -d
    log_ok "update complete"
}

# --- interactive setup ---

interactive_setup() {
    echo -e "${CYAN}┌─────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}  ${GREEN}INSTALL${NC}                                        ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}    1) docker       install via docker           ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}    2) uninstall    remove bob node              ${CYAN}│${NC}"
    echo -e "${CYAN}├─────────────────────────────────────────────────┤${NC}"
    echo -e "${CYAN}│${NC}  ${GREEN}MANAGE${NC}                                         ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}    3) status    4) logs      5) stop            ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}    6) start     7) restart   8) update          ${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────┘${NC}"
    echo ""
    while true; do
        read -rp "  Select [1-8]: " choice
        case "$choice" in
            1) MODE="docker";    break ;;
            2) MODE="uninstall"; break ;;
            3) MODE="status";    break ;;
            4) MODE="logs";      break ;;
            5) MODE="stop";      break ;;
            6) MODE="start";     break ;;
            7) MODE="restart";   break ;;
            8) MODE="update";    break ;;
            *) echo -e "  ${RED}Invalid choice. Please enter 1-8.${NC}" ;;
        esac
    done

    # skip seed/alias prompts for uninstall and management commands
    if [ "$MODE" = "uninstall" ] || [ "$MODE" = "status" ] || [ "$MODE" = "logs" ] || \
       [ "$MODE" = "stop" ] || [ "$MODE" = "start" ] || [ "$MODE" = "restart" ] || [ "$MODE" = "update" ]; then
        return
    fi

    echo ""
    echo -e "${CYAN}┌─────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}  ${GREEN}NODE CONFIGURATION${NC}                             ${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────┘${NC}"
    echo ""
    echo -e "  ${YELLOW}Tip: Interactive mode is safer than CLI for entering seeds.${NC}"
    echo ""
    while [ -z "$NODE_SEED" ]; do
        read -rp "  Node seed: " NODE_SEED
        # Strip surrounding quotes and whitespace from pasted input
        NODE_SEED=$(echo "$NODE_SEED" | sed "s/^[[:space:]]*[\"']*//; s/[\"']*[[:space:]]*$//")
        [ -z "$NODE_SEED" ] && echo -e "  ${RED}Node seed is required.${NC}"
    done

    while [ -z "$NODE_ALIAS" ]; do
        read -rp "  Node alias: " NODE_ALIAS
        # Strip surrounding quotes and whitespace from pasted input
        NODE_ALIAS=$(echo "$NODE_ALIAS" | sed "s/^[[:space:]]*[\"']*//; s/[\"']*[[:space:]]*$//")
        [ -z "$NODE_ALIAS" ] && echo -e "  ${RED}Node alias is required.${NC}"
    done

    echo ""
    read -rp "  Peers (ip:port, comma-separated, Enter=auto): " PEERS

    echo ""
    read -rp "  Max threads (Enter=auto): " input_threads
    if [ -n "$input_threads" ]; then
        MAX_THREADS="$input_threads"
    fi
    echo ""
}

# --- arg parsing ---

parse_args() {
    if [ $# -eq 0 ]; then
        interactive_setup
        return
    fi

    MODE="$1"; shift

    while [ $# -gt 0 ]; do
        case "$1" in
            --peers)       PEERS="$2";       shift 2 ;;
            --threads)     MAX_THREADS="$2"; shift 2 ;;
            --rpc-port)    RPC_PORT="$2";    shift 2 ;;
            --server-port) SERVER_PORT="$2"; shift 2 ;;
            --data-dir)    DATA_DIR="$2";    shift 2 ;;
            --node-seed)   NODE_SEED="$2";   shift 2 ;;
            --node-alias)  NODE_ALIAS="$2";  shift 2 ;;
            --help|-h)     print_usage;      exit 0  ;;
            *) log_error "unknown option: $1"; print_usage; exit 1 ;;
        esac
    done
}

# --- main ---

print_banner() {
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
    echo -e "${GREEN}       Qubic Bob Node Installer${NC}"
    echo -e "       ─────────────────────────"
    echo ""
}

main() {
    print_banner
    parse_args "$@"
    check_root

    # handle uninstall separately
    if [ "$MODE" = "uninstall" ]; then
        do_uninstall
        exit 0
    fi

    # handle management commands (no seed/system check needed)
    case "$MODE" in
        status)  cmd_status;  exit 0 ;;
        logs)    cmd_logs;    exit 0 ;;
        stop)    cmd_stop;    exit 0 ;;
        start)   cmd_start;   exit 0 ;;
        restart) cmd_restart; exit 0 ;;
        update)  cmd_update;  exit 0 ;;
    esac

    if [ -z "$NODE_SEED" ]; then
        log_error "--node-seed is required. Bob cannot start without a node seed."
        print_usage
        exit 1
    fi
    validate_node_params "node-seed" "$NODE_SEED"

    if [ -z "$NODE_ALIAS" ]; then
        log_error "--node-alias is required. Bob cannot start without a node alias."
        print_usage
        exit 1
    fi
    validate_node_params "node-alias" "$NODE_ALIAS"

    check_system

    case "$MODE" in
        docker) install_docker ;;
        *) log_error "unknown mode: ${MODE}"; print_usage; exit 1 ;;
    esac

    # copy script to install directory for future management
    if [ -f "$SELF" ] && [ "$SELF" != "${DATA_DIR}/bob-install.sh" ]; then
        cp "$SELF" "${DATA_DIR}/bob-install.sh"
        chmod +x "${DATA_DIR}/bob-install.sh"
        log_ok "script copied to ${DATA_DIR}/bob-install.sh"
    fi
}

main "$@"
