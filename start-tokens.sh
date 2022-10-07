#!/bin/bash
# build binary
make clean build
mkdir testab
mkdir testab/rootchain
nodeAddresses=""

# Generate node genesis files.
for i in 1 2 3
do
  mkdir testab/tokens$i
  # "-g" flags also generates keys
  build/alphabill tokens-genesis --home testab/tokens$i -g
done

# generate rootchain and partition genesis files
build/alphabill root-genesis --home testab/rootchain -o testab/rootchain/genesis -p testab/tokens1/tokens/node-genesis.json -p testab/tokens2/tokens/node-genesis.json -p testab/tokens3/tokens/node-genesis.json -k testab/rootchain/keys.json -g

#start root chain
build/alphabill root --home testab/rootchain -f testab/rootchain/rounds.db -k testab/rootchain/keys.json -g testab/rootchain/genesis/root-genesis.json > testab/rootchain/rootchain.log &

port=27666
# partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/tokens$i/tokens/keys.json | tail -n1)
  nodeAddresses="$nodeAddresses,$id=/ip4/127.0.0.1/tcp/$port";

  ((port=port+1))
done

nodeAddresses="${nodeAddresses:1}"

port=27666
grpcPort=27766
#start partition nodes
for i in 1 2 3
do
  build/alphabill tokens --home testab/tokens$i -f testab/tokens$i/tokens/blocks.db -k testab/tokens$i/tokens/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" -g testab/rootchain/genesis/partition-genesis-2.json -p "$nodeAddresses" > "testab/tokens$i/tokens$i.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
done
