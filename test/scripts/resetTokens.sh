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

echo "Create new token type"
typeId=1111111111111111111111111111111111111111111111111111111111111111
#7D392644BDB35B30A621037E841C909E118161B0061D1B1A3E37E8B6B795C448
build/alphabill wallet token new-type fungible --symbol AB --type $typeId -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List token types"
build/alphabill wallet token list-types -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

#echo "Mint new token"
# currently requires to manually copy token type id from the previous step and to paste for the --type flag to mint the token
#build/alphabill wallet token new fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/
#build/alphabill wallet token new fungible -u localhost:27766 --log-level DEBUG -l ~/.alphabill/wallet1/ --type 2653480555E1D001E7DB561D11D6F47DA87F61B385F14138B2D466063FA7A6C0 --amount 1000

echo "List the tokens"
build/alphabill wallet token list fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/