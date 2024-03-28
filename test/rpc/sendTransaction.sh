curl -H "Origin: foo" \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":12345,"method":"state_sendTransaction","params":["0x838500657472616e7341018344830001f601f6830000f64101f6"]}' \
     http://127.0.0.1:26866/rpc
