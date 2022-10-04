#!/bin/sh


PID=`ps -eaf | grep "build/alphabill money"  | grep -v grep | awk '{print $2}'`
if [ ! -z "$PID" ]; then
  echo "killing $PID"
  kill $PID
fi
