#!/bin/bash

# exit on error
set -e

source helper.sh

usage() { echo "Usage: $0 [-h usage] [-m number of non-validator money nodes] [-t number of non-validator token nodes]"; exit 0; }

[ $# -eq 0 ] && usage

while getopts "hm:t:" o; do
  case "${o}" in
  m)
    start_non_validator_partition_nodes money "${OPTARG}"
    ;;
  t)
    start_non_validator_partition_nodes tokens "${OPTARG}"
    ;;
  h | *) # help.
    usage && exit 0
    ;;
  esac
done
