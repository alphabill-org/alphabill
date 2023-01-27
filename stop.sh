#!/bin/bash
# exit on error
set -e

function stop() {
  local program=""
  case $1 in
    all)
      program="build/alphabill"
      ;;
    money)
      program="build/alphabill money"
      ;;
    vd)
      program="build/alphabill vd"
      ;;
    tokens)
      program="build/alphabill tokens"
      ;;
    root)
      program="build/alphabill root"
      ;;
    *)
      echo "error: unknown argument $1" >&2
      return 1
    ;;
  esac

  PID=`ps -eaf | grep "$program"  | grep -v grep | awk '{print $2}'`
  if [ ! -z "$PID" ]; then
    echo "killing $PID"
    kill $PID
    return 0
  fi
  echo "program not running"
}

usage() { echo "Usage: $0 [-h usage] [-a stop all] [-r stop root] [-p stop partition: money, vd, tokens]"; exit 0; }

# stop requires an argument either -a for stop all or -p to stop a specific partition
[ $# -eq 0 ] && usage

# handle arguments
while getopts "hap:" o; do
  case "${o}" in
  a) #kill all
    stop "all"
    ;;
  r)
    stop "root"
    ;;
  p)
    stop ${OPTARG}
    ;;
  h | *) # help.
    usage && exit 0
    ;;
  esac
done
