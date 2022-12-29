#!/bin/bash
# build binary
make clean build
mkdir testab
mkdir testab/rootchain
moneyNodeAddresses=""
vdNodeAddresses=""
tokensNodeAddresses=""

# generate fee bill for each partition
moneyFeeBill='{"systemId": "0x00000000", "unitId": "0x0000000000000000000000000000000000000000000000000000000000000002", "ownerPubKey": "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"}'
vdFeeBill='{"systemId": "0x00000001", "unitId": "0x0000000000000000000000000000000000000000000000000000000000000003", "ownerPubKey": "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"}'
tokensFeeBill='{"systemId": "0x00000002", "unitId": "0x0000000000000000000000000000000000000000000000000000000000000004", "ownerPubKey": "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"}'
echo $moneyFeeBill > testab/money-fee-bill.json
echo $vdFeeBill > testab/vd-fee-bill.json
echo $tokensFeeBill > testab/tokens-fee-bill.json

# Generate money node genesis files.
for i in 1 2 3
do
  # "-g" flags also generates keys
  build/alphabill money-genesis --home testab/money$i -g -c testab/money-fee-bill.json -c testab/vd-fee-bill.json -c testab/tokens-fee-bill.json
done

# Generate vd node genesis files.
for i in 1 2 3
do
  mkdir testab/vd$i
  # "-g" flags also generates keys
  build/alphabill vd-genesis --home testab/vd$i -g
done

# Generate token partition node genesis files.
for i in 1 2 3
do
  mkdir testab/tokens$i
  # "-g" flags also generates keys
  build/alphabill tokens-genesis --home testab/tokens$i -g
done

# generate rootchain and partition genesis files
build/alphabill root-genesis --home testab/rootchain -o testab/rootchain/genesis -p testab/tokens1/tokens/node-genesis.json -p testab/tokens2/tokens/node-genesis.json -p testab/tokens3/tokens/node-genesis.json -p testab/money1/money/node-genesis.json -p testab/money2/money/node-genesis.json -p testab/money3/money/node-genesis.json -p testab/vd1/vd/node-genesis.json -p testab/vd2/vd/node-genesis.json -p testab/vd3/vd/node-genesis.json -k testab/rootchain/keys.json -g

#start root chain
build/alphabill root --home testab/rootchain -f testab/rootchain/rounds.db -k testab/rootchain/keys.json -g testab/rootchain/genesis/root-genesis.json > testab/rootchain/rootchain.log &

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
moneyRestPort=26866
#start money partition nodes
for i in 1 2 3
do
  build/alphabill money --home testab/money$i -f testab/money$i/money/blocks.db -k testab/money$i/money/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$moneyPort" --server-address ":$moneyGrpcPort" --rest-server-address "localhost:$moneyRestPort" -g testab/rootchain/genesis/partition-genesis-0.json -p "$moneyNodeAddresses" > "testab/money$i/money$i.log" &
  ((moneyPort=moneyPort+1))
  ((moneyGrpcPort=moneyGrpcPort+1))
  ((moneyRestPort=moneyRestPort+1))
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
vdRestPort=27866
#start vd partition nodes
for i in 1 2 3
do
  build/alphabill vd --home testab/vd$i -f testab/vd$i/vd/blocks.db -k testab/vd$i/vd/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$vdPort" --server-address ":$vdGrpcPort" --rest-server-address "localhost:$vdRestPort" -g testab/rootchain/genesis/partition-genesis-1.json -p "$vdNodeAddresses" > "testab/vd$i/vd$i.log" &
  ((vdPort=vdPort+1))
  ((vdGrpcPort=vdGrpcPort+1))
  ((vdRestPort=vdRestPort+1))
done

tokensPort=28666
# tokens partition node addresses
for i in 1 2 3
do
  id=$(build/alphabill identifier -k testab/tokens$i/tokens/keys.json | tail -n1)
  tokensNodeAddresses="$tokensNodeAddresses,$id=/ip4/127.0.0.1/tcp/$tokensPort";

  ((tokensPort=tokensPort+1))
done

tokensNodeAddresses="${tokensNodeAddresses:1}"

tokensPort=28666
tokensGrpcPort=28766
tokensRestPort=28866
#start tokens partition nodes
for i in 1 2 3
do
  build/alphabill tokens --home testab/tokens$i -f testab/tokens$i/tokens/blocks.db -k testab/tokens$i/tokens/keys.json -r "/ip4/127.0.0.1/tcp/26662" -a "/ip4/127.0.0.1/tcp/$tokensPort" --server-address ":$tokensGrpcPort" --rest-server-address "localhost:$tokensRestPort" -g testab/rootchain/genesis/partition-genesis-2.json -p "$tokensNodeAddresses" > "testab/tokens$i/tokens$i.log" &
  ((tokensPort=tokensPort+1))
  ((tokensGrpcPort=tokensGrpcPort+1))
  ((tokensRestPort=tokensRestPort+1))
done