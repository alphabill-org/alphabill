#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

echo "Deleting wallets"
rm $abHome/wallet1/*
rm $abHome/wallet2/*
rm $abHome/faucet/log.log
rm $abHome/faucet/wallet.db
echo "Done"

mkdir $abHome/wallet1
mkdir $abHome/wallet2
# mkdir $abHome/faucet

cd $abBin

keys=10 #total keys including the initial one

echo "Creating wallets"
echo "Wallet 1"
build/alphabill wallet create -s "use grid fetch reflect file bright average mercy morning leisure sad boil" --log-level DEBUG --log-file $abHome/wallet1/log.log -l $abHome/wallet1/

mainKey=$(build/alphabill wallet get-pubkeys -l $abHome/wallet1/ --quiet)
# 0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64
echo "mainKey=$mainKey"

echo "Wallet 2"
build/alphabill wallet create -s "payment head during analyst system property skirt garage attend typical sing seed" --log-level DEBUG --log-file $abHome/wallet2/log.log -l $abHome/wallet2/


echo "Generating extra keys"
for ((i=2; i <= $keys; i++))
do
    build/alphabill wallet --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ add-key
    build/alphabill wallet --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/ add-key
done

echo "Faucet wallet"
build/alphabill wallet create -s "nothing foam afraid same dumb into swap blade add birth suit real" --log-level DEBUG --log-file $abHome/faucet/log.log -l $abHome/faucet/
echo "Done"

echo "Spend initial bill"
go run scripts/money/spend_initial_bill.go --pubkey $mainKey --alphabill-uri localhost:26766 --bill-id 1 --bill-value 1000000 --timeout 1000
#go run scripts/money/spend_initial_bill.go --pubkey 0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64 --alphabill-uri localhost:26766 --bill-id 1 --bill-value 1000000 --timeout 1000
sleep 3

build/alphabill wallet sync -u localhost:26768 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ --log-level DEBUG

sleep 3

echo "Get wallet 1 balance"
build/alphabill wallet get-balance --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/
echo "Get wallet 2 balance"
build/alphabill wallet get-balance --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/

echo "Give money to other keys in wallet 1"
IFS=$'\n'
keys_array=( $(build/alphabill wallet get-pubkeys -l $abHome/wallet1/ --quiet) )
for (( n=1; n < ${#keys_array[*]}; n++))
do
  key=${keys_array[n]}
  echo "$key (#$n)"
  build/alphabill wallet send -a $key -u localhost:26766 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -k 1 -v 10000
  sleep 5
  build/alphabill wallet sync -u localhost:26768 --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/
done

echo "Get wallet 1 balance"
build/alphabill wallet get-balance --log-level DEBUG --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/
echo "Get wallet 2 balance"
build/alphabill wallet get-balance --log-level DEBUG --log-file $abHome/wallet2/w2.log -l $abHome/wallet2/