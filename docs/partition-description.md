# Partition Description Record

Partition genesis commands (`money-genesis`, `tokens-genesis`,...)
require `partition-description` argument which is file from where the
Partition Description Record of the partition is read.

The file is JSON encoded description of partition configuration with following fields:

Field               | Type      | Description
--------------------|-----------|---
networkId           | uint16    | Network identifier.
partitionId         | uint      | Partition identifier.
partitionTypeId     | uint      | Partition type identifier.
typeIdLength        | uint      | Unit type identifier length in bits.
unitIdLength        | uint      | Unit identifier length in bits.
shardingScheme      | []shardID | List of shard identifiers. Empty (`null` or omit the field) for single shard partition.
t2timeout           | uint      | Partition T2 timeout, nanoseconds.
feeCreditBill       |           | Fee credit info.

## Example

Example of the money partition description
```json
{
  "networkId": 3,
  "partitionId": 1,
  "partitionTypeId": 1,
  "typeIdLength": 8,
  "unitIdLength": 256,
  "shardingScheme": null,
  "t2timeout": 2500000000,
  "feeCreditBill": {
    "unitId": "0x000000000000000000000000000000000000000000000000000000000000000201",
    "ownerPredicate": "0x83004101f6"
  }
}
```
