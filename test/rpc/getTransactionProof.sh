curl -H "Origin: foo" \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":12345,"method":"state_getTransactionProof","params":["6IPzkZxAcuRZZCoBLPdzfO4xHaCt6IqhR4BIR5d4TNk="]}' \
     http://127.0.0.1:26866/rpc
