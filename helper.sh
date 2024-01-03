#!/bin/bash
# generate logger configuration file
function generate_log_configuration() {
  # to iterate over all home directories
  for homedir in testab/*/; do
    # generate log file itself
    cat <<EOT >> "$homedir/logger-config.yaml"
# File name to log to. If not set, logs to stdout.
outputPath:
# Controls if goroutine ID is added to log.
showGoroutineID: true
# The default log level for all loggers
# Possible levels: NONE; ERROR; WARNING; INFO; DEBUG; TRACE
defaultLevel: DEBUG
# Output format for log records (text: "parser friendly" plain text;)
format: text
# Sets time format to use for log record timestamp. Uses Go time
# format, ie "2006-01-02T15:04:05.0000Z0700" for more see
# https://pkg.go.dev/time#pkg-constants
# special value "none" can be used to disable logging timestamp;
timeFormat: "2006-01-02T15:04:05.0000Z0700"
# How to format peer ID values (ie node id):
# - none: do not log peer id at all;
# - short: log shortened id (middle part replaced with single *);
# otherwise full peer id is logged.
# This setting is not respected by ECS handler which always logs full ID.
peerIdFormat: short
EOT
  done
  return 0
}
# default to get the set-up up and running configure root nodes as bootnodes
function generate_boot_nodes() {
    local rootPort=$1
    local bootNodes=""
    for keyf in testab/rootchain*/rootchain/keys.json
    do
      id=$(build/alphabill identifier -k $keyf | tail -n1)
      bootNodes="$id@/ip4/127.0.0.1/tcp/$rootPort,$bootNodes"
      ((rootPort=rootPort+1))
    done
   bootNodes=${bootNodes::-1}
   echo "$bootNodes"
}

# generates genesis files
# expects two arguments
# $1 alphabill partition type ('money', 'tokens') or root as string
# $2 nof genesis files to generate
# $3 custom cli args
function generate_partition_node_genesis() {
local cmd=""
local home=""
case $1 in
  money)
    cmd="money-genesis"
    home="testab/money"
    ;;
  tokens)
    cmd="tokens-genesis"
    home="testab/tokens"
    ;;
  evm)
      cmd="evm-genesis"
      home="testab/evm"
      ;;
  *)
    echo "error: unknown partition $1" >&2
    return 1
    ;;
esac
# execute cmd to generate genesis files
for i in $(seq 1 "$2")
do
  # "-g" flags also generates keys
  build/alphabill "$cmd" --home "${home}$i" -g "$3"
done
}

# generate root genesis
# $1 nof root nodes
function generate_root_genesis() {
  # this function assumes a directory structure with indexed home such as
  # testab/money1/money, testab/money2/money, ...,
  # it scans all partition node genesis files from the directories and uses them to create root genesis
  # build partition node genesis files argument list '-p' for root genesis
  local node_genesis_files=""
  for file in testab/money*/money/node-genesis.json testab/tokens*/tokens/node-genesis.json testab/evm*/evm/node-genesis.json
  do
    if [[ ! -f $file ]]; then
      continue
    fi
    node_genesis_files="$node_genesis_files -p $file"
  done
  # generate individual root node genesis files
  for i in $(seq 1 "$1")
  do
    build/alphabill root-genesis new --home testab/rootchain"$i" -g --block-rate=400 --consensus-timeout=2500 --total-nodes="$1" $node_genesis_files
  done
  # if only one root node, then we are done
  if [ $1 == 1 ]; then
    return 0
  fi
  # else combine to generate common root genesis
  root_genesis_files=""
  for file in testab/rootchain*/rootchain/root-genesis.json
  do
    root_genesis_files="$root_genesis_files --root-genesis=$file"
  done
  # merge root genesis files
  for i in $(seq 1 "$1")
  do
  build/alphabill root-genesis combine --home testab/rootchain"$i" $root_genesis_files
  done
}

function start_root_nodes() {
  local port=26662
  # create a bootnodes
  local bootNodes=""
  bootNodes=$(generate_boot_nodes "$port")
  i=1
  for genesisFile in testab/rootchain*/rootchain/root-genesis.json
  do
    if [[ ! -f $genesisFile ]]; then
      echo "Root genesis files do not exist, generate setup!" 1>&2
      exit 1
    fi
    build/alphabill root --home testab/rootchain$i --address="/ip4/127.0.0.1/tcp/$port" --bootnodes="$bootNodes" >> testab/rootchain$i/rootchain/rootchain.log 2>&1 &
    ((port=port+1))
    ((i=i+1))
  done
  echo "started $(($i-1)) root nodes"
}

function start_partition_nodes() {
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
      genesis_file="testab/rootchain1/rootchain/partition-genesis-1.json"
      aPort=26666
      grpcPort=26766
      restPort=26866
      ;;
    tokens)
      home="testab/tokens"
      key_files="testab/tokens*/tokens/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-2.json"
      aPort=28666
      grpcPort=28766
      restPort=28866
      ;;
    evm)
      home="testab/evm"
      key_files="testab/evm*/evm/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-3.json"
      aPort=29666
      grpcPort=29766
      restPort=29866
      ;;
    *)
      echo "error: unknown partition $1" >&2
      return 1
      ;;
  esac
  # create a bootnodes
  local bootNodes=""
  bootNodes=$(generate_boot_nodes "26662")
  # Start nodes
  i=1
  for keyf in $key_files
  do
    build/alphabill "$1" \
        --home ${home}$i \
        --db ${home}$i/"$1"/blocks.db \
        --tx-db ${home}$i/"$1"/tx.db \
        --key-file $keyf \
        --genesis $genesis_file \
        --state "$(dirname $keyf)/node-genesis-state.cbor" \
        --address "/ip4/127.0.0.1/tcp/$aPort" \
        --bootnodes="$bootNodes" \
        --server-address "localhost:$grpcPort" \
        --rest-server-address "localhost:$restPort" \
        >> ${home}$i/"$1"/"$1".log  2>&1 &
    ((i=i+1))
    ((aPort=aPort+1))
    ((grpcPort=grpcPort+1))
    ((restPort=restPort+1))
  done
    echo "started $(($i-1)) $1 nodes"
}
