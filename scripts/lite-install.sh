#!/bin/bash
# lite-install.sh - Qubic Lite Node installer & manager
#
# Usage:
#   Interactive:  ./lite-install.sh
#   CLI:          ./lite-install.sh <mode> [options]
#
# Install modes:
#   docker              build + run via docker (recommended)
#   uninstall           remove lite node completely
#
# Management modes:
#   status              show container status
#   logs                show live logs (Ctrl+C to exit)
#   stop                stop container
#   start               start container
#   restart             restart container
#   update              rebuild + restart
#
# Options:
#   --operator-seed <seed>   operator identity seed (required for install)
#   --operator-alias <alias> operator alias name (required for install)
#   --peers <ip1,ip2,...>    peer IPs
#   --testnet                testnet mode (default: mainnet)
#   --port <port>            P2P port (default: 21841)
#   --http-port <port>       HTTP/RPC port (default: 41841)
#   --data-dir <path>        install dir (default: /opt/qubic-lite)
#   --avx512                 enable AVX-512
#   --epoch <N>              build for specific epoch (auto-detected if omitted)
#   --no-epoch               skip automatic epoch data download (mainnet)
#   --threads <N>            build threads (default: auto/all cores)
#   --processors <N>         max runtime processors (default: 8)

set -e

# resolve own path before any cd changes the working directory
SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"

# defaults
MODE=""
PEERS=""
TESTNET=false
NODE_PORT=21841
HTTP_PORT=41841
DATA_DIR="/opt/qubic-lite"
REPO_URL="https://github.com/qubic/core-lite.git"
ENABLE_AVX512=false
OPERATOR_SEED=""
OPERATOR_ALIAS=""
SECURITY_TICK=32
TICKING_DELAY=1000
SKIP_EPOCH=false
TARGET_EPOCH=""
BUILD_THREADS=""
MAX_PROCESSORS=""

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

log_info()  { echo -e "${CYAN}[*]${NC} $1"; }
log_ok()    { echo -e "${GREEN}[+]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[-]${NC} $1"; }

validate_operator_params() {
    # Validate OPERATOR_SEED and OPERATOR_ALIAS don't contain characters that break commands
    local param_name="$1"
    local param_value="$2"

    if [ -z "$param_value" ]; then
        return 0
    fi

    # Check for quotes and backslashes
    case "$param_value" in
        *'"'*|*\\*)
            log_error "${param_name} contains invalid characters (quotes or backslashes)"
            exit 1
            ;;
    esac
}

# --- fetch peers from API ---

fetch_peers_from_api() {
    local api_url="https://api.qubic.global/random-peers?service=bobNode&litePeers=8"
    local manual_url="https://app.qubic.li/network/live"

    # all log output to stderr so stdout only contains the peer list
    log_info "fetching peers from API..." >&2

    local response
    response=$(curl -sf --connect-timeout 10 --max-time 30 "${api_url}" 2>/dev/null || true)

    if [ -z "$response" ]; then
        log_warn "API not reachable: ${api_url}" >&2
        log_warn "please find peers manually at: ${manual_url}" >&2
        log_warn "look for active nodes and copy their IPs" >&2
        return 1
    fi

    # extract litePeers array for lite nodes - format: ["ip1","ip2",...]
    local peers_json
    peers_json=$(echo "$response" | grep -oP '"litePeers"\s*:\s*\[[^\]]*\]' | grep -oP '\[[^\]]*\]' || true)

    if [ -z "$peers_json" ] || [ "$peers_json" = "[]" ]; then
        log_warn "no litePeers found in API response" >&2
        log_warn "please find peers manually at: ${manual_url}" >&2
        return 1
    fi

    # convert JSON array to comma-separated IPs: ["1.2.3.4","5.6.7.8"] -> 1.2.3.4,5.6.7.8
    local peers_csv
    peers_csv=$(echo "$peers_json" | tr -d '[]"' | tr ',' '\n' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | head -8 | tr '\n' ',' | sed 's/,$//')

    if [ -z "$peers_csv" ]; then
        log_warn "could not parse peers from API" >&2
        log_warn "please find peers manually at: ${manual_url}" >&2
        return 1
    fi

    local peer_count
    peer_count=$(echo "$peers_csv" | tr ',' '\n' | wc -l)
    log_ok "found ${peer_count} peers from API" >&2

    # only output the peer list to stdout
    echo "$peers_csv"
    return 0
}

