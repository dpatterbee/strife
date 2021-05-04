#!/bin/sh

cd /

litestream restore -if-replica-exists "$DB_PATH"

litestream replicate > litelog 2>&1 &

/app/strife > strifelog