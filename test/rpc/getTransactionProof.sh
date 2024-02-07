curl -H "Origin: foo" \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":12345,"method":"state_getTransactionProof","params":["0xe883f3919c4072e459642a012cf7737cee311da0ade88aa14780484797784cd9"]}' \
     http://127.0.0.1:26866/rpc
