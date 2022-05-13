#!/bin/bash
# build binary
make clean build
mkdir testab
mkdir testab/rootchain
nodeAddresses=""

# Generate node genesis files.
for i in 1 2 3
do
  mkdir testab/node$i
  # "-f" flags also generates keys
  build/alphabill vd-genesis --home testab/node$i -f
done

# generate rootchain and partition genesis files
build/alphabill root-genesis --home testab -p testab/node1/vd/node-genesis.json -p testab/node2/vd/node-genesis.json -p testab/node3/vd/node-genesis.json -k testab/rootchain/keys.json -f

#start root chain
build/alphabill root --home testab -k testab/rootchain/keys.json -g testab/rootchain/root-genesis.json > testab/rootchain/log.log &

port=26666
# partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/node$i/vd/keys.json | tail -n1)
  nodeAddresses="$nodeAddresses,$id=/ip4/127.0.0.1/tcp/$port";

  ((port=port+1))
done

nodeAddresses="${nodeAddresses:1}"

port=26666
grpcPort=26766
#start partition nodes
for i in 1 2 3
do
  build/alphabill vd --home testab/node$i -k testab/node$i/vd/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" -g testab/rootchain/partition-genesis-1.json -p "$nodeAddresses" > "testab/node$i/log.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
done
