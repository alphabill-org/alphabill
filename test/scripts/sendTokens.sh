#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

cd $abBin

echo "List the tokens w1"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766
echo "List the tokens w2"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ -u localhost:27766

echo "Send"
typeId=7D392644BDB35B30A621037E841C909E118161B0061D1B1A3E37E8B6B795C448
w2=0x029e014f63fc5c2187fbd2c9963e1934413493108cf5c4f3edce835286ae5524fb
build/alphabill wallet token send fungible --type $typeId --amount 3 --address $w2 -k 1 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens Wallet 1"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens Wallet 2"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ -u localhost:27766