print_usage() {
    echo "Usage:"
    echo "  Interactive:  $0"
    echo "  CLI:          $0 <mode> --operator-seed <seed> --operator-alias <alias> [options]"
    echo ""
    echo "Modes (install):"
    echo "  docker              build + run via docker (recommended)"
    echo "  uninstall           remove lite node completely"
    echo ""
    echo "Modes (manage):"
    echo "  status              show container status"
    echo "  logs                show live logs (Ctrl+C to exit)"
    echo "  stop                stop container"
    echo "  start               start container"
    echo "  restart             restart container"
    echo "  update              rebuild + restart"
    echo ""
    echo "Options:"
    echo "  --operator-seed <seed>   operator identity seed (REQUIRED for install)"
    echo "  --operator-alias <alias> operator alias name (REQUIRED for install)"
    echo "  --peers <ip1,ip2,...>    peer IPs"
    echo "  --testnet                testnet mode (default: mainnet)"
    echo "  --port <port>            P2P port (default: 21841)"
    echo "  --http-port <port>       HTTP/RPC port (default: 41841)"
    echo "  --data-dir <path>        install dir (default: /opt/qubic-lite)"
    echo "  --avx512                 enable AVX-512"
    echo "  --epoch <N>              build for specific epoch (auto-detected if omitted)"
    echo "  --no-epoch               skip automatic epoch data download (mainnet)"
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

    local ram_kb ram_gb cores avail_gb min_ram
    ram_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    ram_gb=$((ram_kb / 1024 / 1024))
    cores=$(nproc)
    avail_gb=$(df -BG / | tail -1 | awk '{print $4}' | tr -d 'G')

    [ "$TESTNET" = true ] && min_ram=14 || min_ram=60

    [ "$ram_gb" -lt "$min_ram" ] && log_warn "RAM: ${ram_gb}GB (need $((min_ram + 2))GB)" || log_ok "RAM: ${ram_gb}GB"
    log_ok "CPU: ${cores} cores"
    grep -q avx2 /proc/cpuinfo && log_ok "AVX2: yes" || log_warn "AVX2: not detected (required for mainnet)"

    if [ "$ENABLE_AVX512" = true ]; then
        grep -q avx512 /proc/cpuinfo && log_ok "AVX-512: yes" || log_warn "AVX-512: not detected but requested"
    fi

    if [ "$TESTNET" = false ] && [ "$avail_gb" -lt 500 ]; then
        log_warn "Disk: ${avail_gb}GB (need 500GB for mainnet)"
    else
        log_ok "Disk: ${avail_gb}GB"
    fi
}

# --- build node args ---

build_node_args() {
    local args=""
    args="--operator-seed ${OPERATOR_SEED} --operator-alias ${OPERATOR_ALIAS}"
    [ "$TESTNET" = true ] && args="${args} --security-tick ${SECURITY_TICK} --ticking-delay ${TICKING_DELAY}"
    [ -n "$PEERS" ] && args="${args} --peers ${PEERS}"
    echo "$args"
}

# build docker-compose command as YAML list (safer than string)
build_docker_command() {
    local cmd="command: [\"--operator-seed\", \"${OPERATOR_SEED}\", \"--operator-alias\", \"${OPERATOR_ALIAS}\""
    if [ "$TESTNET" = true ]; then
        cmd="${cmd}, \"--security-tick\", \"${SECURITY_TICK}\", \"--ticking-delay\", \"${TICKING_DELAY}\""
    fi
    if [ -n "$PEERS" ]; then
        cmd="${cmd}, \"--peers\", \"${PEERS}\""
    fi
    cmd="${cmd}]"
    echo "$cmd"
}

