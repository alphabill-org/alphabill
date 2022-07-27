#!/bin/sh


PID=`ps -eaf | grep "build/alphabill money"  | grep -v grep | awk '{print $2}'`
if [[ "" !=  "$PID" ]]; then
  echo "killing ${PID%% *}"
  read FIRST __ <<< "$PID"
  echo "f $FIRST"
#  kill $PID
fi