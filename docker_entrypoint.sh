#!/bin/sh

cd /

if [ ! -d logs ]; then
  echo "No logs mounted."
  exit 1
fi

litestream restore -if-replica-exists "$DB_PATH"

litestream replicate > /logs/litelog 2>&1 &

/app/strife > /logs/strifelog 2>&1