# --- docker install ---

install_docker() {
    log_info "setting up lite node (docker)..."

    install_docker_engine
    mkdir -p "${DATA_DIR}" && cd "${DATA_DIR}"

    # clone source locally so we can checkout the right epoch
    log_info "cloning qubic-core-lite..."
    if [ -d "${DATA_DIR}/qubic-core-lite" ]; then
        log_info "source exists, updating..."
        cd "${DATA_DIR}/qubic-core-lite"
        git checkout main --quiet 2>/dev/null || true
        git pull
    else
        git clone "${REPO_URL}" "${DATA_DIR}/qubic-core-lite"
    fi
    cd "${DATA_DIR}"

    # determine target epoch: --epoch flag > auto-detect from storage > HEAD
    local epoch="${TARGET_EPOCH}"
    if [ -z "$epoch" ] && [ "$TESTNET" = false ] && [ "$SKIP_EPOCH" = false ]; then
        log_info "no --epoch given, detecting from storage.qubic.li..."
        local storage_url="https://storage.qubic.li/network"
        epoch=$(curl -sf "${storage_url}/" | grep -o 'ep[0-9]*-full\.zip' | grep -o '[0-9]*' | sort -n | tail -1 || true)
        [ -n "$epoch" ] && log_info "detected epoch ${epoch} from storage" || log_warn "auto-detect failed, using HEAD"
    fi

    # checkout matching source version for the target epoch
    checkout_epoch "${DATA_DIR}/qubic-core-lite" "$epoch"

    # patch max processors if specified
    patch_max_processors "${DATA_DIR}/qubic-core-lite" "$MAX_PROCESSORS"

    cd "${DATA_DIR}"

    # download epoch data
    mkdir -p "${DATA_DIR}/data"
    download_epoch_data "${DATA_DIR}/data" "$epoch"

    local avx_flag="OFF"
    local avx_docker_cmake_lines=""
    if [ "$ENABLE_AVX512" = true ]; then
        avx_flag="ON"
        # Inject -mavx512vbmi2 via CMAKE_PROJECT_INCLUDE (see manual build comment)
        avx_docker_cmake_lines='RUN echo "add_compile_options(-mavx512vbmi2)" > /app/avx512vbmi2.cmake'
    fi

    # exclude large dirs from Docker build context
    printf "data/\nqubic-core-lite/.git/\n" > "${DATA_DIR}/.dockerignore"

    local avx_include_flag=""
    [ "$ENABLE_AVX512" = true ] && avx_include_flag="-DCMAKE_PROJECT_INCLUDE=/app/avx512vbmi2.cmake"

    # use BUILD_THREADS if set, else nproc
    local build_jobs="${BUILD_THREADS:-\$(nproc)}"

    log_info "creating Dockerfile..."
    cat > "${DATA_DIR}/Dockerfile" <<DOCKEREOF
FROM ubuntu:24.04 AS builder
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y \\
    build-essential clang cmake nasm git g++ \\
    libc++-dev libc++abi-dev libjsoncpp-dev uuid-dev zlib1g-dev \\
    libstdc++-12-dev libfmt-dev \\
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY qubic-core-lite/ .
${avx_docker_cmake_lines}
WORKDIR /app/build
RUN cmake .. \\
    -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ \\
    -DBUILD_TESTS=OFF -DBUILD_BINARY=ON \\
    -DCMAKE_BUILD_TYPE=Release -DENABLE_AVX512=${avx_flag} \\
    ${avx_include_flag} \\
    && cmake --build . -- -j${build_jobs}

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y \\
    libc++1 libc++abi1 libjsoncpp25 libfmt9 \\
    && rm -rf /var/lib/apt/lists/*
WORKDIR /qubic
COPY --from=builder /app/build/src/Qubic .
EXPOSE ${NODE_PORT} ${HTTP_PORT}
ENTRYPOINT ["/qubic/Qubic"]
DOCKEREOF

    log_info "building docker image (this takes a while)..."
    docker build -t qubic-lite-node "${DATA_DIR}"

    local docker_cmd
    docker_cmd=$(build_docker_command)

    cat > "${DATA_DIR}/docker-compose.yml" <<COMPOSEEOF
services:
  qubic-lite:
    image: qubic-lite-node
    container_name: qubic-lite
    restart: unless-stopped
    working_dir: /qubic/data
    ports:
      - "${NODE_PORT}:${NODE_PORT}"
      - "${HTTP_PORT}:${HTTP_PORT}"
    volumes:
      - ${DATA_DIR}/data:/qubic/data
    ${docker_cmd}
COMPOSEEOF

    log_info "starting container..."
    docker compose up -d

    log_ok "done!"
    print_status_docker
}

# --- component installers ---

install_docker_engine() {
    if command -v docker &> /dev/null; then
        log_ok "docker: $(docker --version)"
        return
    fi
    log_info "installing docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker && systemctl start docker
    log_ok "docker installed"
}

# --- epoch data download (mainnet) ---

download_epoch_data() {
    local target_dir="$1"
    local epoch="$2"  # optional: skip auto-detect if provided

    if [ "$TESTNET" = true ]; then
        return
    fi

    if [ "$SKIP_EPOCH" = true ]; then
        log_warn "epoch download skipped (--no-epoch)"
        log_warn "download manually from: https://storage.qubic.li/network/"
        return
    fi

    # ensure unzip is available
    if ! command -v unzip &> /dev/null; then
        log_info "installing unzip..."
        apt-get update -qq
        NEEDRESTART_MODE=a apt-get install -y -qq unzip
    fi

    local storage_url="https://storage.qubic.li/network"

    if [ -z "$epoch" ]; then
        log_info "detecting latest epoch from storage.qubic.li..."
        epoch=$(curl -sf "${storage_url}/" | grep -o 'ep[0-9]*-full\.zip' | grep -o '[0-9]*' | sort -n | tail -1)
    fi

    if [ -z "$epoch" ]; then
        log_warn "could not detect latest epoch"
        log_warn "download manually from: ${storage_url}/"
        return
    fi

    local zip_file="ep${epoch}-full.zip"
    local zip_url="${storage_url}/${epoch}/${zip_file}"

    log_info "downloading epoch ${epoch} data (${zip_file}) ..."

    mkdir -p "${target_dir}"

    if ! wget --tries=3 --timeout=120 --waitretry=5 --show-progress -O "${target_dir}/${zip_file}" "${zip_url}"; then
        log_warn "download failed: ${zip_url}"
        log_warn "download manually from: ${storage_url}/"
        rm -f "${target_dir}/${zip_file}"
        return
    fi

    log_info "extracting epoch data..."
    if ! unzip -o -q "${target_dir}/${zip_file}" -d "${target_dir}"; then
        log_warn "extraction failed"
        return
    fi

    rm -f "${target_dir}/${zip_file}"
    DETECTED_EPOCH="${epoch}"
    log_ok "epoch ${epoch} data ready in ${target_dir}"
}

# --- epoch / source checkout ---

checkout_epoch() {
    local src_dir="$1"
    local epoch="$2"
    local settings_file="${src_dir}/src/public_settings.h"

    if [ -z "$epoch" ]; then
        log_info "no target epoch set, building from latest source (HEAD)"
        return
    fi

    # check if HEAD already matches
    local head_epoch
    head_epoch=$(grep -oP '#define\s+EPOCH\s+\K[0-9]+' "$settings_file" 2>/dev/null || true)

    if [ "$head_epoch" = "$epoch" ]; then
        log_ok "source already at epoch ${epoch} (HEAD)"
        return
    fi

    log_info "source is epoch ${head_epoch}, need epoch ${epoch} -- searching git history..."

    # find commit where public_settings.h had the target epoch
    local target_commit=""
    local commits
    commits=$(cd "$src_dir" && git log --all --format="%H" -100 -- src/public_settings.h 2>/dev/null || true)
    for c in $commits; do
        local ep
        ep=$(cd "$src_dir" && git show "${c}:src/public_settings.h" 2>/dev/null \
            | grep -oP '#define\s+EPOCH\s+\K[0-9]+' || true)
        if [ "$ep" = "$epoch" ]; then
            target_commit="$c"
            break
        fi
    done

    if [ -z "$target_commit" ]; then
        log_error "could not find epoch ${epoch} in git history!"
        log_error "available epochs in recent history:"
        for c in $commits; do
            local ep
            ep=$(cd "$src_dir" && git show "${c}:src/public_settings.h" 2>/dev/null \
                | grep -oP '#define\s+EPOCH\s+\K[0-9]+' || true)
            [ -n "$ep" ] && log_error "  epoch ${ep} (${c:0:8})"
        done | sort -u
        exit 1
    fi

    log_info "checking out commit ${target_commit:0:8} for epoch ${epoch}..."
    cd "$src_dir" && git checkout "$target_commit" --quiet
    rm -rf build

    # verify
    local verify_epoch
    verify_epoch=$(grep -oP '#define\s+EPOCH\s+\K[0-9]+' "$settings_file" || true)
    local verify_tick
    verify_tick=$(grep -oP '#define\s+TICK\s+\K[0-9]+' "$settings_file" || true)
    log_ok "checked out epoch ${verify_epoch} (TICK ${verify_tick}, commit ${target_commit:0:8})"
}

# --- patch max processors ---

patch_max_processors() {
    local src_dir="$1"
    local max_procs="$2"
    local settings_file="${src_dir}/src/public_settings.h"

    if [ -z "$max_procs" ]; then
        return
    fi

    if [ ! -f "$settings_file" ]; then
        log_warn "public_settings.h not found, skipping processor patch"
        return
    fi

    log_info "patching MAX_NUMBER_OF_PROCESSORS to ${max_procs}..."
    # patch both TESTNET and mainnet definitions
    sed -i "s/#define MAX_NUMBER_OF_PROCESSORS [0-9]*/#define MAX_NUMBER_OF_PROCESSORS ${max_procs}/g" "$settings_file"

    # verify (check that all occurrences are patched)
    local verify_count
    verify_count=$(grep -c "#define MAX_NUMBER_OF_PROCESSORS ${max_procs}" "$settings_file" 2>/dev/null || echo 0)
    if [ "$verify_count" -ge 1 ]; then
        log_ok "MAX_NUMBER_OF_PROCESSORS set to ${max_procs} (${verify_count} occurrence(s))"
    else
        log_warn "patch may have failed, check public_settings.h"
    fi
}

