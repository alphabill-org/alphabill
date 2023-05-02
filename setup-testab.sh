#!/bin/bash

money_nodes=3
vd_nodes=3
token_nodes=3
root_nodes=3
reset_db_only=false
# exit on error
set -e

# print help
usage() { echo "Generate 'testab' structure, log configuration and genesis files. Usage: $0 [-h usage] [-m number of money nodes] [-t number of token nodes] [-d number of vd nodes]  [-r number of root nodes] [-c reset all DB files]"; exit 0; }
# handle arguments
# NB! add check to make parameter is numeric
while getopts "chd:m:t:r:" o; do
  case "${o}" in
  c)
    reset_db_only=true
    ;;
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

if [ "$reset_db_only" == true ]; then
  echo "deleting all *.db files"
  find testab/*/* -name *.db -type f -delete
  exit 0
fi

#make clean will remove "testab" directory with all of the content
echo "clearing 'testab' directory and building alphabill"
make clean build

# get common functions
source helper.sh

mkdir testab
# Generate all genesis files
echo "generating genesis files"
# Generate money node genesis files.
if [ "$money_nodes" -ne 0 ]; then
  generate_partition_node_genesis "money" "$money_nodes"
fi
# Generate money node genesis files.
if [ "$vd_nodes" -ne 0 ]; then
  generate_partition_node_genesis "vd" "$vd_nodes"
fi
# Generate money node genesis files.
if [ "$token_nodes" -ne 0 ]; then
  generate_partition_node_genesis "token" "$token_nodes"
fi
# generate root node genesis files
generate_root_genesis $root_nodes
# generate log configuration for all nodes
generate_log_configuration
