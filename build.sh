#!/bin/sh

build() {
  DOCKER_BUILDKIT=1 docker build \
    --build-arg "LITESTREAM_ACCESS_KEY_ID=$LITESTREAM_ACCESS_KEY_ID" \
    --build-arg "LITESTREAM_SECRET_ACCESS_KEY=$LITESTREAM_SECRET_ACCESS_KEY" \
    --build-arg "DB_REPLICA_URL=$DB_REPLICA_URL" \
    --build-arg "TOKEN=$TOKEN" \
    -t strife .
  exit 0
}

error() {
  echo "striferc not correctly configured."
  exit 1
}

if [ -f striferc ]; then
  . "$(pwd)/striferc"
  build
fi
if [ -f /root/striferc ]; then
  . /root/striferc
  build
fi
error
