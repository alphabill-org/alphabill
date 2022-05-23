#!/bin/bash
rm -rf testab
nodeGenesisFiles=()

function generateNodeGenesisFiles {
  local type=${1:?}       # partition type
  local homeDir=${2:?}    # nodes home dir
  local nodesCount=${3:?} # nr of nodes
  local params=${4}       # extra parameters to add to the genesis command

  if [ "$type" != "vd" ] && [ "$type" != "money" ]; then
    echo "Invalid partition type: $type"
    exit
  fi
  local files=()
  for ((i = 1; i <= nodesCount; i++)); do
    echo "generating genesis for node $type$i. node home dir: $homeDir/$type$i"
    build/alphabill "$type"-genesis --home "$homeDir/$type$i" "$params" -f
    nodeGenesisFiles+="-p $homeDir/$type$i/$type/node-genesis.json "
  done

  return
}

# generatePartitionGenesisFails generates partition genesis files. first argument is an array of node genesis files.
function generatePartitionGenesisFails {
  local -n nodeGenesisFiles=${1:?}
  local -n rootPartitionHome=${2:?}

  return
}

# money partition
generateNodeGenesisFiles "money" "testab" 1
# vd partition
generateNodeGenesisFiles "vd" "testab" 2
echo "${nodeGenesisFiles[@]}"

# generate rootchain and partition genesis files
mkdir testab/rootchain
build/alphabill root-genesis "--home testab $nodeGenesisFiles -k testab/rootchain/keys.json -f"


generateNodeGenesisFiles "test" "testab" 1

startRootChain() {

  return
}

generatePartitionGenesisFails "d"
