#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

cd $abBin

#build/alphabill wallet token sync -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/
#exit
echo "List token types"
build/alphabill wallet token list-types -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List the tokens w1"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

echo "List the tokens w2"
build/alphabill wallet get-pubkeys --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ -u localhost:27766