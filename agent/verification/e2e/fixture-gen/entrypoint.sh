#!/bin/bash
# Produces /artifacts/good-pgN.dump for each supported Postgres major and a
# single /artifacts/broken.dump sentinel. For each major: spawn a sibling
# postgres:N container against the host daemon, wait for pg_isready, seed the
# t_a + t_b schema, pg_dump -Fc to the artifacts dir, and remove the
# container. Producer and consumer (pg_restore in the agent's spawned target)
# share the major so the custom-format archive header matches.
set -euo pipefail

VERSIONS=(12 13 14 15 16 17 18)
ARTIFACTS=/artifacts
BROKEN="$ARTIFACTS/broken.dump"

mkdir -p "$ARTIFACTS"

CID=""
cleanup() {
  if [ -n "${CID:-}" ]; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

for V in "${VERSIONS[@]}"; do
  CID="$(docker run -d -e POSTGRES_PASSWORD=test "postgres:$V")"

  READY=0
  for _ in $(seq 1 30); do
    if docker exec "$CID" pg_isready -U postgres -h 127.0.0.1 >/dev/null 2>&1; then
      READY=1
      break
    fi
    sleep 1
  done

  if [ "$READY" -ne 1 ]; then
    echo "FAIL: postgres:$V never became ready within 30s"
    docker logs "$CID" 2>&1 | tail -30
    exit 1
  fi

  docker exec -i "$CID" psql -U postgres -d postgres -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE public.t_a (id int PRIMARY KEY, name text);
CREATE TABLE public.t_b (
  id int PRIMARY KEY,
  a_id int NOT NULL REFERENCES public.t_a(id),
  value text
);
INSERT INTO public.t_a (id, name) VALUES (1, 'alpha'), (2, 'beta'), (3, 'gamma');
INSERT INTO public.t_b (id, a_id, value) VALUES
  (1, 1, 'x'), (2, 1, 'y'), (3, 2, 'z'), (4, 3, 'w'), (5, 3, 'v');
SQL

  OUT="$ARTIFACTS/good-pg${V}.dump"
  docker exec "$CID" pg_dump -Fc -U postgres postgres > "$OUT"

  docker rm -f "$CID" >/dev/null
  CID=""

  echo "fixture: good-pg${V}.dump=$(stat -c%s "$OUT")B"
done

printf 'not-a-valid-pg-dump-custom-format-archive\n' > "$BROKEN"
chmod 644 "$ARTIFACTS"/good-pg*.dump "$BROKEN"
echo "fixture: broken.dump=$(stat -c%s "$BROKEN")B"
