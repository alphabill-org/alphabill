#!/bin/bash

# exit on error
set -e

source helper.sh

usage() {
  echo "Usage: $0 [-h usage] [-m number of non-validator money nodes] [-t number of non-validator tokens nodes] [-k number of non-validator tokens-enterprise nodes] [-i initial bill owner (money node only)]"
  exit 0
}

[ $# -eq 0 ] && usage

partitionType=""
extraFlags=""

while getopts "hm:i:t:k:" o; do
  case "${o}" in
    m)
      partitionType=money
      nodeCount="${OPTARG}"
      ;;
    t)
      partitionType=tokens
      nodeCount="${OPTARG}"
      ;;
    k)
      partitionType=tokens-enterprise
      nodeCount="${OPTARG}"
      ;;
    i)
      extraFlags+=" --initial-bill-owner-predicate ${OPTARG}"
      ;;
    h | *) # help.
      usage && exit 0
      ;;
  esac
done

if [ ! -z "${partitionType}" ]; then
  start_non_validator_partition_nodes $partitionType $nodeCount "$extraFlags"
else
  usage && exit 0
fi
