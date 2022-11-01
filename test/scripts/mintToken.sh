#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

cd $abBin

echo "List token types"
build/alphabill wallet token list-types -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

echo "Mint new token"
tokenTypeId=7D392644BDB35B30A621037E841C909E118161B0061D1B1A3E37E8B6B795C448
# currently requires to manually copy token type id from the previous step and to paste for the --type flag to mint the token
build/alphabill wallet token new fungible --type $tokenTypeId --amount 1 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766
build/alphabill wallet token new fungible --type $tokenTypeId --amount 2 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766
build/alphabill wallet token new fungible --type $tokenTypeId --amount 5 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766

echo "List the tokens"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

