curl -H "Origin: foo" \
     -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":12345,"method":"state_getUnitsByOwnerID","params":["0xf52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"]}' \
     http://127.0.0.1:26866/rpc
