#!/bin/bash

set -e

enterpriseTokensNodes=3
enterpriseTokensSystemID=5
adminOwnerPredicate=830041025820f34a250bf4f2d3a432a43381cecc4ab071224d9ceccb6277b5779b937f59055f

# print help
usage() {
  echo "Generate genesis for a new enterprise tokens partition and a new rootchain configuration."
  echo "Usage: $0 [-h usage] [-k number of nodes in the enterprise tokens partition] [-s system ID of the enterprise tokens partition] [-a admin owner predicate of the enterprise tokens partition] [-n root round number for the new configuration to take effect]"
  exit 0
}

# handle arguments
while getopts "hk:s:a:n:r" o; do
  case "${o}" in
    k)
      enterpriseTokensNodes=${OPTARG}
      ;;
    s)
      enterpriseTokensSystemID=${OPTARG}
      ;;
    a)
      adminOwnerPredicate=${OPTARG}
      ;;
    n)
      newConfRound=${OPTARG}
      ;;
    h | *) # help.
      usage
      ;;
  esac
done

source ./helper.sh

if [ -d testab/enterprise-tokens-$enterpriseTokensSystemID-1 ]; then
  echo "enterprise tokens partition with system ID $enterpriseTokensSystemID already exists"
  exit 1
fi

enterpriseTokensPDRFile=testab/enterprise-tokens-$enterpriseTokensSystemID-pdr.json
cat > $enterpriseTokensPDRFile <<EOF
{
  "network_identifier": 3,
  "system_identifier": $enterpriseTokensSystemID,
  "type_id_length": 8,
  "unit_id_length": 256,
  "t2timeout": 2500000000
}
EOF

for i in $(seq 1 "$enterpriseTokensNodes"); do
  enterpriseTokensNodeHome=testab/enterprise-tokens-$enterpriseTokensSystemID-$i
  echo "Generating enterprise tokens partition node genesis in $enterpriseTokensNodeHome"
  build/alphabill tokens-genesis \
                  --home $enterpriseTokensNodeHome \
                  --gen-keys \
                  --partition-description=$enterpriseTokensPDRFile \
                  --admin-owner-predicate $adminOwnerPredicate
done

echo "Generating log configurations"
generate_log_configuration "testab/enterprise-tokens-$enterpriseTokensSystemID-*/"

# Find all existing node genesis files
partitionNodeGenesisFiles=""
for file in testab/*/*/node-genesis.json; do
  partitionNodeGenesisFiles="$partitionNodeGenesisFiles -p $file"
done

newConfDir=rootchain-$(date +%s)
newConfPaths=""
totalRootNodes=`find testab/ -maxdepth 1 -type d -name 'rootchain[0-9]*' | wc -l`
for rootNodeHome in testab/rootchain[0-9]*; do
  echo "Signing new rootchain configuration with $rootNodeHome"
  build/alphabill root-genesis new \
                  --home $rootNodeHome \
                  --key-file $rootNodeHome/rootchain/keys.json \
                  --output-dir $rootNodeHome/$newConfDir \
                  --block-rate=400 \
                  --consensus-timeout=2500 \
                  --total-nodes="$totalRootNodes" \
                  $partitionNodeGenesisFiles
  newConfPaths="$newConfPaths --root-genesis=$rootNodeHome/$newConfDir/root-genesis.json"
done

echo "Combining new rootchain configuration in testab/$newConfDir"
build/alphabill root-genesis combine \
                --output testab/$newConfDir \
                $newConfPaths

if [ -z ${newConfRound+x} ]; then
  echo "Root round not set - not uploading new configuration"
  exit 0
fi

for i in $(seq $rootRpcPortStart $((rootRpcPortStart+totalRootNodes-1))); do
  echo "Adding new rootchain configuration to localhost:$i"
  curl -H "Content-Type: application/json" -X PUT http://localhost:$i/api/v1/configurations?start-round=$newConfRound -d @testab/$newConfDir/root-genesis.json
done

start_partition_nodes enterprise-tokens $enterpriseTokensSystemID $newConfDir
