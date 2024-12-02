#!/bin/bash

rootPortStart=26662

# generate logger configuration file
function generate_log_configuration() {
  # to iterate over all home directories
  for homedir in $1; do
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
# generate bootstrap parameter from key file and port
function generate_boot_node() {
    local keyf=$1
    local rootPort=$2
    if [[ ! -f $keyf ]]; then
      echo "bootstrap parameter generation error: file missing $keyf"
      exit 1
    fi
    id=$(build/alphabill identifier -k $keyf | tail -n1)
    echo "$id@/ip4/127.0.0.1/tcp/$rootPort"
}

# generates genesis files
# first two arguments are mandatory, third is optional
# $1 Alphabill partition type ('money', 'tokens', 'evm', 'orchestration', 'tokens-enterprise') as string
# $2 nof genesis files to generate
# $3 custom CLI args
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
  orchestration)
    cmd="orchestration-genesis"
    home="testab/orchestration"
    ;;
  tokens-enterprise)
    cmd="tokens-genesis"
    home="testab/tokens_enterprise"
    ;;
  *)
    echo "error: unknown partition $1" >&2
    return 1
    ;;
esac
# execute cmd to generate genesis files
echo "generating $2 genesis files for $1 partition"
for i in $(seq 1 "$2")
do
  # "-g" flag generates keys
  build/alphabill "$cmd" --home "${home}$i" -g $3
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
  for file in testab/money*/money/node-genesis.json testab/tokens*/tokens/node-genesis.json testab/evm*/evm/node-genesis.json testab/orchestration*/orchestration/node-genesis.json
  do
    if [[ ! -f $file ]]; then
      continue
    fi
    node_genesis_files="$node_genesis_files -p $file"
  done
  # generate individual root node genesis files
  echo "generating $1 genesis files for root node"
  for i in $(seq 1 "$1")
  do
    build/alphabill root-genesis new --home testab/rootchain"$i" -g --block-rate=400 --consensus-timeout=2500 --total-nodes="$1" $node_genesis_files
  done
  # create --root-genesis argument list
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
  # generate trust base file
  build/alphabill root-genesis gen-trust-base --home testab $root_genesis_files
  # sign trust base file by each root node
  for i in $(seq 1 "$1")
  do
  build/alphabill root-genesis sign-trust-base --home testab -k testab/rootchain"$i"/rootchain/keys.json
  done
}

function start_root_nodes() {
  # use root node 1 as bootstrap node
  local bootNode=""
  local port=$rootPortStart
  local rpcPort=25866
  bootNode=$(generate_boot_node testab/rootchain1/rootchain/keys.json "$rootPortStart")
  i=1
  for genesisFile in testab/rootchain*/rootchain/root-genesis.json
  do
    if [[ ! -f $genesisFile ]]; then
      echo "Root genesis files do not exist, generate setup!" 1>&2
      exit 1
    fi
    if [[ $i -eq 1 ]]; then
          build/alphabill root --home testab/rootchain$i --address="/ip4/127.0.0.1/tcp/$port" --trust-base-file testab/root-trust-base.json --rpc-server-address "localhost:$rpcPort" --metrics prometheus >> testab/rootchain$i/rootchain/rootchain.log 2>&1 &
          # give bootstrap node a head start
          sleep 0.200
    else
          build/alphabill root --home testab/rootchain$i --address="/ip4/127.0.0.1/tcp/$port" --trust-base-file testab/root-trust-base.json --bootnodes="$bootNode" --rpc-server-address "localhost:$rpcPort" --metrics prometheus >> testab/rootchain$i/rootchain/rootchain.log 2>&1 &
    fi
    ((port=port+1))
    ((i=i+1))
    ((rpcPort=rpcPort+1))    
  done
  echo "started $(($i-1)) root nodes"
}

