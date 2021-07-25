#!/bin/sh

cd /

if [ ! -d logs ]; then
  echo "No logs mounted."
  exit 1
fi

logdir="/logs/$(date +%Y%m%d%H%M%S)"
mkdir $logdir

echo $logdir > /app/logdir

litestream restore -if-replica-exists "$DB_PATH"

litestream replicate > "$logdir/litelog" 2>&1 &

/app/strife > "$logdir/strifelog" 2>&1 &

ln -f "$logdir/litelog" /logs/litelog
ln -f "$logdir/strifelog" /logs/strifelog

while true; do
  sleep 86400
done
