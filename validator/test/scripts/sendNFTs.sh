#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

cd $abBin

mainKey2=$(build/alphabill wallet get-pubkeys -l $abHome/wallet2/ --quiet)

echo "List the tokens w1"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List the tokens w2"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/

echo "Send NFT to w2"
nftId2=2000000000000000000000000000000000000000000000000000000000000002
build/alphabill wallet token send non-fungible --token-identifier $nftId2 --address "$mainKey2" -k 1 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens w1"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List the tokens w2"
build/alphabill wallet token list non-fungible -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/