#!/bin/sh


PID=`ps -eaf | grep "build/alphabill vd"  | grep -v grep | awk '{print $2}'`
if [[ "" !=  "$PID" ]]; then
  echo "killing $PID"
  kill $PID
fi