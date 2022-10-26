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
tokenTypeId=56F7E26D4B1EC7B8301F62F407184CD195CCEE9E330CF38E748AF19B12A87A61
# currently requires to manually copy token type id from the previous step and to paste for the --type flag to mint the token
build/alphabill wallet token new fungible --type $tokenTypeId --amount 1000 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766

echo "List the tokens"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766

