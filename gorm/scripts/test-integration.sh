#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
module_directory="$(cd "$script_directory/.." && pwd)"
go_command="${GO_COMMAND:-go}"

mysql_image="${WEAVE_GORM_MYSQL_IMAGE:-mysql@sha256:d58ac93387f644e4e040c636b8f50494e78e5afc27ca0a87348b2f577da2b7ff}"
postgres_image="${WEAVE_GORM_POSTGRES_IMAGE:-postgres@sha256:ef9d1517df69c4d27dbb9ddcec14f431a2442628603f4e9daa429b92ae6c3cd1}"
mysql_container="weave-gorm-mysql-$$"
postgres_container="weave-gorm-postgresql-$$"
mysql_password="weave-gorm-local"
postgres_password="weave-gorm-local"

cleanup() {
  local container
  for container in "$mysql_container" "$postgres_container"; do
    if docker container inspect "$container" >/dev/null 2>&1; then
      docker container rm --force "$container" >/dev/null
    fi
  done
  printf 'Removed integration containers; their tmpfs database data was discarded.\n'
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

wait_for_mysql() {
  local attempt
  for attempt in {1..60}; do
    if docker exec "$mysql_container" mysqladmin ping \
      --host=127.0.0.1 \
      --user=root \
      "--password=$mysql_password" \
      --silent >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  docker logs --tail 100 "$mysql_container" >&2
  return 1
}

wait_for_postgresql() {
  local attempt
  for attempt in {1..60}; do
    if docker exec "$postgres_container" pg_isready \
      --username=weave \
      --dbname=weave_gorm >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  docker logs --tail 100 "$postgres_container" >&2
  return 1
}

docker run --detach \
  --name "$mysql_container" \
  --tmpfs /var/lib/mysql:rw,noexec,nosuid,size=512m \
  --env MYSQL_ROOT_PASSWORD="$mysql_password" \
  --env MYSQL_DATABASE=weave_gorm \
  --publish 127.0.0.1::3306 \
  "$mysql_image" \
  --character-set-server=utf8mb4 \
  --collation-server=utf8mb4_bin >/dev/null

docker run --detach \
  --name "$postgres_container" \
  --tmpfs /var/lib/postgresql/data:rw,noexec,nosuid,size=512m \
  --env POSTGRES_USER=weave \
  --env POSTGRES_PASSWORD="$postgres_password" \
  --env POSTGRES_DB=weave_gorm \
  --env 'POSTGRES_INITDB_ARGS=--locale=C --encoding=UTF8' \
  --publish 127.0.0.1::5432 \
  "$postgres_image" >/dev/null

wait_for_mysql
wait_for_postgresql

mysql_port="$(docker port "$mysql_container" 3306/tcp | sed -n 's/.*://p' | tail -n 1)"
postgres_port="$(docker port "$postgres_container" 5432/tcp | sed -n 's/.*://p' | tail -n 1)"
[[ -n "$mysql_port" && -n "$postgres_port" ]]

docker exec "$mysql_container" mysqld --version
docker exec "$postgres_container" postgres --version

cd "$module_directory"
WEAVE_GORM_MYSQL_DSN="root:${mysql_password}@tcp(127.0.0.1:${mysql_port})/weave_gorm?charset=utf8mb4&parseTime=true&loc=UTC" \
WEAVE_GORM_POSTGRES_DSN="host=127.0.0.1 port=${postgres_port} user=weave password=${postgres_password} dbname=weave_gorm sslmode=disable" \
  "$go_command" test -mod=readonly -race -tags=integration -count=1 -run '^TestIntegrationProfiles$' -v .