# --- status output ---

print_status_docker() {
    local mode_label="mainnet"
    [ "$TESTNET" = true ] && mode_label="testnet"

    echo ""
    echo -e "${GREEN}--- lite node ready (${mode_label}) ---${NC}"
    echo "  dir:      ${DATA_DIR}"
    echo "  P2P:      ${NODE_PORT}"
    echo "  HTTP/RPC: http://localhost:${HTTP_PORT}"
    [ -n "$PEERS" ] && echo "  peers:    ${PEERS}"
    echo ""
    echo "  docker compose -f ${DATA_DIR}/docker-compose.yml ps       # status"
    echo "  docker compose -f ${DATA_DIR}/docker-compose.yml logs -f  # logs"
    echo ""
    echo "  http://localhost:${HTTP_PORT}/live/v1   # live status"
    echo "  http://localhost:${HTTP_PORT}/query/v1  # query API"
    echo ""
    if [ "$TESTNET" = false ]; then
        echo "  epoch data: ${DATA_DIR}/data"
        echo "  update epochs: https://storage.qubic.li/network/"
        echo ""
    fi
}

# --- uninstall ---

do_uninstall() {
    log_info "uninstalling lite node..."

    # stop and remove docker containers
    if [ -f "${DATA_DIR}/docker-compose.yml" ]; then
        log_info "stopping docker containers..."
        docker compose -f "${DATA_DIR}/docker-compose.yml" down -v 2>/dev/null || true
        log_ok "containers stopped and volumes removed"
    fi

    # remove docker image
    if docker image inspect qubic-lite-node &>/dev/null; then
        log_info "removing docker image..."
        docker rmi qubic-lite-node 2>/dev/null || true
        log_ok "docker image removed"
    fi

    # stop systemd service if exists
    if systemctl is-active --quiet qubic-lite 2>/dev/null; then
        log_info "stopping systemd service..."
        systemctl stop qubic-lite
        systemctl disable qubic-lite
        rm -f /etc/systemd/system/qubic-lite.service
        systemctl daemon-reload
        log_ok "service removed"
    elif [ -f /etc/systemd/system/qubic-lite.service ]; then
        log_info "removing systemd service file..."
        systemctl disable qubic-lite 2>/dev/null || true
        rm -f /etc/systemd/system/qubic-lite.service
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
    log_ok "lite node uninstalled"
}

