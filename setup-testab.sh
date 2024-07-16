#!/bin/bash

money_nodes=3
token_nodes=3
evm_nodes=3
orchestration_nodes=3
root_nodes=3
enterprise_token_nodes=0
reset_db_only=false
initial_bill_owner_predicate=null
# exit on error
set -e

# print help
usage() {
  echo "Generate 'testab' structure, log configuration and genesis files. Usage: $0 [-h usage] [-m number of money nodes] [-t number of token nodes] [-e number of EVM nodes] [-o number of orchestration nodes] [-r number of root nodes] [-c reset all DB files] [-i initial bill owner predicate] [-k number of enterprise token partition nodes]"
  exit 0
}
# handle arguments
while getopts "chm:t:r:e:o:i:k:" o; do
  case "${o}" in
  c)
    reset_db_only=true
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
  e)
    evm_nodes=${OPTARG}
    ;;
  o)
    orchestration_nodes=${OPTARG}
    ;;
  i)
    initial_bill_owner_predicate=${OPTARG}
    ;;
  k)
    enterprise_token_nodes=${OPTARG}
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

# make clean will remove "testab" directory with all of the content
echo "clearing 'testab' directory and building Alphabill"
make clean build
mkdir testab

# get common functions
source helper.sh

moneySdrFlags=""

# Generate token nodes genesis files.
if [ "$token_nodes" -ne 0 ]; then
  tokensSdr='{"system_identifier": 2, "t2timeout": 2500, "fee_credit_bill": {"unit_id": "0x000000000000000000000000000000000000000000000000000000000000001200", "owner_predicate":"0x830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"}}'
  echo "$tokensSdr" >testab/tokens-sdr.json
  moneySdrFlags+=" -c testab/tokens-sdr.json"
  generate_partition_node_genesis "tokens" "$token_nodes"
fi
# Generate EVM nodes genesis files.
if [ "$evm_nodes" -ne 0 ]; then
  evmSdr='{"system_identifier": 3, "t2timeout": 2500, "fee_credit_bill": {"unit_id": "0x000000000000000000000000000000000000000000000000000000000000001300", "owner_predicate": "0x830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"}}'
  echo "$evmSdr" >testab/evm-sdr.json
  moneySdrFlags+=" -c testab/evm-sdr.json"
  generate_partition_node_genesis "evm" "$evm_nodes"
fi
# Generate money nodes genesis files.
if [ "$money_nodes" -ne 0 ]; then
  moneySdr='{"system_identifier": 1, "t2timeout": 2500, "fee_credit_bill": {"unit_id": "0x000000000000000000000000000000000000000000000000000000000000001100", "owner_predicate": "0x830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"}}'
  echo "$moneySdr" >testab/money-sdr.json
  moneySdrFlags+=" -c testab/money-sdr.json"
  customCliArgs=$moneySdrFlags
  if [ "$initial_bill_owner_predicate" != null ]; then
    customCliArgs+=" --initial-bill-owner-predicate $initial_bill_owner_predicate"
  fi
  generate_partition_node_genesis "money" "$money_nodes" "$customCliArgs"
fi
# Generate orchestration nodes genesis files.
if [ "$orchestration_nodes" -ne 0 ]; then
  generate_partition_node_genesis "orchestration" "$orchestration_nodes" " --owner-predicate 830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"
fi
# Generate enterprise token partition genesis files
if [ "$enterprise_token_nodes" -ne 0 ]; then
  enterpriseTokensSdr='{"system_identifier": 5, "t2timeout": 2500}'
  echo "$enterpriseTokensSdr" >testab/tokens-sdr-sid-5.json
  generate_partition_node_genesis "tokens-enterprise" "$enterprise_token_nodes" "--system-identifier 5 --admin-key 028834d671a927762584091403259bff4bc972c917c7de8eb558118fabf9733384"
fi

# generate root node genesis files
generate_root_genesis $root_nodes

# generate log configuration for all nodes
generate_log_configuration "testab/*/"
