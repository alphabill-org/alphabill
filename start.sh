#!/bin/bash

# exit on error
set -e

source helper.sh

usage() {
  echo "Usage: $0 [-h usage] [-r start root] [-p start partition: money, tokens, orchestration, tokens-enterprise]"
  exit 0
}

# start requires an argument
[ $# -eq 0 ] && usage

# handle arguments
while getopts "hrp:" o; do
  case "${o}" in
  r)
    echo -n "starting root nodes..." && start_root_nodes
    ;;
  p)
    echo "starting ${OPTARG} nodes..." && start_shard_nodes "${OPTARG}"
    ;;
  h | *) # help.
    usage
    ;;
  esac
done
