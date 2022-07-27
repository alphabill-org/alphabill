#!/bin/bash
# build binary
make clean build
mkdir testab
mkdir testab/rootchain
moneyNodeAddresses=""
vdNodeAddresses=""

# Generate money node genesis files.
for i in 1 2 3
do
  # "-g" flags also generates keys
  build/alphabill money-genesis --home testab/money$i -g
done

# Generate vd node genesis files.
for i in 1 2 3
do
  mkdir testab/vd$i
  # "-g" flags also generates keys
  build/alphabill vd-genesis --home testab/vd$i -g
done

# generate rootchain and partition genesis files
build/alphabill root-genesis --home testab/rootchain -o testab/rootchain/genesis -p testab/money1/money/node-genesis.json -p testab/money2/money/node-genesis.json -p testab/money3/money/node-genesis.json -p testab/vd1/vd/node-genesis.json -p testab/vd2/vd/node-genesis.json -p testab/vd3/vd/node-genesis.json -k testab/rootchain/keys.json -g

#start root chain
build/alphabill root --home testab/rootchain -k testab/rootchain/keys.json -g testab/rootchain/genesis/root-genesis.json > testab/rootchain/log.log &

moneyPort=26666
# money partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/money$i/money/keys.json | tail -n1)
  moneyNodeAddresses="$moneyNodeAddresses,$id=/ip4/127.0.0.1/tcp/$moneyPort";

  ((moneyPort=moneyPort+1))
done

moneyNodeAddresses="${moneyNodeAddresses:1}"

moneyPort=26666
moneyGrpcPort=26766
#start money partition nodes
for i in 1 2 3
do
  build/alphabill money --home testab/money$i -f testab/money$i/money/blocks.db -k testab/money$i/money/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$moneyPort" --server-address ":$moneyGrpcPort" -g testab/rootchain/genesis/partition-genesis-0.json -p "$moneyNodeAddresses" > "testab/money$i/log.log" &
  ((moneyPort=moneyPort+1))
  ((moneyGrpcPort=moneyGrpcPort+1))
done

vdPort=27666
# vd partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/vd$i/vd/keys.json | tail -n1)
  vdNodeAddresses="$vdNodeAddresses,$id=/ip4/127.0.0.1/tcp/$vdPort";

  ((vdPort=vdPort+1))
done

vdNodeAddresses="${vdNodeAddresses:1}"

vdPort=27666
vdGrpcPort=27766
#start vd partition nodes
for i in 1 2 3
do
  build/alphabill vd --home testab/vd$i -f testab/vd$i/vd/blocks.db -k testab/vd$i/vd/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$vdPort" --server-address ":$vdGrpcPort" -g testab/rootchain/genesis/partition-genesis-1.json -p "$vdNodeAddresses" > "testab/vd$i/log.log" &
  ((vdPort=vdPort+1))
  ((vdGrpcPort=vdGrpcPort+1))
done
