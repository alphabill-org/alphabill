#!/bin/bash
# build binary
make clean build
mkdir testab
mkdir testab/rootchain
nodeAddresses=""

# Generate node genesis files.
for i in 1 2 3
do
  mkdir testab/vd$i
  # "-g" flags also generates keys
  build/alphabill vd-genesis --home testab/vd$i -g
done

# generate rootchain and partition genesis files
build/alphabill root-genesis --home testab/rootchain -o testab/rootchain/genesis -p testab/vd1/vd/node-genesis.json -p testab/vd2/vd/node-genesis.json -p testab/vd3/vd/node-genesis.json -k testab/rootchain/keys.json -g

#start root chain
build/alphabill root --home testab/rootchain -k testab/rootchain/keys.json -g testab/rootchain/genesis/root-genesis.json > testab/rootchain/log.log &

port=27666
# partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/vd$i/vd/keys.json | tail -n1)
  nodeAddresses="$nodeAddresses,$id=/ip4/127.0.0.1/tcp/$port";

  ((port=port+1))
done

nodeAddresses="${nodeAddresses:1}"

port=27666
grpcPort=27766
#start partition nodes
for i in 1 2 3
do
  build/alphabill vd --home testab/vd$i -f testab/vd$i/vd/blocks.db -k testab/vd$i/vd/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" -g testab/rootchain/genesis/partition-genesis-1.json -p "$nodeAddresses" > "testab/vd$i/log.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
done
