# Partition Description Record

Partition genesis commands (`money-genesis`, `tokens-genesis`,...)
require `partition-description` argument which is file from where the
Partition Description Record of the partition is read.

The file is JSON encoded description of partition configuration with following fields:

Field              | Type      | Description
-------------------|-----------|---
network_identifier | uint16    | partition ID.
system_identifier  | uint      | partition ID.
type_id_length     | uint      | unit type identifier length in bits.
unit_id_length     | uint      | unit identifier length in bits.
sharding_scheme    | []shardID | list of shard identifiers. Empty (`null` or omit the field) for single shard partition.
t2timeout          | uint      | partition T2 timeout, nanoseconds.
fee_credit_bill    |           | fee credit info.

## Example

Example of the money partition description
```json
{
  "network_identifier": 3,
  "system_identifier": 1,
  "type_id_length": 8,
  "unit_id_length": 256,
  "sharding_scheme": null,
  "t2timeout": 2500000000,
  "fee_credit_bill": {
    "unit_id": "0x000000000000000000000000000000000000000000000000000000000000000201",
    "owner_predicate": "0x83004101f6"
  }
}
```