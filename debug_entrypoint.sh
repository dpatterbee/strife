#!/bin/sh

cd /

if [ ! -d logs ]; then
  echo "No logs mounted."
  exit 1
fi

litestream restore -if-replica-exists "$DB_PATH"

litestream replicate > /logs/litelog 2>&1 &

/dlv --listen=:40000 --headless=true --api-version=2 --accept-multiclient exec /app/strife
