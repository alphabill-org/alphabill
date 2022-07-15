#!/bin/bash

nodeAddresses=""

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
  build/alphabill vd --home testab/vd$i -f testab/vd$i/vd/blocks.db -k testab/vd$i/vd/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" -g testab/rootchain/partition-genesis-1.json -p "$nodeAddresses" > "testab/vd$i/log.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
done
