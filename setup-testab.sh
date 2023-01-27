#!/bin/bash

money_nodes=3
vd_nodes=3
token_nodes=3
root_nodes=3
# exit on error
set -e

# print help
usage() { echo "Usage: $0 [-h usage] [-m number of money nodes] [-t number of token nodes] [-d number of vd nodes]  [-r number of root nodes]"; exit 0; }

# handle arguments
# NB! add check to make parameter is numeric
while getopts "hd:m:t:r:" o; do
  case "${o}" in
  d)
    vd_nodes=${OPTARG}
    ;;
  m)
    money_nodes=${OPTARG}
    ;;
  t)
    token_nodes=${OPTARG}
    ;;
  r)
    root_nodes=${OPTARG}
    ;;
  h | *) # help.
    usage
    ;;
  esac
done

echo "clearing testab directory and building alphabill"
make clean build

# get common functions
source helper.sh

mkdir testab
# Generate all genesis files
echo "generating genesis files"
# Generate money node genesis files.
generate_partition_node_genesis "money" $money_nodes
# Generate money node genesis files.
generate_partition_node_genesis "vd" $vd_nodes
# Generate money node genesis files.
generate_partition_node_genesis "token" $token_nodes
# generate root node genesis files
generate_root_genesis $root_nodes