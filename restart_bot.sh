#!/bin/sh

if pkill -15 strife
then
  while pkill -0 strife
  do
    usleep 10000
  done
fi
logdir=$(cat /app/logdir)


/app/strife >> "$logdir/strifelog" 2>&1 &
