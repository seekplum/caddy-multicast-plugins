#!/bin/bash

set -e
ETVAL=0

ROOT_DIR="$(cd "$(dirname "$BASH_SOURCE[0]")" && pwd)"
NAMESPACE="multicast"
COMPOST_FILE="${ROOT_DIR}/docker/docker-compose.yml"
CADDY_VERSION="2.8.4"
CADDY_IMAGE="seekplum/caddy-multicast:${CADDY_VERSION}"
export CADDY_VERSION=${CADDY_VERSION}
export CADDY_IMAGE=${CADDY_IMAGE}

if command -v docker-compose &>/dev/null; then
    docker_compose_command="docker-compose -p ${NAMESPACE} -f ${COMPOST_FILE}"
else
    docker_compose_command="docker compose -p ${NAMESPACE} -f ${COMPOST_FILE}"
fi

function print_error() {
    echo -e "\033[31m$1\033[0m"
}

function dco() {
    ${docker_compose_command} $*
}

function dco_shell() {
    ${docker_compose_command} exec $1 sh -c "${@:2}"
}

function reload() {
    ${docker_compose_command} exec caddy caddy reload --config /etc/caddy/Caddyfile --force
}

function upload {
    docker push ${CADDY_IMAGE}
}

print_help() {
    echo "Usage: bash $0 {-h|dco|build|up|down|up-force}"
    echo "e.g: $0 dco config"
}

# 命令行参数小于 1 时打印提示信息后退出
if [ $# -lt 1 ]; then
    print_help
    exit 1
fi

case "$1" in
dco)
    dco ${@:2}
    ;;
build)
    dco build
    ;;
up)
    dco up -d
    ;;
down)
    dco down --remove-orphans -v
    ;;
up-force)
    dco up -d --force-recreate
    ;;
reload)
    reload
    ;;
upload)
    upload
    ;;
-h | --help)
    print_help
    ETVAL=1
    ;;
*) # 匹配都失败执行
    print_help
    ETVAL=1
    ;;
esac

exit ${ETVAL}
