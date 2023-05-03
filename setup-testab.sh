#!/bin/bash

money_nodes=3
vd_nodes=3
token_nodes=3
root_nodes=1
reset_db_only=false
# exit on error
set -e

# print help
usage() {
  echo "Generate 'testab' structure, log configuration and genesis files. Usage: $0 [-h usage] [-m number of money nodes] [-t number of token nodes] [-d number of vd nodes] [-c reset all DB files]"
  exit 0
}

# handle arguments
# NB! add check to make parameter is numeric
while getopts "chd:m:t:" o; do
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
mkdir testab

# get common functions
source helper.sh

# Generate all genesis files
echo "generating genesis files"

moneySdrFlags=""

# Generate vd nodes genesis files.
if [ "$vd_nodes" -ne 0 ]; then
  vdSdr='{"systemId": "0x00000001", "unitId": "0x0000000000000000000000000000000000000000000000000000000000000003", "ownerPubKey": "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"}'
  echo "$vdSdr" >testab/vd-sdr.json
  moneySdrFlags+=" -c testab/vd-sdr.json"
  generate_partition_node_genesis "vd" "$vd_nodes"
fi
# Generate token nodes genesis files.
if [ "$token_nodes" -ne 0 ]; then
  tokensSdr='{"systemId": "0x00000002", "unitId": "0x0000000000000000000000000000000000000000000000000000000000000004", "ownerPubKey": "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"}'
  echo "$tokensSdr" >testab/tokens-sdr.json
  moneySdrFlags+=" -c testab/tokens-sdr.json"
  generate_partition_node_genesis "token" "$token_nodes"
fi
# Generate money nodes genesis files.
if [ "$money_nodes" -ne 0 ]; then
  moneySdr='{"systemId": "0x00000000", "unitId": "0x0000000000000000000000000000000000000000000000000000000000000002", "ownerPubKey": "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"}'
  echo "$moneySdr" >testab/money-sdr.json
  moneySdrFlags+=" -c testab/money-sdr.json"
  generate_partition_node_genesis "money" "$money_nodes" "$moneySdrFlags"
fi

# generate root node genesis files
generate_root_genesis $root_nodes

# generate log configuration for all nodes
generate_log_configuration
