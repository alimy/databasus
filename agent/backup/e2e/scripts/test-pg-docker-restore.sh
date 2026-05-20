#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(dirname "$0")"
source "$SCRIPT_DIR/backup-restore-helpers.sh"

MOCK_SERVER="${MOCK_SERVER_OVERRIDE:-http://e2e-br-mock-server:4050}"
PG_VERSION="${PG_VERSION:-17}"
PG_IMAGE="postgres:${PG_VERSION}"
BR_NETWORK="${BR_NETWORK:-br-net-${PG_VERSION}}"
RESTORED_VOLUME="${RESTORED_VOLUME:-br-docker-restored-${PG_VERSION}}"
WAL_QUEUE_VOLUME="${WAL_QUEUE_VOLUME:-br-docker-wal-queue-${PG_VERSION}}"

PG_ORIGINAL="pg-original-${PG_VERSION}"
PG_RESTORED="pg-restored-${PG_VERSION}"

PG_MAJOR=$(echo "$PG_VERSION" | grep -oE '^[0-9]+')
if [ "$PG_MAJOR" -ge 18 ]; then
  CONTAINER_PGDATA="/var/lib/postgresql/${PG_MAJOR}/docker"
else
  CONTAINER_PGDATA="/var/lib/postgresql/data"
fi

PG_USER=testuser
PG_PASSWORD=testpassword
PG_DB=testdb
PG_PORT=5432

cleanup_containers() {
  docker rm -f "$PG_ORIGINAL" 2>/dev/null || true
  docker rm -f "$PG_RESTORED" 2>/dev/null || true
}

trap cleanup_containers EXIT

echo "=== PG ${PG_VERSION} docker-mode backup-restore lifecycle ==="
echo "  Image: $PG_IMAGE"
echo "  Container PGDATA: $CONTAINER_PGDATA"
echo "  Network: $BR_NETWORK"
echo "  Restored volume: $RESTORED_VOLUME"
echo "  WAL queue volume: $WAL_QUEUE_VOLUME"

echo "=== Phase 1: Setup agent ==="
setup_agent

echo "=== Phase 2: Pre-clean any stale containers and volume contents ==="
cleanup_containers
docker run --rm -v "${RESTORED_VOLUME}:/v" alpine:3.21 sh -c 'rm -rf /v/* /v/.[!.]* 2>/dev/null || true'
docker run --rm -v "${WAL_QUEUE_VOLUME}:/v" alpine:3.21 sh -c 'rm -rf /v/* /v/.[!.]* 2>/dev/null || true; chmod 777 /v'

echo "=== Phase 3: Start original postgres:${PG_VERSION} container ==="
docker run -d \
  --name "$PG_ORIGINAL" \
  --network "$BR_NETWORK" \
  -e POSTGRES_USER="$PG_USER" \
  -e POSTGRES_PASSWORD="$PG_PASSWORD" \
  -e POSTGRES_DB="$PG_DB" \
  -v "${WAL_QUEUE_VOLUME}:/wal-queue" \
  "$PG_IMAGE" \
  -c wal_level=replica \
  -c max_wal_senders=3 \
  -c archive_mode=on \
  -c "archive_command=test ! -f /wal-queue/%f && cp %p /wal-queue/%f" \
  -c checkpoint_timeout=30s \
  > /dev/null

# pg_isready returns OK during the docker entrypoint's brief init phase before
# PG is restarted as the final server. Issue a real query against the user's DB
# instead, which only succeeds against the final server.
for i in $(seq 1 60); do
  if docker exec "$PG_ORIGINAL" psql -U "$PG_USER" -d "$PG_DB" -t -A -c "SELECT 1" 2>/dev/null | grep -q '^1$'; then
    echo "Original PG ready after ${i}s"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "FAIL: Original PG did not become ready"
    docker logs "$PG_ORIGINAL" 2>&1 | tail -30
    exit 1
  fi
  sleep 1
done

