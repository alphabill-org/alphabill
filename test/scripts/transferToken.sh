#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

cd $abBin

echo "List the tokens"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "Transfer to 'always true'"
typeId=0D81C5073FF484C681BEFB3F809F0F5B2E71B5D4CDAB92C8ED19860D04F4192A
token1=C1ACD74EC93FFA0441401C36FD566661189B5A0FCE9D5E2B645A0D6D01EFCB86
token2=FD09D8E297136790E1D89D7DED8D0751D2415640376405636BDC70C8A8D3AB86
w2=029e014f63fc5c2187fbd2c9963e1934413493108cf5c4f3edce835286ae5524fb
build/alphabill wallet token transfer fungible --token $token1 --address true -k 1 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766
build/alphabill wallet token transfer fungible --token $token2 --address 029e014f63fc5c2187fbd2c9963e1934413493108cf5c4f3edce835286ae5524fb -k 1 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens Wallet 1"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens Wallet 2"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ -u localhost:27766