# --- management commands ---

check_installed() {
    if [ ! -f "${DATA_DIR}/docker-compose.yml" ]; then
        log_error "lite node not installed in ${DATA_DIR}"
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
    log_info "checking HTTP endpoint..."
    local api_response
    api_response=$(curl -sf --max-time 5 "http://localhost:${HTTP_PORT}/live/v1" 2>/dev/null) && {
        echo "$api_response" | head -c 500
        echo ""
    } || log_warn "HTTP not responding on port ${HTTP_PORT}"
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
    log_info "rebuilding docker image..."
    docker build -t qubic-lite-node "${DATA_DIR}"
    log_info "restarting container..."
    docker compose -f "${DATA_DIR}/docker-compose.yml" up -d
    log_ok "update complete"
}

# --- interactive setup ---

interactive_setup() {
    echo -e "${CYAN}┌─────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}  ${GREEN}INSTALL${NC}                                        ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}    1) docker       install via docker           ${CYAN}│${NC}"
    echo -e "${CYAN}│${NC}    2) uninstall    remove lite node             ${CYAN}│${NC}"
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

    # skip remaining prompts for uninstall and management commands
    if [ "$MODE" = "uninstall" ] || [ "$MODE" = "status" ] || [ "$MODE" = "logs" ] || \
       [ "$MODE" = "stop" ] || [ "$MODE" = "start" ] || [ "$MODE" = "restart" ] || [ "$MODE" = "update" ]; then
        return
    fi

    echo ""
    echo -e "${CYAN}┌─────────────────────────────────────────────────┐${NC}"
    echo -e "${CYAN}│${NC}  ${GREEN}NODE CONFIGURATION${NC}                             ${CYAN}│${NC}"
    echo -e "${CYAN}└─────────────────────────────────────────────────┘${NC}"
    echo ""
    while [ -z "$OPERATOR_SEED" ]; do
        read -rp "  Operator seed: " OPERATOR_SEED
        # Strip surrounding quotes and whitespace from pasted input
        OPERATOR_SEED=$(echo "$OPERATOR_SEED" | sed "s/^[[:space:]]*[\"']*//; s/[\"']*[[:space:]]*$//")
        [ -z "$OPERATOR_SEED" ] && echo -e "  ${RED}Operator seed is required.${NC}"
    done

    while [ -z "$OPERATOR_ALIAS" ]; do
        read -rp "  Operator alias: " OPERATOR_ALIAS
        # Strip surrounding quotes and whitespace from pasted input
        OPERATOR_ALIAS=$(echo "$OPERATOR_ALIAS" | sed "s/^[[:space:]]*[\"']*//; s/[\"']*[[:space:]]*$//")
        [ -z "$OPERATOR_ALIAS" ] && echo -e "  ${RED}Operator alias is required.${NC}"
    done

    echo ""
    read -rp "  Max processors (Enter=8): " input_processors
    if [ -n "$input_processors" ] && [ "$input_processors" != "0" ]; then
        MAX_PROCESSORS="$input_processors"
    fi

    # automatically fetch peers from API
    echo ""
    PEERS=$(fetch_peers_from_api) || true

    if [ -z "$PEERS" ]; then
        echo ""
        log_warn "automatic peer discovery failed"
        read -rp "  Enter peers manually (ip1,ip2, comma-separated): " PEERS
        # sanitize: remove leading # or whitespace
        PEERS=$(echo "$PEERS" | sed 's/^[#[:space:]]*//')

        if [ -z "$PEERS" ]; then
            log_error "at least one peer is required to start the node"
            log_error "find peers at: https://app.qubic.li/network/live"
            exit 1
        fi
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
            --peers)         PEERS="$2";         shift 2 ;;
            --testnet)       TESTNET=true;       shift   ;;
            --port)          NODE_PORT="$2";      shift 2 ;;
            --http-port)     HTTP_PORT="$2";      shift 2 ;;
            --data-dir)      DATA_DIR="$2";       shift 2 ;;
            --operator-seed) OPERATOR_SEED="$2";  shift 2 ;;
            --operator-alias) OPERATOR_ALIAS="$2"; shift 2 ;;
            --avx512)        ENABLE_AVX512=true;  shift   ;;
            --security-tick) SECURITY_TICK="$2";  shift 2 ;;
            --ticking-delay) TICKING_DELAY="$2";  shift 2 ;;
            --epoch)         TARGET_EPOCH="$2";   shift 2 ;;
            --no-epoch)      SKIP_EPOCH=true;     shift   ;;
            --threads)       BUILD_THREADS="$2";  shift 2 ;;
            --processors)    MAX_PROCESSORS="$2"; shift 2 ;;
            --help|-h)       print_usage;         exit 0  ;;
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
    echo -e "${GREEN}      Qubic Lite Node Installer${NC}"
    echo -e "      ──────────────────────────"
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

    if [ -z "$OPERATOR_SEED" ]; then
        log_error "--operator-seed is required."
        print_usage
        exit 1
    fi
    validate_operator_params "operator-seed" "$OPERATOR_SEED"

    if [ -z "$OPERATOR_ALIAS" ]; then
        log_error "--operator-alias is required."
        print_usage
        exit 1
    fi
    validate_operator_params "operator-alias" "$OPERATOR_ALIAS"

    # auto-fetch peers if not provided via CLI
    if [ -z "$PEERS" ]; then
        PEERS=$(fetch_peers_from_api) || true
        if [ -z "$PEERS" ]; then
            log_error "no peers available and API unreachable"
            log_error "use --peers <ip1,ip2,...> or find peers at: https://app.qubic.li/network/live"
            exit 1
        fi
    fi

    check_system

    case "$MODE" in
        docker) install_docker ;;
        *) log_error "unknown mode: ${MODE}"; print_usage; exit 1 ;;
    esac

    # copy script to install directory for future management
    if [ -f "$SELF" ] && [ "$SELF" != "${DATA_DIR}/lite-install.sh" ]; then
        cp "$SELF" "${DATA_DIR}/lite-install.sh"
        chmod +x "${DATA_DIR}/lite-install.sh"
        log_ok "script copied to ${DATA_DIR}/lite-install.sh"
    fi
}

main "$@"
