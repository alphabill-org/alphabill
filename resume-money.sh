#!/bin/bash

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
  build/alphabill money --home testab/money$i -f testab/money$i/money/blocks.db -k testab/money$i/money/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" -g testab/rootchain/rootchain/partition-genesis-0.json -p "$nodeAddresses" > "testab/money$i/log.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
done
