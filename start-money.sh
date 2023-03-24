#!/bin/bash
# build binary
make clean build
mkdir testab
mkdir testab/rootchain
nodeAddresses=""

# Generate node genesis files.
for i in 1 2 3
do
  # "-g" flags also generates keys
  build/alphabill money-genesis --home testab/money$i -g
done

# generate rootchain and partition genesis files
build/alphabill root-genesis --home testab/rootchain -o testab/rootchain/genesis -p testab/money1/money/node-genesis.json -p testab/money2/money/node-genesis.json -p testab/money3/money/node-genesis.json -k testab/rootchain/keys.json -g

#start root chain
build/alphabill root --home testab/rootchain -f testab/rootchain/rounds.db -k testab/rootchain/keys.json -g testab/rootchain/genesis/root-genesis.json > testab/rootchain/rootchain.log &

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
restPort=26866
#start partition nodes
for i in 1 2 3
do
  build/alphabill money --home testab/money$i -f testab/money$i/money/blocks.db -k testab/money$i/money/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$port" --server-address ":$grpcPort" --rest-server-address "localhost:$restPort" -g testab/rootchain/genesis/partition-genesis-0.json -p "$nodeAddresses" > "testab/money$i/money$i.log" &
  ((port=port+1))
  ((grpcPort=grpcPort+1))
  ((restPort=restPort+1))
done

#start Money partition backend
build/alphabill money-backend start -u localhost:26766 -s localhost:9654 -f testab/money-backend/bills.db --log-file testab/money-backend/money-backend.log --log-level DEBUG &

echo "Started money backend, check the API at http://localhost:9654/api/v1/swagger/"
