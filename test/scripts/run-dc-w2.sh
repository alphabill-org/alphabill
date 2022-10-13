#!/bin/sh

cd ~/work/alphabill/alphabill

echo "Get wallet 2 balance before DC"
build/alphabill wallet get-balance --log-file /Users/pavel/.alphabill/wallet2/w2.log -l ~/.alphabill/wallet2/ --log-level DEBUG

echo "Run wallet 2 DC"
build/alphabill wallet collect-dust -u localhost:26766 --log-file ~/.alphabill/wallet2/w2.log -l ~/.alphabill/wallet2/ --log-level DEBUG
build/alphabill wallet sync -u localhost:26768 --log-file ~/.alphabill/wallet2/w2.log -l ~/.alphabill/wallet2/ --log-level DEBUG

echo "Get wallet 2 balance after DC"
build/alphabill wallet get-balance --log-file /Users/pavel/.alphabill/wallet2/w2.log -l ~/.alphabill/wallet2/ --log-level DEBUG