# starts node
# $1 partition type i.e. one of [money/tokens/evm/orchestration/tokens-enterprise]
function start_partition_nodes() {
local cmd=""
local home=""
local key_files=""
local genesis_file=""
local trust_base_file="testab/root-trust-base.json"
local aPort=0
local rpcPort=0
  case $1 in
    money)
      cmd="money"
      home="testab/money"
      key_files="testab/money[0-9]*/money/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-1.json"
      aPort=26666
      rpcPort=26866
      ;;
    tokens)
      cmd="tokens"
      home="testab/tokens"
      key_files="testab/tokens[0-9]*/tokens/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-2.json"
      aPort=28666
      rpcPort=28866
      ;;
    evm)
      cmd="evm"
      home="testab/evm"
      key_files="testab/evm[0-9]*/evm/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-3.json"
      aPort=29666
      rpcPort=29866
      ;;
    orchestration)
      cmd="orchestration"
      home="testab/orchestration"
      key_files="testab/orchestration*/orchestration/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-4.json"
      aPort=30666
      rpcPort=30866
      ;;
    tokens-enterprise)
      cmd="tokens"
      home="testab/tokens_enterprise"
      key_files="testab/tokens_enterprise[0-9]*/tokens/keys.json"
      genesis_file="testab/rootchain1/rootchain/partition-genesis-5.json"
      aPort=31666
      rpcPort=31866
      ;;
    *)
      echo "error: unknown partition $1" >&2
      return 1
      ;;
  esac
  # create bootnodes
  local bootNodes=""
  bootNodes=$(generate_boot_node testab/rootchain1/rootchain/keys.json "$rootPortStart")
  # Start nodes
  i=1
  for keyf in $key_files
  do
    build/alphabill "$cmd" \
        --home ${home}$i \
        --db ${home}$i/"$cmd"/blocks.db \
        --shard-db ${home}$i/"$cmd"/shard.db \
        --tx-db ${home}$i/"$cmd"/tx.db \
        --key-file $keyf \
        --genesis $genesis_file \
        --trust-base-file $trust_base_file \
        --state "$(dirname $keyf)/node-genesis-state.cbor" \
        --address "/ip4/127.0.0.1/tcp/$aPort" \
        --bootnodes="$bootNodes" \
        --rpc-server-address "localhost:$rpcPort" \
        >> "$(dirname $keyf)/$cmd".log  2>&1 &
    ((i=i+1))
    ((aPort=aPort+1))
    ((rpcPort=rpcPort+1))
  done
    echo "started $(($i-1)) $1 nodes"
}

function start_non_validator_partition_nodes() {
  partition=$1
  count=$2
  home="testab/$partition-non-validator"
  trust_base_file="testab/root-trust-base.json"

  echo "starting $count non-validator $partition nodes"

  # Set up partition specific variables
  case $partition in
      money)
          partitionGenesis="testab/rootchain1/rootchain/partition-genesis-1.json"
          p2pPort=36666
          rpcPort=36866
          sdrFlags="-c testab/money-pdr.json"
          sdrFlags+=" --partition-description=$PWD/testab/money-pdr.json"

          [ -f testab/evm-pdr.json ] && sdrFlags+=" -c testab/evm-pdr.json"
          [ -f testab/tokens-pdr.json ] && sdrFlags+=" -c testab/tokens-pdr.json"
          ;;
      tokens)
          partitionGenesis="testab/rootchain1/rootchain/partition-genesis-2.json"
          p2pPort=38666
          rpcPort=38866
          sdrFlags="--partition-description=$PWD/testab/tokens-pdr.json"
          ;;
  esac

  # create bootnodes
  local bootNodes=$(generate_boot_node testab/rootchain1/rootchain/keys.json "$rootPortStart")

  # Start non-validator partition nodes
  for i in $(seq $count); do
    if [[ ! -d ${home}$i ]]; then
      build/alphabill $partition-genesis --home ${home}$i -g $sdrFlags
      generate_log_configuration ${home}$i
    fi

    rpcServerAddress="localhost:$rpcPort"

    # Already started?
    if lsof -i:$rpcPort >/dev/null; then
      echo "non-validator $partition node" $i "already running? ($rpcServerAddress in use)"
      ((p2pPort=p2pPort+1))
      ((rpcPort=rpcPort+1))
      continue
    fi

    echo "starting non-validator $partition node" $i "($rpcServerAddress)"
    build/alphabill $partition \
      --home ${home}$i \
      --db ${home}$i/$partition/blocks.db \
      --shard-db ${home}$i/$partition/shard.db \
      --tx-db ${home}$i/$partition/tx.db \
      --key-file ${home}$i/$partition/keys.json \
      --genesis $partitionGenesis \
      --trust-base-file $trust_base_file \
      --state ${home}$i/$partition/node-genesis-state.cbor \
      --address "/ip4/127.0.0.1/tcp/$p2pPort" \
      --bootnodes="$bootNodes" \
      --rpc-server-address $rpcServerAddress \
      >> ${home}$i/$partition/$partition.log 2>&1 &
    ((p2pPort=p2pPort+1))
    ((rpcPort=rpcPort+1))
  done
}
