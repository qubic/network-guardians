#!/usr/bin/env bash
# bob-clean-ticks.sh — pause bob, delete bad tick_data/tick_vote entries from
# both keydb instances inside the guardian bob container, then restart bob
# (by killing the worker and letting bob's own watchdog respawn it).
#
# Usage:
#   bob-clean-ticks.sh [-c CONTAINER] [-p PORT[,PORT...]] [-m MAX_VOTE] TICK [TICK...]
#
# Examples:
#   bob-clean-ticks.sh 49124963 49124964 49124965
#   bob-clean-ticks.sh -c qubic-bob -p 6379,6666 49124963
set -euo pipefail

CONTAINER="qubic-bob"
PORTS=(6379 6666)
MAX_VOTE=676
BOB_PATTERN="/app/bob"
WORKER_PATTERN="no-watchdog"   # matches the worker process spawned by the watchdog

usage() {
    sed -n '2,13p' "$0"
    exit "${1:-0}"
}

while getopts ":c:p:m:h" opt; do
    case "$opt" in
        c) CONTAINER="$OPTARG" ;;
        p) IFS=',' read -r -a PORTS <<< "$OPTARG" ;;
        m) MAX_VOTE="$OPTARG" ;;
        h) usage 0 ;;
        *) usage 1 ;;
    esac
done
shift $((OPTIND - 1))

if [ "$#" -eq 0 ]; then
    echo "error: at least one tick must be provided" >&2
    usage 1
fi

TICKS=("$@")
for t in "${TICKS[@]}"; do
    [[ "$t" =~ ^[0-9]+$ ]] || { echo "error: tick '$t' is not a number" >&2; exit 1; }
done

if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    echo "Container '$CONTAINER' is not running." >&2
    exit 1
fi

CLI="keydb-cli"
if ! docker exec "$CONTAINER" sh -c "command -v $CLI >/dev/null 2>&1"; then
    CLI="redis-cli"
fi
echo "Container: $CONTAINER"
echo "CLI:       $CLI"
echo "Ports:     ${PORTS[*]}"
echo "Ticks:     ${TICKS[*]}"
echo "Max vote:  $MAX_VOTE"

resume_bob() {
    echo "Resuming any paused bob processes (SIGCONT)..."
    docker exec "$CONTAINER" pkill -CONT -f "$BOB_PATTERN" || true
}
trap resume_bob ERR
trap 'resume_bob' EXIT

echo "Pausing bob (SIGSTOP on all /app/bob processes)..."
docker exec "$CONTAINER" pkill -STOP -f "$BOB_PATTERN"
if ! docker exec "$CONTAINER" sh -c "ps -eo stat,args | grep -v grep | grep -q '^T.*${BOB_PATTERN}'"; then
    echo "WARNING: could not confirm bob is paused — aborting" >&2
    exit 1
fi

for port in "${PORTS[@]}"; do
    echo "=== port $port ==="
    if ! docker exec "$CONTAINER" "$CLI" -p "$port" ping >/dev/null 2>&1; then
        echo "WARNING: no response on port $port, skipping" >&2
        continue
    fi

    echo "Deleting tick_data keys on $port..."
    for t in "${TICKS[@]}"; do
        docker exec "$CONTAINER" "$CLI" -p "$port" del "tick_data:$t"
    done

    echo "Deleting tick_vote keys on $port..."
    for t in "${TICKS[@]}"; do
        keys=$(seq 0 "$MAX_VOTE" | awk -v t="$t" '{printf "tick_vote:%s:%s ", t, $1}')
        docker exec "$CONTAINER" sh -c "$CLI -p $port del $keys"
    done
done

# --- Restart bob ---
# 1. Kill the worker (SIGKILL, since it's currently SIGSTOPped and can't handle signals otherwise).
# 2. SIGCONT the watchdog so it wakes up, notices the dead child, and respawns it.
echo "Killing bob worker ($WORKER_PATTERN)..."
docker exec "$CONTAINER" pkill -KILL -f "$WORKER_PATTERN" || true

echo "Resuming watchdog so it respawns the worker..."
trap - ERR EXIT
docker exec "$CONTAINER" pkill -CONT -f "$BOB_PATTERN" || true

# Give the watchdog a moment and verify a fresh worker is up
sleep 2
if docker exec "$CONTAINER" sh -c "pgrep -f '$WORKER_PATTERN' >/dev/null"; then
    echo "Bob worker is back up."
else
    echo "WARNING: bob worker did not come back up — check 'docker logs $CONTAINER'" >&2
    exit 1
fi

echo "Done."
