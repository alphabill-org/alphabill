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

echo "Transfer to 'always true'"
token1=B7F704D8A9BF8CE396C51B812AD69A7ECCDD595709E00706F75B637AB9564949
token2=47596DA0AA360C7A4C5123E195D7BD362AACF77CBC1AAE88966522BD6287C83E
w2=0x029E014F63FC5C2187FBD2C9963E1934413493108CF5C4F3EDCE835286AE5524FB
build/alphabill wallet token transfer fungible --token-identifier $token2 --address $w2 -k 1 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766
build/alphabill wallet token transfer fungible --token-identifier $token1 --address true -k 1 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens Wallet 1"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens Wallet 2"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ -u localhost:27766
