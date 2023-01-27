#!/bin/bash

clean_start=true
build=true
# exit on error
set -e

# print help
usage() { echo "Usage: $0 [-h usage] [-b skip build alphabill] [-r resume instead of regenerating everything]"; exit 0; }

# handle arguments
while getopts "hbr" o; do
  case "${o}" in
  b)
    build=false
    ;;
  r)
    clean_start=false
    ;;
  h | *) # help.
    usage
    ;;
  esac
done

# build binary
if [ "$build" == true ]; then
  make build
fi

# get common functions
source helper.sh

if [ $clean_start == true ]; then
  echo "clearing testab directory"
  rm -rf testab || true
  mkdir testab
  # Generate all genesis files
  echo "generating genesis files"
  # Generate money node genesis files.
  generate_partition_node_genesis "money" 3
  # Generate money node genesis files.
  generate_partition_node_genesis "vd" 3
  # Generate money node genesis files.
  generate_partition_node_genesis "token" 3
  # generate root node genesis files
  generate_root_genesis 3
fi

# start root
echo "starting root nodes"
start_root_nodes

# start money partition
echo "starting money partition"
start_partition_nodes "money"

# start vd partition
echo "starting vd partition"
start_partition_nodes "vd"

# start token partition
echo "starting tokens partition"
start_partition_nodes "tokens"