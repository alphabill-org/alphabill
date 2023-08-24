#!/bin/bash

# exit on error
set -e

source helper.sh

usage() { echo "Usage: $0 [-h usage] [-r start root] [-p start partition: money, vd, tokens, evm] [-b start backend: money, tokens, evm]"; exit 0; }

# stop requires an argument either -a for stop all or -p to stop a specific partition
[ $# -eq 0 ] && usage

# handle arguments
while getopts "hb:rp:" o; do
  case "${o}" in
  r)
    echo "starting root nodes..." && start_root_nodes
    ;;
  p)
    echo "starting ${OPTARG} nodes..." && start_partition_nodes "${OPTARG}"
    ;;
  b)
    echo "starting ${OPTARG} backend" && start_backend "${OPTARG}"
    ;;
  h | *) # help.
    usage && exit 0
    ;;
  esac
done
