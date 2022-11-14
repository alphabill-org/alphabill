#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

cd $abBin

echo "List token types w1"
build/alphabill wallet token list-types -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "Mint new token w1"
tokenTypeId=1000000000000000000000000000000000000000000000000000000000000000
build/alphabill wallet token new fungible --type $tokenTypeId --amount 1 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766
build/alphabill wallet token new fungible --type $tokenTypeId --amount 2 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766
build/alphabill wallet token new fungible --type $tokenTypeId --amount 5 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766

echo "List the tokens w1"
build/alphabill wallet token list --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

