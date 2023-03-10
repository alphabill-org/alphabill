#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

cd $abBin

url=http://localhost:8080

echo "List token types"
build/alphabill wallet token list-types -u $url --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "List the tokens w1"
build/alphabill wallet token list --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u $url -k 0

echo "List the tokens w2"
build/alphabill wallet token list --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ -u $url -k 0
