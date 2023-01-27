#!/bin/bash

# generate local addresses by incrementing port number
# expects two arguments
# $1 - path to key files
# $2 - port number
function generate_peer_addresses () {
local port=$2
local addresses=
for keyfile in $1
do
  id=$(build/alphabill identifier -k "$keyfile" | tail -n1)
  addresses="$addresses,$id=/ip4/127.0.0.1/tcp/$port";
  ((port=port+1))
done
addresses="${addresses:1}"
echo "$addresses"
}

# generates genesis files
# expects two arguments
# $1 alphabill partition type ('money', 'vd', 'toke') or root as string
# $2 nof genesis files to generate
function generate_partition_node_genesis () {
local cmd=""
local home=""
case $1 in
  money)
    cmd="money-genesis"
    home="testab/money"
    ;;
  vd)
    cmd="vd-genesis"
    home="testab/vd"
    ;;
  token)
    cmd="tokens-genesis"
    home="testab/tokens"
    ;;
  *)
    echo "error: unknown partition $1" >&2
    return 1
    ;;
esac
# execute cmd to generate genesis files
for i in $(seq 1 $2)
do
  # "-g" flags also generates keys
  build/alphabill "$cmd" --home "${home}$i" -g
done
}

# generate root genesis
# $1 nof root nodes
function generate_root_genesis () {
  # this function assumes a directory structure with indexed home such as
  # testab/money1/money, testab/money2/money, ..., testab/vd1/vd, testab/vd2/vd,...
  # it scans all partition node genesis files from the directories and uses them to create root genesis
  # build partition node genesis files argument list '-p' for root genesis
  local node_genesis_files=""
  for file in testab/money*/money/node-genesis.json testab/vd*/vd/node-genesis.json testab/tokens*/tokens/node-genesis.json
  do
    node_genesis_files="$node_genesis_files -p $file"
  done
  # generate individual root node genesis files
  for i in $(seq 1 $1)
  do
    build/alphabill root-genesis new --home testab/rootchain$i -g --total-nodes=$1 $node_genesis_files
  done
  # if only one root node, then we are done
  if [ $1 == 1 ]; then
    return 0
  fi
  # else combine to generate common root genesis
  root_genesis_files=""
  for file in testab/rootchain*/rootchain/root-genesis.json
  do
    root_genesis_files="$root_genesis_files -r $file"
  done
  # merge root genesis files
  for i in $(seq 1 $1)
  do
  build/alphabill root-genesis combine --home testab/rootchain$i $root_genesis_files
  done
}

function start_root_nodes () {
  local rPort=29666
  local pPort=26662
  root_node_addresses=$(generate_peer_addresses "testab/rootchain*/rootchain/keys.json" $rPort)
  i=1
  for fgen in testab/rootchain*/rootchain/root-genesis.json
  do
  build/alphabill root --home testab/rootchain$i -f testab/rootchain$i/rootchain/rootchain.db -k testab/rootchain$i/rootchain/keys.json --partition-listener="/ip4/127.0.0.1/tcp/$pPort" -g $fgen --root-listener="/ip4/127.0.0.1/tcp/$rPort" -p $root_node_addresses  >> testab/rootchain$i/rootchain/rootchain.log &
    ((rPort=rPort+1))
    ((pPort=pPort+1))
    ((i=i+1))
  done
  echo "started $(($i-1)) root nodes, addresses: $root_node_addresses"
}

function start_partition_nodes () {
local home=""
local key_files=""
local genesis_file=""
local aPort=0
local grpcPort=0
local restPort=0
  case $1 in
    money)
      home="testab/money"
      key_files="testab/money*/money/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-0.json"
      aPort=26666
      grpcPort=26766
      restPort=26866
      ;;
    vd)
      home="testab/vd"
      key_files="testab/vd*/vd/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-1.json"
      aPort=27666
      grpcPort=27766
      restPort=27866
      ;;
    tokens)
      home="testab/tokens"
      key_files="testab/tokens*/tokens/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-2.json"
      aPort=28666
      grpcPort=28766
      restPort=28866
      ;;
    *)
      echo "error: unknown partition $1" >&2
      return 1
      ;;
  esac
  # generate node addresses
  nodeAddresses=$(generate_peer_addresses "$key_files" $aPort)
  index=1
  for keyf in $key_files
  do
    build/alphabill "$1" --home ${home}$index -f ${home}$index/$1/blocks.db -k $keyf -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$aPort" --server-address ":$grpcPort" --rest-server-address "localhost:$restPort" -g $genesis_file -p $nodeAddresses >> "${home}$index/$1.log" &
    ((index=index+1))
    ((aPort=aPort+1))
    ((grpcPort=grpcPort+1))
    ((restPort=restPort+1))
  done
}

