#!/bin/sh

PID=`ps -eaf | grep "build/alphabill tokens"  | grep -v grep | awk '{print $2}'`
if [ ! -z  "$PID" ]; then
  echo "killing $PID"
  kill $PID
fi
