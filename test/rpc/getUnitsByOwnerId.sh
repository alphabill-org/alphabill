curl -H "Origin: foo" \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":12345,"method":"state_getUnitsByOwnerID","params":["9SAiu0UEB9kvE78cUxKKZ2vPMEgY6fQaXvTr6unA1rA="]}' \
     http://127.0.0.1:26866/rpc
