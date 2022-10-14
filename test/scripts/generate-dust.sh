#!/bin/sh

cd ~/work/alphabill/alphabill

keys=10
# first key:
w=0x029e014f63fc5c2187fbd2c9963e1934413493108cf5c4f3edce835286ae5524fb
for ((i=1; i <= 100; i++))
do
    build/alphabill wallet sync -u localhost:26768 --log-file ~/.alphabill/wallet1/log.log -l ~/.alphabill/wallet1/
    echo $i
    for ((k=1; k <= $keys; k++))
    do
      echo "sending from account $k to wallet $w"
      build/alphabill wallet send -a $w -u localhost:26766 --log-file ~/.alphabill/wallet1/log.log -l ~/.alphabill/wallet1/ -k $k -v 1
    done
    sleep 1
done

build/alphabill wallet sync -u localhost:26768 --log-file ~/.alphabill/wallet1/log.log -l ~/.alphabill/wallet1/
build/alphabill wallet sync -u localhost:26768 --log-file ~/.alphabill/wallet2/log.log -l ~/.alphabill/wallet2/

echo "Get wallet 1 balance"
build/alphabill wallet get-balance --log-file /Users/pavel/.alphabill/wallet1/log.log -l ~/.alphabill/wallet1/
echo "Get wallet 2 balance"
build/alphabill wallet get-balance --log-file /Users/pavel/.alphabill/wallet2/log.log -l ~/.alphabill/wallet2/
