#!/bin/bash
# exit on error
set -e

function stop() {
  local program=""
  case $1 in
    all)
      program="build/alphabill"
      ;;
    shard)
      program="build/alphabill shard-node"
      ;;
    root)
      program="build/alphabill root-node"
      ;;
    *)
      echo "error: unknown argument $1" >&2
      return 1
    ;;
  esac

  PID=$(ps -eaf | grep "$program"  | grep -v grep | awk '{print $2}')
  if [ -n "$PID" ]; then
    echo "killing $PID"
    kill $PID
    return 0
  fi
  echo "program not running"
}

usage() { echo "Usage: $0 [-h usage] [-a stop all] [-r stop root] [-s stop shards]"; exit 0; }

# stop requires an argument either -a for stop all or -s to stop a all shard nodes
[ $# -eq 0 ] && usage

# handle arguments
while getopts "hars" o; do
  case "${o}" in
  a) #kill all
    stop "all"
    ;;
  r)
    stop "root"
    ;;
  s)
    stop "shard"
    ;;
  h | *) # help.
    usage && exit 0
    ;;
  esac
done
