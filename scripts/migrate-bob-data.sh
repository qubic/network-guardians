#!/bin/bash
set -euo pipefail

#=============================================================================
# migrate-bob-data.sh
#
# Migrates data from the running qubic-bob container (which has anonymous
# volumes at /data/kvrocks, /data/redis, /data/bob) into bind-mounted
# directories under ./data/.
#
# Patches the existing docker-compose.yml in place:
#   - Replaces "qubic-bob-data:/data" with three bind mounts
#   - Removes the top-level "volumes:" block for qubic-bob-data
#
# Usage: Run from /opt/qubic-bob (or wherever your docker-compose.yml lives)
#   chmod +x migrate-bob-data.sh
#   ./migrate-bob-data.sh
#=============================================================================

CONTAINER_NAME="qubic-bob"
INSTALL_DIR="$(pwd)"
DATA_DIR="${INSTALL_DIR}/data"
COMPOSE_FILE="docker-compose.yml"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "============================================"
echo " qubic-bob Data Migration Script"
echo " $(date)"
echo "============================================"
echo ""
echo " Install dir: ${INSTALL_DIR}"
echo " Data dir:    ${DATA_DIR}"
echo ""

# --- Pre-flight checks ---
if [ ! -f "$COMPOSE_FILE" ]; then
    echo "ERROR: $COMPOSE_FILE not found in current directory."
    echo "       Please run this script from /opt/qubic-bob"
    exit 1
fi

if ! grep -q "qubic-bob-data:/data" "$COMPOSE_FILE"; then
    echo "ERROR: Could not find 'qubic-bob-data:/data' in $COMPOSE_FILE."
    echo "       Has the migration already been run?"
    exit 1
fi

if ! docker inspect "$CONTAINER_NAME" > /dev/null 2>&1; then
    echo "ERROR: Container '$CONTAINER_NAME' not found."
    exit 1
fi

if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME")" != "true" ]; then
    echo "WARNING: Container '$CONTAINER_NAME' is not running."
    echo "         Data copy may be incomplete if services didn't shut down cleanly."
    read -p "Continue anyway? (y/N): " confirm
    [[ "$confirm" =~ ^[Yy]$ ]] || exit 0
fi

echo "[1/7] Stopping internal services inside the container..."
echo "       This ensures redis/kvrocks flush data to disk."
echo ""

docker exec "$CONTAINER_NAME" /bin/sh -c '
    if command -v supervisorctl > /dev/null 2>&1; then
        echo "  -> Stopping all supervised services..."
        supervisorctl stop all 2>/dev/null || true
        sleep 3
        echo "  -> Services stopped."
    else
        echo "  -> supervisorctl not found, attempting direct process kill..."
        kill $(pgrep -f redis-server 2>/dev/null) 2>/dev/null || true
        kill $(pgrep -f kvrocks 2>/dev/null) 2>/dev/null || true
        sleep 3
        echo "  -> Processes signaled."
    fi
' || {
    echo "WARNING: Could not stop internal services. Proceeding with copy anyway."
}

echo ""
echo "[2/7] Creating data directories..."
mkdir -p "${DATA_DIR}/kvrocks" "${DATA_DIR}/redis" "${DATA_DIR}/bob"

echo ""
echo "[3/7] Copying data from container to host..."
echo ""

for subdir in kvrocks redis bob; do
    echo "  -> /data/${subdir} => ${DATA_DIR}/${subdir}/"
    docker cp "$CONTAINER_NAME":/data/${subdir}/. "${DATA_DIR}/${subdir}/"
done

echo ""
echo "       Data sizes:"
du -sh "${DATA_DIR}"/* 2>/dev/null || echo "       (empty)"
echo ""
echo "       Total:"
du -sh "${DATA_DIR}"
echo ""

# Verify we got data
EMPTY=""
for subdir in kvrocks redis bob; do
    if [ ! "$(ls -A "${DATA_DIR}/${subdir}" 2>/dev/null)" ]; then
        EMPTY="$EMPTY $subdir"
    fi
done

if [ -n "$EMPTY" ]; then
    echo "WARNING: The following directories are empty after copy:$EMPTY"
    read -p "Continue anyway? (y/N): " confirm
    [[ "$confirm" =~ ^[Yy]$ ]] || exit 0
fi

echo "[4/7] Backing up current docker-compose.yml..."
cp "$COMPOSE_FILE" "${COMPOSE_FILE}.backup.${TIMESTAMP}"
echo "       Saved as ${COMPOSE_FILE}.backup.${TIMESTAMP}"
echo ""

echo "[5/7] Stopping container with docker compose..."
docker compose down
echo ""

echo "[6/7] Patching docker-compose.yml..."

# Get the indentation used for the existing volume line
INDENT=$(grep "qubic-bob-data:/data" "$COMPOSE_FILE" | sed 's/\(-.*\)//')

# Replace the named volume mount with three bind mounts
sed -i "s|^.*- qubic-bob-data:/data.*$|${INDENT}- ./data/kvrocks:/data/kvrocks\n${INDENT}- ./data/redis:/data/redis\n${INDENT}- ./data/bob:/data/bob|" "$COMPOSE_FILE"

# Remove the top-level volumes block and the qubic-bob-data entry
# This handles both "volumes:" followed by "  qubic-bob-data:" at the end of file
sed -i '/^volumes:/,/^[^ ]/{/^volumes:/d; /^[[:space:]]*qubic-bob-data:/d; /^[[:space:]]*$/d}' "$COMPOSE_FILE"

# Clean up any trailing blank lines at end of file
sed -i -e :a -e '/^\n*$/{$d;N;ba}' "$COMPOSE_FILE"

echo "       Patched. Changes:"
diff "${COMPOSE_FILE}.backup.${TIMESTAMP}" "$COMPOSE_FILE" || true
echo ""

echo "[7/7] Starting services..."
docker compose up -d
echo ""

# Wait and verify
sleep 5
if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null)" == "true" ]; then
    echo "============================================"
    echo " Migration complete!"
    echo "============================================"
    echo ""
    echo " Container is running with bind mounts:"
    docker inspect "$CONTAINER_NAME" --format '{{range .Mounts}}  {{.Type}}  {{.Source}} -> {{.Destination}}{{"\n"}}{{end}}'
    echo ""
    echo " All data is now under: ${DATA_DIR}/"
    ls -la "${DATA_DIR}/"
    echo ""
    echo " Compose backup at: ${COMPOSE_FILE}.backup.${TIMESTAMP}"
    echo ""
    echo " Cleaning up orphaned Docker volumes..."
    docker volume rm "qubic-bob_qubic-bob-data" 2>/dev/null && \
        echo "   Removed qubic-bob_qubic-bob-data" || \
        echo "   qubic-bob_qubic-bob-data already removed"
    echo ""
    echo "   Pruning remaining anonymous volumes..."
    docker volume prune -f
else
    echo "WARNING: Container does not appear to be running!"
    echo "         Check logs with: docker compose logs"
    echo "         Your data is safe at: ${DATA_DIR}/"
    echo "         Your original compose file: ${COMPOSE_FILE}.backup.${TIMESTAMP}"
fi
