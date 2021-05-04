#!/bin/sh

cd /

litestream restore -if-replica-exists "$DB_PATH"

litestream replicate &

eval "/app/strife"