curl -H "Origin: foo" \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":12345,"method":"state_getBlock","params":[1]}' \
     http://127.0.0.1:26866/rpc
