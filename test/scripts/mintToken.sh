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
tokenTypeId=019E9928DD3F1396DF673F19F6634369B41190AD62AD80CDCE8FF24E3E07441A
# currently requires to manually copy token type id from the previous step and to paste for the --type flag to mint the token
build/alphabill wallet token new fungible --type $tokenTypeId --amount 1000 --log-level DEBUG -l ~/.alphabill/wallet1/ --log-file $abHome/wallet1/w1.log -u localhost:27766

echo "List the tokens"
build/alphabill wallet token list fungible --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -u localhost:27766
