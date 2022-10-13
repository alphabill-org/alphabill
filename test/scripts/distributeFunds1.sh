#!/bin/bash

homeDir=~
abBin=$homeDir/work/alphabill/alphabill
abHome=$homeDir/.alphabill

echo "homeDir=$homeDir"
echo "abBin=$abBin"

build/alphabill wallet sync -u localhost:26768 --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ --log-level DEBUG

echo "Give money to other keys in wallet 1"
IFS=$'\n'
keys_array=( $(build/alphabill wallet get-pubkeys -l $abHome/wallet1/ --quiet) )
#for (( n=1; n < ${#keys_array[*]}; n++))
#do
  key=${keys_array[1]}
  echo "$key (#1)"
  build/alphabill wallet send -a $key -u localhost:26766 --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ -k 1 -v 10000 --log-level DEBUG
  sleep 2
  build/alphabill wallet sync -u localhost:26768 --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ --log-level DEBUG
#done

echo "Get wallet 1 balance"
build/alphabill wallet get-balance --log-file $abHome/wallet1/w1.log -l $abHome/wallet1/ --log-level DEBUG
