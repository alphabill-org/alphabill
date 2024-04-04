# Alphabill Partition Node JSON-RPC API

- [admin_getNodeInfo](#admin_getnodeinfo)
- [state_getRoundNumber](#state_getroundnumber)
- [state_getUnit](#state_getunit)
- [state_getUnitsByOwnerID](#state_getunitsbyownerid)
- [state_sendTransaction](#state_sendtransaction)
- [state_getTransactionProof](#state_gettransactionproof)
- [state_getBlock](#state_getblock)

## admin_getNodeInfo
Returns general information about the Alphabill partition node.

### Parameters
None

### Returns
* *nodeInfo* (object)
  * `systemId` (number) - A unique identifier of the partition this node is running.
  * `name` (string) - Name of the node, one of 'money node', 'tokens node' or 'evm node'.
  * `self` (object *peerInfo*) - An object containing the peer info of the node:
    * `identifier` (string) - Peer ID derived from the public key of the peer.
    * `addresses` (array of string) - An array of network addresses of the peer in human-readable multiaddr format.
  * `bootstrapNodes` (array of *peerInfo*) - Bootstrap peers.
  * `rootValidators` (array of *peerInfo*) - Root chain validators.
  * `partitionValidators` (array of *peerInfo*) - Complete list of partition validators.
  * `openConnections` (array of *peerInfo*) - Connected peers.

### Example
Request
```
curl -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"admin_getNodeInfo"}' \
     http://127.0.0.1:26866/rpc
```
Response
```
{"jsonrpc":"2.0","id":1,"result":{"systemId":1,"name":"money node","self":{"identifier":"16Uiu2HAm5vakCs1Eq1Y6W2zNaUCNEFrnKWpLWRk5aBzrNNkQ6vPa","addresses":["/ip4/127.0.0.1/tcp/26666"]},"bootstrapNodes":[{"identifier":"16Uiu2HAmKG85pVV8YLboc9jcqAYta5y9DMhB3pjWAnBRy1LLvtEC","addresses":["/ip4/127.0.0.1/tcp/26662"]}],"rootValidators":[{"identifier":"16Uiu2HAmKG85pVV8YLboc9jcqAYta5y9DMhB3pjWAnBRy1LLvtEC","addresses":["/ip4/127.0.0.1/tcp/26662"]},{"identifier":"16Uiu2HAmKmZ19iXctcnp27DLBaneHWFk6DR1JUbzo44Rzqb7gqCe","addresses":["/ip4/127.0.0.1/tcp/26663"]},{"identifier":"16Uiu2HAmJFe711jsopJ1CcGNA6ecZuLecGDojeDi8EDpencJ4HUy","addresses":["/ip4/127.0.0.1/tcp/26664"]}],"partitionValidators":[{"identifier":"16Uiu2HAm5vakCs1Eq1Y6W2zNaUCNEFrnKWpLWRk5aBzrNNkQ6vPa","addresses":["/ip4/127.0.0.1/tcp/26666"]},{"identifier":"16Uiu2HAmMGrgJcDqMJMmAmLnugNR8pAzp7hR1VLn7Pek8fPgeV8A","addresses":["/ip4/127.0.0.1/tcp/26667"]}],"openConnections":[{"identifier":"16Uiu2HAmKG85pVV8YLboc9jcqAYta5y9DMhB3pjWAnBRy1LLvtEC","addresses":["/ip4/127.0.0.1/tcp/26662"]},{"identifier":"16Uiu2HAmKmZ19iXctcnp27DLBaneHWFk6DR1JUbzo44Rzqb7gqCe","addresses":["/ip4/127.0.0.1/tcp/26663"]},{"identifier":"16Uiu2HAmJFe711jsopJ1CcGNA6ecZuLecGDojeDi8EDpencJ4HUy","addresses":["/ip4/127.0.0.1/tcp/26664"]},{"identifier":"16Uiu2HAmMGrgJcDqMJMmAmLnugNR8pAzp7hR1VLn7Pek8fPgeV8A","addresses":["/ip4/127.0.0.1/tcp/26667"]}]}}
```

## state_getRoundNumber
Returns the round number of the latest unicity certificate.

### Parameters
None

### Returns
* *roundNumber* (string) - Round number of the latest unicity certificate.

### Example
Request
```
curl -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"state_getRoundNumber"}' \
    http://127.0.0.1:26866/rpc
```
Response
```
{"jsonrpc":"2.0","id":1,"result":"30"}
```

## state_getUnit
Returns the unit data, optionally including the state proof.

### Parameters
1. *unitId* (string) - Hex encoded unit identifier.
2. *includeStateProof* (boolean) - If true, the response will also include the state proof.
```
params: [
  "0x000000000000000000000000000000000000000000000000000000000000000100",
  false
]
```

### Returns
* *unit* (object)
  * `unitId` (string) - Hex encoded unit identifier.
  * `data` (object) - Unit data depending on the unit type.
  * `ownerPredicate` (string) - Hex encoded owner predicate of the unit.
  * `stateProof` (object *stateProof*) - A proof of the state (contents of `data` and `ownerPredicate`) of the unit in the latest round.

### Example
Request
```
curl -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"state_getUnit","params":["0x000000000000000000000000000000000000000000000000000000000000000100",false]}' \
     http://127.0.0.1:26866/rpc
```
Response
```
{"jsonrpc":"2.0","id":1,"result":{"unitId":"0x000000000000000000000000000000000000000000000000000000000000000100","data":{"value":"1000000000000000000","lastUpdate":"0","backlink":"","locked":"0"},"ownerPredicate":"0x83004101f6"}}
```

## state_getUnitsByOwnerID
Returns unit identifiers by owner identifier. Owner identifier is derived from
the owner predicate of the unit. Currently only SHA256 hash of the
public key is used as the owner identifier.

### Parameters
1. *ownerId* (string) - Hex encoded owner identifier.

```
params: [
  "0xf52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0",
]
```

### Returns
* *unitIds* (array of string) - Hex encoded unit identifiers.

### Example
Request
```
curl -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"state_getUnitsByOwnerID","params":["0xf52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"]}' \
     http://127.0.0.1:26866/rpc
```
Response
```
{"jsonrpc":"2.0","id":1,"result":["0x000000000000000000000000000000000000000000000000000000000000001100","0x000000000000000000000000000000000000000000000000000000000000001300","0x000000000000000000000000000000000000000000000000000000000000001200"]}
```

## state_sendTransaction
Sends a raw CBOR encoded signed transaction to the network. Returns
the transaction hash, that can be used with `state_getTransactionProof`
method to confirm the execution of the transaction.

### Parameters
1. *tx* (string) - Hex encoded raw transaction.
```
params: [
  "0x838500657472616e7341018344830001f601f6830000f64101f6",
]
```

### Returns
* *txHash* (string) - Hex encoded transaction hash. Hash algorithm is a dependent on the partition configuration.

### Example
Request
```
curl -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"state_sendTransaction","params":["0x838500657472616e7341018344830001f601f6830000f64101f6"]}' \
     http://127.0.0.1:26866/rpc
```
Response
```
{"jsonrpc":"2.0","id":1,"result":{"0xe883f3919c4072e459642a012cf7737cee311da0ade88aa14780484797784cd9"}}
```

## state_getTransactionProof
Returns the transaction execution proof, if the transaction has been
executed. This method can be used to confirm the transaction, but also
to get the proof needed as input for other transactions.

### Parameters
1. *txHash* (string) - Hex encoded transaction hash.

```
params: [
  "0xe883f3919c4072e459642a012cf7737cee311da0ade88aa14780484797784cd9"
]
```

### Returns
* *recordAndProof* (object)
  * `txRecord` (string) - Hex encoded transaction record.
  * `txProof` (string) - Hex encoded transaction proof.

Or `null` if the transaction has not been executed.

### Example
Request
```
curl -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"state_getTransactionProof","params":["0xe883f3919c4072e459642a012cf7737cee311da0ade88aa14780484797784cd9"]}' \
     http://127.0.0.1:26866/rpc
```
Response
```
{"jsonrpc":"2.0","id":1,"result":{"txRecord":"0x00","txProof":"0x00"}}
```
Or if transaction has not been executed:
```
{"jsonrpc":"2.0","id":1,"result":null}
```

## state_getBlock
Returns the raw CBOR encoded block for the given round number.

### Parameters
1. *roundNumber* (string) - Round number to get the block for.
```
params: [
  "1",
]
```

### Returns
* *block* (string) - Hex encoded raw block.

### Example
Request
```
curl -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"state_getBlock","params":["1"]}' \
     http://127.0.0.1:26866/rpc
```
Response
```
{"jsonrpc":"2.0","id":1,"result":"0x838401f6783531365569753248416d4d4772674a6344714d4a4d6d416d4c6e75674e523870417a7037685231564c6e3750656b386650676556384158201aaa0b033a7d1314808ca1e1cae099170e54c9213fe50f930e8e62d41a3b78198083865820285a974d34c659e5496eb6519a08d6c993940455270ecad436c507e41d541e165820285a974d34c659e5496eb6519a08d6c993940455270ecad436c507e41d541e1658200000000000000000000000000000000000000000000000000000000000000000481bc16d674ec800000100830183a2634b65794400000001644861736858209f58172c2c3ddbca77117a1c0a40f3956e581bf757ff79898a81fd232d5b20eba2634b65794400000001644861736858200c51cc070dc0f1435d11c5d1409e6624e9c051dd251bfcf3aea31c1351f3181ba2634b6579440000000264486173685820c101cd63244a5980629072407e1f24a4cdeb39cd8d61d60ca3f9d575c08a48275820ed9480bbc6a1582a45ea019dd33b086bb85dd2e184d676fb9787671730af086285061a65fb57dd5820f9d294f93663291d4a21edcd8cfc97fc9a2bac1130f7cbb561f26a22819055195820f3b893a2c762c3314c3ad1cc56ec6b9fd1d5814d7871014588141c84ba676a478382783531365569753248416d4a46653731316a736f704a314363474e413665635a754c656347446f6a65446938454470656e634a3448557958416ad3781214d3474a76d47c4ceea9951ad046b38606a668a06a243136b15427bd1b778b492215a152a3a3891058e4290d69faaa60fee4dde26dc0eb842d6ef6360182783531365569753248416d4b47383570565638594c626f63396a637141597461357939444d684233706a57416e425279314c4c767445435841975096df4df3d4bbf27ebcf4a415895a8cc2b5321430e0df0f04026d5182a1c5177b942adfb9c39a6dcf239e209f573bbc40ee7c4d0ad46b917e2f749a74a1cb0182783531365569753248416d4b6d5a313969586374636e703237444c42616e654857466b364452314a55627a6f3434527a716237677143655841260fbde3233d9eb0c20a666fd92efcfbbbdecf56e25e2cacee130011b2a604c750bfa1f96e4dd6ee541e77f6bf9ba0b580837b05ef7e68c9d614421a4829d5e500"}
```
