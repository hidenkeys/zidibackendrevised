#!/usr/bin/env bash

set -euo pipefail

container_name="zidi-commerce-e2e-$$"

cleanup() {
  docker stop "$container_name" >/dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM

docker run --rm --detach \
  --name "$container_name" \
  --publish 127.0.0.1::5432 \
  --env POSTGRES_USER=zidi_test \
  --env POSTGRES_PASSWORD=zidi_test \
  --env POSTGRES_DB=zidi_commerce_e2e \
  postgres:16-alpine >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$container_name" pg_isready --username zidi_test --dbname zidi_commerce_e2e >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

if ! docker exec "$container_name" pg_isready --username zidi_test --dbname zidi_commerce_e2e >/dev/null 2>&1; then
	 echo "PostgreSQL did not become ready" >&2
	 docker logs "$container_name" >&2 || true
	 exit 1
fi

host_binding="$(docker port "$container_name" 5432/tcp)"
host_port="${host_binding##*:}"

export COMMERCE_TEST_DATABASE_URL="postgres://zidi_test:zidi_test@127.0.0.1:${host_port}/zidi_commerce_e2e?sslmode=disable"
export GOCACHE="${GOCACHE:-/tmp/zidi-commerce-go-cache}"

go test -tags=integration ./e2e -count=1 -v
