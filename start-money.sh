#!/bin/bash
# build binary
make clean build
mkdir testab
mkdir testab/money-rootchain
nodeAddresses=""

# Generate node genesis files.
for i in 1 2 3
do
  # "-f" flags also generates keys
  build/alphabill money-genesis --home testab/money$i -f
done

# generate rootchain and partition genesis files
build/alphabill root-genesis --home testab/money-rootchain -p testab/money1/money/node-genesis.json -p testab/money2/money/node-genesis.json -p testab/money3/money/node-genesis.json -k testab/money-rootchain/keys.json -f

#start root chain
build/alphabill root --home testab/money-rootchain -k testab/money-rootchain/keys.json -g testab/money-rootchain/rootchain/root-genesis.json > testab/money-rootchain/log.log &

port=26666
# partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/money$i/money/keys.json | tail -n1)
  nodeAddresses="$nodeAddresses,$id=/ip4/127.0.0.1/tcp/$port";

  ((port=port+1))
done

nodeAddresses="${nodeAddresses:1}"

port=26666
grpcPort=26766
#start partition nodes
for i in 1 2 3
do
  build/alphabill money --home testab/money$i -k testab/money$i/money/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" -g testab/money-rootchain/rootchain/partition-genesis-0.json -p "$nodeAddresses" > "testab/money$i/log.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
done