echo "=== Phase 4: Insert test data into original PG ==="
docker exec "$PG_ORIGINAL" psql -U "$PG_USER" -d "$PG_DB" -c "
CREATE TABLE IF NOT EXISTS e2e_test_data (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    value INT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
DELETE FROM e2e_test_data;
INSERT INTO e2e_test_data (name, value) VALUES
    ('row1', 100),
    ('row2', 200),
    ('row3', 300);
CHECKPOINT;
"

echo "=== Phase 5: Start agent backup (docker exec mode) ==="
curl -sf -X POST "$MOCK_SERVER/mock/reset" > /dev/null
curl -sf -X POST "$MOCK_SERVER/mock/set-version" \
  -H "Content-Type: application/json" \
  -d '{"version":"v1.0.0"}' > /dev/null

cd /tmp
cat > databasus.json <<AGENTCONF
{
  "databasusHost": "$MOCK_SERVER",
  "dbId": "test-db-id",
  "token": "test-token",
  "pgHost": "$PG_ORIGINAL",
  "pgPort": $PG_PORT,
  "pgUser": "$PG_USER",
  "pgPassword": "$PG_PASSWORD",
  "pgType": "docker",
  "pgDockerContainerName": "$PG_ORIGINAL",
  "pgWalDir": "/wal-queue",
  "deleteWalAfterUpload": true
}
AGENTCONF

"$AGENT" _run > /tmp/agent-output.log 2>&1 &
AGENT_PID=$!
echo "Agent started with PID $AGENT_PID"

echo "=== Phase 6: Generate WAL while backup runs ==="
(
  while true; do
    docker exec "$PG_ORIGINAL" psql -U "$PG_USER" -d "$PG_DB" -c "
      INSERT INTO e2e_test_data (name, value)
      SELECT 'bulk_' || g, g FROM generate_series(1, 1000) g;
      SELECT pg_switch_wal();
    " > /dev/null 2>&1 || break
    sleep 2
  done
) &
WAL_GEN_PID=$!

echo "=== Phase 7: Wait for backup to complete ==="
wait_for_backup_complete "$MOCK_SERVER" 180

# The PITR target must sit BETWEEN the backup checkpoint and the last uploaded
# WAL's commit times. Capture target now, then drive enough WAL traffic and
# wait long enough for the agent's 10s WAL streamer cycle to push commits past
# the target into the mock.
PITR_TARGET=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "Captured PITR target time: $PITR_TARGET"

initial_wal_count=$(curl -sf "$MOCK_SERVER/mock/backup-status" 2>/dev/null | grep -o '"walSegmentCount":[0-9]*' | grep -o '[0-9]*$' || echo "0")
echo "WAL count at target-time capture: $initial_wal_count"

# Drive WAL past the target time.
sleep 15

# Force a final WAL switch so the trailing segment is sealed and eligible for upload.
docker exec "$PG_ORIGINAL" psql -U "$PG_USER" -d "$PG_DB" -c "
  INSERT INTO e2e_test_data (name, value) VALUES ('post_target', 9999);
  SELECT pg_switch_wal();
" > /dev/null 2>&1 || true

# Wait for the agent's WAL streamer (10s polling) to upload at least 3 more segments.
for i in $(seq 1 90); do
  current_wal_count=$(curl -sf "$MOCK_SERVER/mock/backup-status" 2>/dev/null | grep -o '"walSegmentCount":[0-9]*' | grep -o '[0-9]*$' || echo "0")
  if [ "$current_wal_count" -ge $((initial_wal_count + 3)) ]; then
    echo "WAL count grew from $initial_wal_count to $current_wal_count past target"
    break
  fi
  if [ "$i" -eq 90 ]; then
    echo "WARN: WAL count only reached $current_wal_count (wanted $((initial_wal_count + 3)))"
  fi
  sleep 1
done

echo "=== Phase 8: Stop WAL generator, agent, and original PG ==="
kill $WAL_GEN_PID 2>/dev/null || true
wait $WAL_GEN_PID 2>/dev/null || true
stop_agent
docker stop "$PG_ORIGINAL" > /dev/null
docker rm "$PG_ORIGINAL" > /dev/null

run_docker_restore_lifecycle() {
  local target_time="$1"
  local lifecycle_label="$2"

  echo "=== ${lifecycle_label}: agent restore ==="
  rm -rf /restored-pgdata/* /restored-pgdata/.[!.]* 2>/dev/null || true

  local restore_args=(restore --skip-update --databasus-host "$MOCK_SERVER" --token test-token --target-dir /restored-pgdata)
  if [ -n "$target_time" ]; then
    restore_args+=(--target-time "$target_time")
  fi

  "$AGENT" "${restore_args[@]}"

  echo "--- postgresql.auto.conf ---"
  cat /restored-pgdata/postgresql.auto.conf

  if [ "$PG_MAJOR" -ge 18 ]; then
    if ! grep -q "/var/lib/postgresql/${PG_MAJOR}/docker/databasus-wal-restore" /restored-pgdata/postgresql.auto.conf; then
      echo "FAIL: postgresql.auto.conf does not contain version-aware WAL restore path for PG ${PG_MAJOR}"
      exit 1
    fi
  else
    if ! grep -q "/var/lib/postgresql/data/databasus-wal-restore" /restored-pgdata/postgresql.auto.conf; then
      echo "FAIL: postgresql.auto.conf does not contain legacy WAL restore path for PG ${PG_MAJOR}"
      exit 1
    fi
  fi

  if [ -n "$target_time" ]; then
    if grep -q "recovery_target_time = '[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}T" /restored-pgdata/postgresql.auto.conf; then
      echo "FAIL: postgresql.auto.conf contains RFC3339 'T' separator — PG would reject it"
      exit 1
    fi
    if ! grep -qE "recovery_target_time = '[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\+00:00'" /restored-pgdata/postgresql.auto.conf; then
      echo "FAIL: postgresql.auto.conf recovery_target_time is not in PG-friendly space+offset format"
      exit 1
    fi
  fi

  echo "=== ${lifecycle_label}: start restored postgres:${PG_VERSION} container ==="
  docker run -d \
    --name "$PG_RESTORED" \
    --network "$BR_NETWORK" \
    -e POSTGRES_USER="$PG_USER" \
    -e POSTGRES_PASSWORD="$PG_PASSWORD" \
    -e POSTGRES_DB="$PG_DB" \
    -v "${RESTORED_VOLUME}:${CONTAINER_PGDATA}" \
    "$PG_IMAGE" \
    > /dev/null

  echo "=== ${lifecycle_label}: wait for recovery + promotion ==="
  local recovered=0
  for i in $(seq 1 90); do
    if docker exec "$PG_RESTORED" pg_isready -U "$PG_USER" -d "$PG_DB" > /dev/null 2>&1; then
      local in_recovery
      in_recovery=$(docker exec "$PG_RESTORED" psql -U "$PG_USER" -d "$PG_DB" -t -A -c "SELECT pg_is_in_recovery();" 2>/dev/null || echo "")
      if [ "$in_recovery" = "f" ]; then
        recovered=1
        echo "Recovered and promoted after ${i}s"
        break
      fi
    fi
    sleep 1
  done

  if [ "$recovered" -ne 1 ]; then
    echo "FAIL: restored PG did not recover within 90 seconds"
    echo "--- container logs ---"
    docker logs "$PG_RESTORED" 2>&1 | tail -50
    exit 1
  fi

  echo "=== ${lifecycle_label}: verify restored data ==="
  local row_count
  row_count=$(docker exec "$PG_RESTORED" psql -U "$PG_USER" -d "$PG_DB" -t -A -c "SELECT COUNT(*) FROM e2e_test_data;")
  if [ "$row_count" -lt 3 ]; then
    echo "FAIL: expected at least 3 rows, got $row_count"
    exit 1
  fi

  local value_row1 value_row3
  value_row1=$(docker exec "$PG_RESTORED" psql -U "$PG_USER" -d "$PG_DB" -t -A -c "SELECT value FROM e2e_test_data WHERE name='row1';")
  value_row3=$(docker exec "$PG_RESTORED" psql -U "$PG_USER" -d "$PG_DB" -t -A -c "SELECT value FROM e2e_test_data WHERE name='row3';")

  if [ "$value_row1" != "100" ] || [ "$value_row3" != "300" ]; then
    echo "FAIL: data integrity check failed (row1=$value_row1, row3=$value_row3)"
    exit 1
  fi

  echo "PASS: ${lifecycle_label} — $row_count rows, data integrity verified"

  docker stop "$PG_RESTORED" > /dev/null
  docker rm "$PG_RESTORED" > /dev/null
}

echo "=== Phase 9: Restore lifecycle WITHOUT PITR ==="
run_docker_restore_lifecycle "" "no-pitr"

echo "=== Phase 10: Restore lifecycle WITH PITR (target-time) ==="
run_docker_restore_lifecycle "$PITR_TARGET" "pitr"

echo "PG ${PG_VERSION} docker-mode backup-restore lifecycle: ALL CHECKS PASSED"
