#!/bin/bash

port=28666
# partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/tokens$i/tokens/keys.json | tail -n1)
  nodeAddresses="$nodeAddresses,$id=/ip4/127.0.0.1/tcp/$port";

  ((port=port+1))
done

nodeAddresses="${nodeAddresses:1}"

port=28666
grpcPort=28766
restPort=28866
#start partition nodes
for i in 1 2 3
do
  build/alphabill tokens --home testab/tokens$i -f testab/tokens$i/tokens/blocks.db -k testab/tokens$i/tokens/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" --rest-server-address "localhost:$restPort" -g testab/rootchain/genesis/partition-genesis-2.json -p "$nodeAddresses" >> "testab/tokens$i/tokens$i.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
  ((restPort=restPort+1))
done
