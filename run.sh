#!/bin/sh

run() {
  docker run -v "$LOG_DIR":/logs -dit strife
  exit 0
}

error() {
  echo "striferc not correctly configured."
  exit 1
}

if [ -f striferc ]; then
  . striferc
  run
fi
if [ -f /root/striferc ]; then
  . /root/striferc
  run
fi
error

