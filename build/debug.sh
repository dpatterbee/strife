#!/bin/sh

run() {
  DOCKER_BUILDKIT=1 docker build \
    --build-arg "LITESTREAM_ACCESS_KEY_ID=$LITESTREAM_ACCESS_KEY_ID" \
    --build-arg "LITESTREAM_SECRET_ACCESS_KEY=$LITESTREAM_SECRET_ACCESS_KEY" \
    --build-arg "DB_REPLICA_URL=$DB_REPLICA_URL" \
    --build-arg "TOKEN=$TOKEN" \
    -t strife-debug -f Dockerfile.debug .

  docker run -v "$LOG_DIR":/logs -dit strife-debug
  exit 0
}

error() {
  echo "striferc not correctly configured."
  exit 1
}

if [ -f striferc ]; then
  . "$(pwd)/striferc"
  run
fi
if [ -f /root/striferc ]; then
  . /root/striferc
  run
fi
error

