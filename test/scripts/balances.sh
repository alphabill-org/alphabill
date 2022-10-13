#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"
echo "abHome=$abHome"


echo "Get wallet 1 balance"
build/alphabill wallet sync -u localhost:26768 --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ --log-level DEBUG
build/alphabill wallet get-balance --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ --log-level DEBUG

echo "Get wallet 2 balance"
build/alphabill wallet sync -u localhost:26768 --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ --log-level DEBUG
build/alphabill wallet get-balance --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ --log-level DEBUG
