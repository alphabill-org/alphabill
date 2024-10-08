#!/bin/bash

money_nodes=3
token_nodes=3
evm_nodes=3
orchestration_nodes=3
root_nodes=3
enterprise_token_nodes=0
reset_db_only=false
initial_bill_owner_predicate=null
admin_owner_predicate=830041025820f34a250bf4f2d3a432a43381cecc4ab071224d9ceccb6277b5779b937f59055f
# exit on error
set -e

# print help
usage() {
  echo "Generate 'testab' structure, log configuration and genesis files. Usage: $0 [-h usage] [-m number of money nodes] [-t number of token nodes] [-e number of EVM nodes] [-o number of orchestration nodes] [-r number of root nodes] [-c reset all DB files] [-i initial bill owner predicate] [-k number of enterprise token partition nodes] [-a enterprise token partition admin owner predicate]"
  exit 0
}
# handle arguments
while getopts "chm:t:r:e:o:i:k:a:" o; do
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
  a)
    admin_owner_predicate=${OPTARG}
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
  tokensPDR='{"network_identifier": 3, "system_identifier": 2, "type_id_length": 8, "unit_id_length": 256, "t2timeout": 2500000000, "fee_credit_bill": {"unit_id": "0x000000000000000000000000000000000000000000000000000000000000001201", "owner_predicate":"0x830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"}}'
  echo "$tokensPDR" >testab/tokens-pdr.json
  moneySdrFlags+=" -c testab/tokens-pdr.json"
  generate_partition_node_genesis "tokens" "$token_nodes" "--partition-description=$PWD/testab/tokens-pdr.json"
fi
# Generate EVM nodes genesis files.
if [ "$evm_nodes" -ne 0 ]; then
  evmPDR='{"network_identifier": 3, "system_identifier": 3, "type_id_length": 8, "unit_id_length": 256, "t2timeout": 2500000000, "fee_credit_bill": {"unit_id": "0x000000000000000000000000000000000000000000000000000000000000001301", "owner_predicate": "0x830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"}}'
  echo "$evmPDR" >testab/evm-pdr.json
  moneySdrFlags+=" -c testab/evm-pdr.json"
  generate_partition_node_genesis "evm" "$evm_nodes" "--partition-description=$PWD/testab/evm-pdr.json"
fi
# Generate money nodes genesis files.
if [ "$money_nodes" -ne 0 ]; then
  moneyPDR='{"network_identifier": 3, "system_identifier": 1, "type_id_length": 8, "unit_id_length": 256, "t2timeout": 2500000000, "fee_credit_bill": {"unit_id": "0x000000000000000000000000000000000000000000000000000000000000001101", "owner_predicate": "0x830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"}}'
  echo "$moneyPDR" >testab/money-pdr.json
  moneySdrFlags+=" -c testab/money-pdr.json"
  moneySdrFlags+=" --partition-description=$PWD/testab/money-pdr.json"
  customCliArgs=$moneySdrFlags
  if [ "$initial_bill_owner_predicate" != null ]; then
    customCliArgs+=" --initial-bill-owner-predicate $initial_bill_owner_predicate"
  fi
  generate_partition_node_genesis "money" "$money_nodes" "$customCliArgs"
fi
# Generate orchestration nodes genesis files.
if [ "$orchestration_nodes" -ne 0 ]; then
  orchestrationPDR='{"network_identifier": 3, "system_identifier": 4, "type_id_length": 8, "unit_id_length": 256, "t2timeout": 2500000000}'
  echo "$orchestrationPDR" >testab/orchestration-pdr.json
  generate_partition_node_genesis "orchestration" "$orchestration_nodes" "--partition-description=$PWD/testab/orchestration-pdr.json --owner-predicate 830041025820f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"
fi
# Generate enterprise token partition genesis files
if [ "$enterprise_token_nodes" -ne 0 ]; then
  enterpriseTokensPDR='{"network_identifier": 3, "system_identifier": 5, "type_id_length": 8, "unit_id_length": 256, "t2timeout": 2500000000}'
  echo "$enterpriseTokensPDR" >testab/tokens-pdr-sid-5.json
  generate_partition_node_genesis "tokens-enterprise" "$enterprise_token_nodes" "--partition-description=$PWD/testab/tokens-pdr-sid-5.json --admin-owner-predicate $admin_owner_predicate"
fi

# generate root node genesis files
generate_root_genesis $root_nodes

# generate log configuration for all nodes
generate_log_configuration "testab/*/"
