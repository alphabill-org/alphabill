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
  build/alphabill tokens --home testab/tokens$i -f testab/tokens$i/tokens/blocks.db -k testab/tokens$i/tokens/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" --rest-server-address "localhost:$restPort" -g testab/rootchain/genesis/partition-genesis-2.json -p "$nodeAddresses" > "testab/tokens$i/tokens$i.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
  ((restPort=restPort+1))
done

#start UTP backend
build/alphabill token-backend start -u localhost:28766 -s localhost:9735 -f testab/token-backend/tokens.db --log-file testab/token-backend/token-backend.log &

echo "Started tokens backend, check the API at http://localhost:9735/api/v1/swagger/"
