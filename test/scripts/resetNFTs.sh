#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

echo "Deleting wallets"
rm $abHome/wallet1/*
rm $abHome/wallet2/*

mkdir $abHome/wallet1
mkdir $abHome/wallet2

cd $abBin

echo "Wallet 1"
build/alphabill wallet create -s "use grid fetch reflect file bright average mercy morning leisure sad boil" --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

mainKey1=$(build/alphabill wallet get-pubkeys -l $abHome/wallet1/ --quiet)
# 0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64
echo "mainKey1=$mainKey1"

echo "Wallet 2"
build/alphabill wallet create -s "payment head during analyst system property skirt garage attend typical sing seed" --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/

mainKey2=$(build/alphabill wallet get-pubkeys -l $abHome/wallet2/ --quiet)
echo "mainKey2=$mainKey2"

# NB! assumes nodes are running

echo "Create new NFT type"
typeId2=2000000000000000000000000000000000000000000000000000000000000000
build/alphabill wallet token new-type non-fungible --symbol ABNFT --type $typeId2 -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List token types"
build/alphabill wallet token list-types -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "Mint new NFT"
nftId1=2000000000000000000000000000000000000000000000000000000000000001
build/alphabill wallet token new non-fungible --type $typeId2 --token-identifier $nftId1 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766

nftId2=2000000000000000000000000000000000000000000000000000000000000002
build/alphabill wallet token new non-fungible --type $typeId2 --token-identifier $nftId2 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766

echo "List the tokens w1"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List the tokens w2"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/

echo "Send NFT to w2"
build/alphabill wallet token send non-fungible --token-identifier $nftId2 --address "$mainKey2" -k 1 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens w1"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List the tokens w2"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/