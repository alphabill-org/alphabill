#!/bin/bash
set -x
homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

cd $abBin

echo "Wallet 1"
#build/alphabill wallet create -s "use grid fetch reflect file bright average mercy morning leisure sad boil" --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/

mainKey1=$(build/alphabill wallet get-pubkeys -l $abHome/wallet1/ --quiet)
# 0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64
echo "mainKey1=$mainKey1"

sync=true
w1=$(echo "--sync $sync -u localhost:27766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/")


pred1=0x53510087 #push bool false, equal; to satisfy: 5100
pred=0x535101 #always true

echo "Create new fungible token types"
typeId1=01
typeId2=02
typeId3=03
typeId4=04
build/alphabill wallet token new-type fungible $w1 --symbol AB --type $typeId1 --subtype-clause $pred1
build/alphabill wallet token new-type fungible $w1 --symbol AB --type $typeId2 --parent-type $typeId1 --subtype-clause $pred --subtype-input 0x535100
build/alphabill wallet token new-type fungible $w1 --symbol AB --type $typeId3 --parent-type $typeId2 --subtype-input 0x53,0x535100
# the following command is a failing one, tx is rejected since creation input is invalid
#build/alphabill wallet token new-type fungible $w1 --symbol AB --type $typeId4 --parent-type $typeId2 --subtype-input empty,empty

echo "Create new NFT types"
nftTypeId1=10
nftTypeId2=20
nftTypeId3=30
nftTypeId4=40
build/alphabill wallet token new-type non-fungible $w1 --symbol ABNFT --type $nftTypeId1 --subtype-clause $pred1
build/alphabill wallet token new-type non-fungible $w1 --symbol ABNFT --type $nftTypeId2 --parent-type $nftTypeId1 --subtype-clause $pred --subtype-input 0x535100
build/alphabill wallet token new-type non-fungible $w1 --symbol ABNFT --type $nftTypeId3 --parent-type $nftTypeId2 --subtype-input 0x53,0x535100
#build/alphabill wallet token new-type non-fungible $w1 --symbol ABNFT --type $nftTypeId4 --parent-type $nftTypeId2 --subtype-input empty,empty


echo "List token types"
build/alphabill wallet token list-types $w1
