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
function boot_node() {
  local home=$1
  local rootPort=$2
  nodeId=$(build/alphabill node-id --home $home | tail -n1)
  echo "/ip4/127.0.0.1/tcp/$rootPort/p2p/$nodeId"
}

# Initializes shard nodes
# first two arguments are mandatory, third is optional
# $1 partition type identifier
# $2 number of nodes to init
# $3 custom CLI args
function init_shard_nodes() {
  local home=""
  partitionTypeId=$1
  case $partitionTypeId in
    1)
      home="testab/money"
      ;;
    2)
      home="testab/tokens"
      ;;
    3)
      home="testab/evm"
      ;;
    4)
      home="testab/orchestration"
      ;;
    5)
      home="testab/tokens-enterprise"
      ;;
    *)
      echo "error: unknown partition type $1" >&2
      return 1
      ;;
  esac

  nodeInfoFiles=
  echo "initializing $2 nodes for partition type $partitionTypeId"
  for i in $(seq 1 "$2")
  do
    # "-g" flag generates keys
    build/alphabill shard-node init --home "${home}$i" -g $3
    nodeInfoFiles+=" --node-info ${home}$i/node-info.json"
  done

  # Generate shard-conf once to testab
  build/alphabill shard-conf generate --home testab \
                  --network-id 3 \
                  --partition-id $partitionTypeId \
                  --partition-type-id $partitionTypeId \
                  --epoch-start 1 \
                  $nodeInfoFiles

  for i in $(seq 1 "$2")
  do
    # Copy shard-conf to node
    cp testab/shard-conf-${partitionTypeId}_0.json ${home}$i/shard-conf.json

    # Generate genesis state from shard-conf
    build/alphabill shard-conf genesis --home ${home}${i}
  done
}

# Initiallize root nodes
# $1 number of root nodes
function init_root_nodes() {
  home=testab/root
  nodeInfoFiles=
  echo "initializing $1 nodes for root chain"
  for i in $(seq 1 "$1")
  do
    build/alphabill root-node init --home "${home}$i" -g
    nodeInfoFiles+=" --node-info ${home}$i/node-info.json"
  done

  # Generate trust-base once to testab
  build/alphabill trust-base generate --home testab --network-id 3 $nodeInfoFiles

  # Sign trust-base by each node
  for i in $(seq 1 "$1")
  do
    build/alphabill trust-base sign --home ${home}$i --trust-base testab/trust-base.json
  done
}

function start_root_nodes() {
  # use root node 1 as bootstrap node
  local bootNode=""
  local p2pPort=$rootPortStart
  local rpcPort=25866

  bootNode=$(boot_node testab/root1 "$rootPortStart")

  i=1
  for node in testab/root*
  do
    if [[ $i -ne 1 ]]; then
      bootNodeParam="--bootnodes=$bootNode"
    fi

    build/alphabill root-node run \
                    --home testab/root$i \
                    --address "/ip4/127.0.0.1/tcp/$p2pPort" \
                    $bootNodeParam \
                    --trust-base testab/trust-base.json \
                    --rpc-server-address "localhost:$rpcPort" \
                    --log-format text \
                    --log-level debug \
                    --metrics prometheus \
                    >> testab/root$i/debug.log 2>&1 &
    nodePID=$!
    # wait until node starts listening on RPC port OR exits because of some error
    until lsof -i:$rpcPort >/dev/null || ! ps -p $nodePID >/dev/null
    do
      echo -n "."
      sleep 0.200
    done

    if ! ps -p $nodePID >/dev/null; then
      echo "failed"
      exit
    fi

    # uplaod all shard confs
    for shardConf in testab/shard-conf-*
    do
      curl -X PUT -H "Content-Type: application/json" -d @${shardConf} \
           http://localhost:${rpcPort}/api/v1/configurations
    done

    ((p2pPort=p2pPort+1))
    ((rpcPort=rpcPort+1))
    ((i=i+1))
  done

  echo
  echo "started $(($i-1)) root nodes"
}

# starts shard nodes
# $1 partition type i.e. one of [money/tokens/evm/orchestration/tokens-enterprise]
function start_shard_nodes() {
  local homePrefix=""
  local p2pPort=0
  local rpcPort=0
  case $1 in
    money)
      homePrefix="testab/money"
      p2pPort=26666
      rpcPort=26866
      ;;
    tokens)
      homePrefix="testab/tokens"
      p2pPort=28666
      rpcPort=28866
      ;;
    evm)
      homePrefix="testab/evm"
      p2pPort=29666
      rpcPort=29866
      ;;
    orchestration)
      homePrefix="testab/orchestration"
      p2pPort=30666
      rpcPort=30866
      ;;
    tokens-enterprise)
      homePrefix="testab/tokens-enterprise"
      p2pPort=31666
      rpcPort=31866
      ;;
    *)
      echo "error: unknown partition $1" >&2
      return 1
      ;;
  esac

  bootNode=$(boot_node testab/root1 "$rootPortStart")

  # Start nodes
  i=1
  for home in `ls -d ${homePrefix}[0-9]*`
  do
    build/alphabill shard-node run \
        --home $home \
        --trust-base testab/trust-base.json \
        --address "/ip4/127.0.0.1/tcp/$p2pPort" \
        --bootnodes $bootNode \
        --rpc-server-address "localhost:$rpcPort" \
        --log-format text \
        --log-level debug \
        >> ${home}/debug.log 2>&1 &

    if [[ $i -eq 1 ]]; then
      echo "sleeping"
      sleep 5
    fi

    ((i=i+1))
    ((p2pPort=p2pPort+1))
    ((rpcPort=rpcPort+1))
  done
  echo "started $(($i-1)) $1 nodes"
}

function start_non_validator_shard_nodes() {
  partition=$1
  count=$2
  extraFlags=$3
  partitionType=$partition
  home="testab/$partition-non-validator"

  echo "starting $count non-validator $partition nodes"

  # Set up partition specific variables
  case $partition in
    money)
      partitionId=1
      p2pPort=36666
      rpcPort=36866
      ;;
    tokens)
      partitionId=2
      p2pPort=38666
      rpcPort=38866
      ;;
    tokens-enterprise)
      partitionId=5
      p2pPort=41666
      rpcPort=41866
      ;;
  esac

  # create bootnodes
  local bootNodes=$(boot_node testab/root1 "$rootPortStart")

  # Start non-validator partition nodes
  for i in $(seq $count); do
    if [[ ! -d ${home}$i ]]; then
      build/alphabill shard-node init --home ${home}$i -g

      # Copy shard-conf and genesis state to node
      cp testab/${partition}1/shard-conf.json ${home}$i
      cp testab/${partition}1/state.cbor ${home}$i

      # generate_log_configuration ${home}$i
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
    build/alphabill shard-node run \
      --home ${home}$i \
      --trust-base testab/trust-base.json \
      --address "/ip4/127.0.0.1/tcp/$p2pPort" \
      --bootnodes "$bootNodes" \
      --rpc-server-address $rpcServerAddress \
      --log-format text \
      --log-level debug \
      >> ${home}$i/debug.log 2>&1 &

    ((p2pPort=p2pPort+1))
    ((rpcPort=rpcPort+1))
  done
}
