#!/bin/bash

dbFile=~/.alphabill/tokens/tokens.db

rm $dbFile

./build/alphabill token-backend start -u localhost:28766 -s localhost:8080 -f $dbFile &
