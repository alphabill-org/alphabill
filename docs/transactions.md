# Raw Transaction Format

- [*TransactionOrder*](#transactionorder)
  - [*Payload*](#payload)
    - [*ClientMetadata*](#clientmetadata)
  - [Transaction Types](#transaction-types)
    - [Money Partition](#money-partition)
      - [Transfer Bill](#transfer-bill)
      - [Split Bill](#split-bill)
      - [Transfer Bill to Dust Collector](#transfer-bill-to-dust-collector)
      - [Swap Bills With Dust Collector](#swap-bills-with-dust-collector)
      - [Transfer to Fee Credit](#transfer-to-fee-credit)
      - [Add Fee Credit](#add-fee-credit)
      - [Close Fee Credit](#close-fee-credit)
      - [Reclaim Fee Credit](#reclaim-fee-credit)
    - [Tokens Partition](#tokens-partition)
      - [Create Non-fungible Token Type](#create-non-fungible-token-type)
      - [Create Non-fungible Token](#create-non-fungible-token)
      - [Transfer Non-fungible Token](#transfer-non-fungible-token)
      - [Update Non-fungible Token](#update-non-fungible-token)
      - [Create Fungible Token Type](#create-fungible-token-type)
      - [Create Fungible Token](#create-fungible-token)
      - [Transfer Fungible Token](#transfer-fungible-token)
      - [Split Fungible Token](#split-fungible-token)
      - [Burn Fungible Token](#burn-fungible-token)
      - [Join Fungible Tokens](#join-fungible-tokens)
      - [Add Fee Credit](#add-fee-credit-1)
      - [Close Fee Credit](#close-fee-credit-1)
- [Examples](#examples)
  - [Transfer Bill](#transfer-bill-1)
  - [Split Bill](#split-bill-1)
- [TODO](#todo)
- [References](#references)

## *TransactionOrder*

Alphabill transactions are encoded using a deterministic CBOR data
format[^1]. The top level data item is *TransactionOrder*, which
instructs Alphabill to execute a transaction with a
unit. *TransactionOrder* is always encoded as an array of 3 data items
in the exact order: *Payload*, *OwnerProof* and *FeeProof*. Using the
CBOR Extended Diagnostic Notation[^2] and omitting the subcontent of
these array items, the top level array can be expressed as:

```
/TransactionOrder/ [
    /Payload/    [/omitted/],
    /OwnerProof/ h'',
    /FeeProof/   h''
]
```

Data items in the top level array:

1. *Payload* (array) is described in section [*Payload*](#payload).

2. *OwnerProof* (byte string) contains the arguments to satisfy the
owner condition of the unit specified by *UnitID* in the
*Payload*. The most common example of *OwnerProof* is a digital
signature signing the CBOR encoded *Payload*.

3. *FeeProof* (byte string) contains the arguments to satisfy the
owner condition of the fee credit record specified by
*FeeCreditRecordID* in the *ClientMetadata* of the *Payload*. The most
common example of *FeeProof* is a digital signature signing the CBOR
encoded *Payload*. *FeeProof* can be set to ```null``` (CBOR simple
value 22) in case *OwnerProof* also satisfies the owner condition of
the fee credit record.

### *Payload*

*Payload* is an array of data items which is usually covered by
signature and consists of the following (with example values):

```
/Payload/ [
    /SystemIdentifier/ h'00000000',
    /Type/             "trans",
    /UnitID/           h'0000000000000000000000000000000000000000000000000000000000000001',
    /Attributes/       [/omitted, Type dependent/],
    /ClientMetadata/   [/omitted/]
]
```

Data items in the *Payload* array:

1. *SystemIdentifier* (byte string) is a 4-byte identifier of the
transaction system/partition that is supposed to execute the
transaction. *SystemIdentifier*s currently in use:

    - *h'00000000'* - money partition
    - *h'00000002'* - tokens partition 

2. *Type* (text string) is the type of the transaction. See section
[Transaction Types](#transaction-types) for the list of supported values and
their corresponding *Attributes*.

3. *UnitID* (byte string) specifies the unit involved in the
   transaction.

4. *Attributes* (array) is an array of transaction attributes that
depends on the transaction type and are described in section
[Transaction Types](#transaction-types).

5. *ClientMetadata* (array) is described in section
[*ClientMetadata*](#clientmetadata).

#### *ClientMetadata*

*ClientMetadata* is an array of data items that set the conditions for
the execution of the transaction. It consists of the following (with
example values):

```
/ClientMetadata/ [
    /Timeout/           1344,
    /MaxTransactionFee/ 1,
    /FeeCreditRecordID/ h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA5'
]
```

Data items in the *ClientMetadata* array:

1. *Timeout* (unsigned integer) is the highest block number that this
   transaction can be executed in.

2. *MaxTransactionFee* (unsigned integer) is the maximum
   fee the user is willing to pay for the execution of this
   transaction.

3. *FeeCreditRecordID* (byte string) is an optional identifier of the
   fee credit record used to pay for the execution of this
   transaction.

### Transaction Types

Each partition defines its own transaction types
(*TransactionOrder*.*Payload*.*Type*) and the corresponding
transaction attributes (*TransactionOrder*.*Payload*.*Attributes*).

> TODO: Talk a bit more about partitions and their parameters like
> hash algorithm and unit types. Currently the general part talks about units,
> but money partition talks about bills.

> TODO: explain backlinks here and not under every transaction type

#### Money Partition

Money partiton has two types of units: bills and fee credit records.

##### Transfer Bill

This transaction transfers a bill to a new owner. The value of the
transferred bill is unchanged.

*TransactionOrder*.*Payload*.*Type* = "trans"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transAttributes/ [
    /TargetOwner/ h'5376A8014F01411DBB429D60228CACDEA90D4B5F1C0B022D7F03D9CB9E5042BE3F74B7A8C23A8769AC01',
    /TargetValue/ 999999800099999996,
    /Backlink/    h'F4C65D760DA53F0F6D43E06EAD2AA6095CCF702A751286FA97EC958AFA085839'
]
```

1. *TargetOwner* (byte string) is the new owner condition of the bill.
2. *TargetValue* (unsigned integer) must be equal to the value of the
   bill. The reason for including the value of the bill in the
   transaction order is to enable the recipient of the transaction to
   learn the received amount without having to look up the bill.
3. *Backlink* (byte string) is the SHA-256 hash of the previous
   transaction with the bill. The hash is calculated over the raw CBOR
   encoded bytes of the *TransactionOrder* data item.

##### Split Bill

This transaction splits a bill in two, creating a new bill with its
own owner condition (*TargetOwner*) and a value (*TargetValue*). The
value of the bill being split is reduced by the value of the new bill
and is stored in the *RemainingValue* attribute. The sum of values
specified by *TargetValue* and *RemainingValue* attributes must be
equal to the value of the bill before the split.

*TransactionOrder*.*Payload*.*Type* = "split"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/splitAttributes/ [
    /TargetValue/    99900000000,
    /TargetOwner/    h'5376A8014F0162C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D8769AC01',
    /RemainingValue/ 999999899999999996,
    /Backlink/       h'2C8E1F55FC20A44687AB5D18D11F5E3544D2989DFFBB8250AA6EBA5EF4CEC319'
]
```

1. *TargetValue* (unsigned integer) is the value of the new bill.
2. *TargetOwner* (byte string) is the owner condition of the new bill.
3. *RemainingValue* (unsigned integer) is the remaining value of the
   bill being split.
   > TODO: Why is this needed? The recipient of this transaction does not need to know the *RemainingValue* as is the case with *TargetValue* in Transfer Bill transaction. Also, Transfer to Fee Credit has no such field (and can't have), although it is very similar to a split.)
4. *Backlink* (byte string) is the hash of the previous transaction
   with the bill being split. The hash is calculated over raw CBOR
   encoded bytes of the *TransactionOrder* data item.

##### Transfer Bill to Dust Collector

This transaction transfers a bill to a special owner - Dust Collector
(DC). After transferring multiple bills to DC, a single, larger-value
bill can be obtained from DC with the [Swap Bills With Dust
Collector](#swap-bills-with-dust-collector) transaction. The set of
bills being swapped needs to be decided beforehand, as the *UnitID* of
the new bill obtained from DC is calculated from the *UnitID*s of the
bills being swapped, and specified in the *Nonce* attribute of this
transaction.

Dust is not defined, any bills can be sent to DC and swapped for a
larger-value bill.

*TransactionOrder*.*Payload*.*Type* = "transDC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transDCAttributes/ [
    /Nonce/       h'',
    /TargetOwner/ h'',
    /TargetValue/ 999999899999999996,
    /Backlink/    h'2C8E1F55FC20A44687AB5D18D11F5E3544D2989DFFBB8250AA6EBA5EF4CEC319'
]
```

1. *Nonce* (byte string) is the *UnitID* of the new bill to be
   obtained from DC with the [Swap Bills With Dust
   Collector](#swap-bills-with-dust-collector) transaction. Calculated
   as the hash of concatenated *UnitID*s of the bills being swapped,
   sorted in ascending order.
2. *TargetOwner* (byte string) is the owner condition of the new bill
   later obtained with the [Swap Bills With Dust
   Collector](#swap-bills-with-dust-collector) transaction.
3. *TargetValue* (unsigned integer) is the value of the bill
   transferred to DC with this transaction.
4. *Backlink* (byte string) is the hash of the previous transaction
   with the bill transferred to DC with this transaction. The hash is
   calculated over raw CBOR encoded bytes of the *TransactionOrder*
   data item.

##### Swap Bills With Dust Collector

> TODO

*TransactionOrder*.*Payload*.*Type* = "swapDC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/swapDCAttributes/ [
    /TargetOwner/     h'',
    /BillIdentifiers/ [/TODO/],
    /DcTransfers/     [/TODO/],
    /Proofs/          [/TODO/],
    /TargetValue/     3
]
```

1. *TargetOwner* (byte string) is the owner condition of the new bill.
2. *BillIdentifiers* (array of byte strings) is a variable length array
   of bill identifiers that are being swapped.
3. *DcTransfers* (array of TransactionRecords) is TODO should be opaque byte string
4. *Proofs* (array of TxProofs) is TODO does API return array? can client handle it as opaque byte string?
5. *TargetValue* (unsigned integer) is the value of the new bill. 

##### Transfer to Fee Credit

This transaction reserves money on the money partition to be paid as
fees on a target partition. Money partition can also be the target
partition. If the sum of the transferred amount and transaction fee is
less than the value of the bill, it is similar to the [Split
Bill](#split-bill) transaction in that both reduce the value of an
existing bill and transfer it to a new owner. In this case, the new
owner is a special fee credit record, managed by the target
partition. In case the remaining value of the bill is 0, the bill is
deleted.

To bootstrap a fee credit record on the money partition, the fees for
this transaction are handled outside the fee credit system. That is,
the value of the bill used to make this transfer is reduced by the
*Amount* transferred, and by the amount paid in fees for this
transaction.

Note that an [Add Fee Credit](#add-fee-credit) transaction
needs to be executed on the target partition after each [Transfer to
Fee Credit](#transfer-to-fee-credit) transaction. If any other
transaction is executed with the bill between those two, the value
transferred to fee credit is lost.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "transFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transFCAttributes/ [
    /Amount/                 100000000,
    /TargetSystemIdentifier/ h'00000002',
    /TargetRecordID/         h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA5',
    /EarliestAdditionTime/   13,
    /LatestAdditionTime/     23,
    /Nonce/                  null,
    /Backlink/               h'52F43127F58992B6FCFA27A64C980E70D26C2CDE0281AC93435D10EB8034B695'
]
```

1. *Amount* (unsigned integer) is the amount of money to reserve for
   paying fees in the target partition. Has to be less than the value
   of the bill +
   *TransactionOrder*.*Payload*.*ClientMetadata*.*MaxTransactionFee*.
2. *TargetSystemIdentifier* (byte string) is the system identifier of
   the target partition where the amount can be spent on fees.
3. *TargetRecordID* (byte string) is the target fee credit record
   identifier (*FeeCreditRecordID* of the corresponding [Add Fee
   Credit](#add-fee-credit) transaction). Hash of the private key
   making the transaction is used.
4. *EarliestAdditionTime* (unsigned integer) is the earliest round
   when the corresponding [Add Fee Credit](#add-fee-credit)
   transaction can be executed in the target system (usually current
   round number).
5. *LatestAdditionTime* (unsigned integer) is the latest round when
   the corresponding [Add Fee Credit](#add-fee-credit) transaction can
   be executed in the target system (usually current round number +
   some timeout).
6. *Nonce* (byte string) is the hash of the last [Add Fee
   Credit](#add-fee-credit) transaction executed for the
   *TargetRecordID* in the target partition, or `null` if it does not
   exist yet.
7. *Backlink* (byte string) is the backlink to the previous
   transaction with this bill (the bill being transferred to fee
   credit).

##### Add Fee Credit

This transaction creates or updates a fee credit record on the target
partition (the partition this transaction is executed on), by
presenting a proof of fee credit, reserved in the money partition with
[Transfer to Fee Credit](#transfer-to-fee-credit) transaction. As a
result, execution of other fee paying transactions becomes possible.

The fee for this transaction will also be paid from the fee credit
record being created/updated.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "addFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/addFCAttributes/ [
    /FeeCreditOwnerCondition/ h'5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01',
    /FeeCreditTransfer/       [/omitted/],
    /FeeCreditTransferProof/  [/omitted/]
]
```

1. *FeeCreditOwnerCondition* (byte string, optional) is the owner
   condition for the created fee credit record. It needs to be
   satisifed by the *TransactionOrder*.*FeeProof* data item of the
   transactions using the record to pay fees.
   > TODO: What if fee credit record is being updated? Is it allowed? Needs to have same value as before?
2. *FeeCreditTransfer* (array) is the transaction record of the
   [Transfer To Fee Credit](#transfer-to-fee-credit) transaction. It
   is constructed by a node when the transaction is executed and
   included in a block. Necessary for the target partition to verify
   the amount reserved as fee credit in the money partiton.
3. *FeeCreditTransferProof* (array) is the proof of execution of the
    transaction provided in *FeeCreditTransfer* attribute. Necessary
    for the target partition to verify the amount reserved as fee
    credit in the money partiton.

> TODO: change the types of *FeeCreditTransfer* and *FeeCreditTransferProof* to byte string.

##### Close Fee Credit

This transaction closes a fee credit record and makes it possible to
reclaim the money with the [Reclaim Fee Credit](#reclaim-fee-credit)
transaction on the money partition.

Note that fee credit records cannot be closed partially. 

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "closeFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/closeFCAttributes/ [
    /Amount/       100000000,
    /TargetUnitID/ h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA5',
    /Nonce/        null
]
```

1. *Amount* (unsigned integer) is the current balance of the fee
   credit record.
2. *FeeCreditRecordID* (byte string) is the identifier of the fee
   credit record to be closed.
3. *Nonce* (byte string) is the hash of the last transaction executed
   with the bill in money partition that will be used to reclaim the
   fee credit.

##### Reclaim Fee Credit

> TODO

*TransactionOrder*.*Payload*.*Type* = "reclFC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/reclFCAttributes/ [
    /CloseFeeCreditTransfer/ [/TransactionRecord/],
    /CloseFeeCreditProof/    [/TransactionProof/],
    /Backlink/               h''
]
```

1. *CloseFeeCreditTransfer* - bill transfer record of type "close fee credit" (based on closed bill GET /proof)
2. *CloseFeeCreditProof* - transaction proof of "close fee credit" transaction (based on closed bill GET /proof)
3. *Backlink* - hash of this unit's previous transacton (bill backlink that will be changed in your money account)

#### Tokens Partition

##### Create Non-fungible Token Type

*TransactionOrder*.*Payload*.*Type* = "createNType"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createNTypeAttributes/ [
    /Symbol/                             "symbol",
    /Name/                               "long name",
    /Icon/                               ["image/png", h''],
    /ParentTypeID/                       h'00',
    /SubTypeCreationPredicate/           h'535101',
    /TokenCreationPredicate/             h'5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01',
    /InvariantPredicate/                 h'535101',
    /DataUpdatePredicate/                h'535101',
    /SubTypeCreationPredicateSignatures/ [h'53']
]
```

1. Symbol - the symbol (short name) of this token type; note that the symbols are not guaranteed to be unique;
2. Name - the long name of this token type;
3. Icon - the icon of this token type
    1. Type - the MIME content type identifying an image format
    2. Data - the image in the format specified by type
4. ParentTypeID - identifies the parent type that this type derives from; 0 indicates there is no parent type;
5. SubTypeCreationPredicate - the predicate clause that controls defining new sub-types of this type;
6. TokenCreationPredicate - the predicate clause that controls creating new tokens of this type
7. InvariantPredicate - the invariant predicate clause that all tokens of this type (and of sub- types of this type) inherit into their owner condition;
8. DataUpdatePredicate - the clause that all tokens of this type (and of sub-types of this type) inherit into their data update predicates
9. SubTypeCreationPredicateSignatures - inputs to satisfy the sub-type creation predicates of all parents.

##### Create Non-fungible Token

*TransactionOrder*.*Payload*.*Type* = "createNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createNTokenAttributes/ [
    /OwnerCondition/                     h'',
    /NFTTypeID/                          h'',
    /Name/                               "",
    /URI/                                "",
    /Data/                               h'',
    /DataUpdatePredicate/                h'',
    /TokenCreationPredicateSignatures/   [h'']
]
```

1. OwnerCondition - the initial owner condition of the new token
2. NFTTypeID - identifies the type of the new token
3. Name - the name of the new token
4. URI - the optional URI of an external resource associated with the new token
5. Data - the optional data associated with the new token
6. DataUpdatePredicate - the data update predicate of the new token
7. TokenCreationPredicateSignatures - inputs to satisfy the token creation predicates of all parent types

##### Transfer Non-fungible Token

*TransactionOrder*.*Payload*.*Type* = "transNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transNTokenAttributes/ [
    /NewOwnerCondition/            h'',
    /Nonce/                        h'',
    /Backlink/                     h'',
    /NFTTypeID/                    h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. NewOwnerCondition - the owner condition of the token
2. Nonce - optional nonce
3. Backlink - the backlink to the previous transaction with the token
4. NFTTypeID - identifies the type of the token
5. InvariantPredicateSignatures - inputs to satisfy the token type invariant predicates down the inheritance chain

##### Update Non-fungible Token

*TransactionOrder*.*Payload*.*Type* = "updateNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/updateNTokenAttributes/ [
    /Data/                 h'',
    /Backlink/             h'',
    /DataUpdateSignatures/ [h'']
]
```

1. Data - the new data to replace the data currently associated with the token
2. Backlink - the backlink to the previous transaction with the token
3. DataUpdateSignatures - inputs to satisfy the token data update predicates down the inheritance chain

##### Create Fungible Token Type

*TransactionOrder*.*Payload*.*Type* = "createFType"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createFTypeAttributes/ [
    /Symbol/                             "symbol",
    /Name/                               "long name",
    /Icon/                               ["image/png", h''],
    /ParentTypeID/                       h'00',
    /DecimalPlaces/                      8,
    /SubTypeCreationPredicate/           h'535101',
    /TokenCreationPredicate/             h'5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01',
    /InvariantPredicate/                 h'535101',
    /SubTypeCreationPredicateSignatures/ [h'53']
]
```

1. Symbol - the symbol (short name) of this token type; note that the symbols are not guaranteed to be unique
2. Name - the long name of this token type
3. Icon - the icon of this token type
4. ParentTypeID - identifies the parent type that this type derives from; 0 indicates there is no parent type
5. DecimalPlaces - the number of decimal places to display for values of tokens of the new type
6. SubTypeCreationPredicate - the predicate clause that controls defining new sub-types of this type;
7. TokenCreationPredicate - the predicate clause that controls creating new tokens of this type
8. InvariantPredicate - the invariant predicate clause that all tokens of this type (and of sub- types of this type) inherit into their owner condition;
9. SubTypeCreationPredicateSignatures - inputs to satisfy the sub-type creation predicates of all parents.

##### Create Fungible Token

*TransactionOrder*.*Payload*.*Type* = "createFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createFTokenAttributes/ [
    /OwnerCondition/                     h'',
    /TypeID/                             h'',
    /Value/                              1000,
    /TokenCreationPredicateSignatures/   [h'']
]
```

1. OwnerCondition - the initial owner condition of the new token
2. TypeID - identifies the type of the new token
3. Value  - the value of the new token
4. TokenCreationPredicateSignatures - inputs to satisfy the token creation predicates of all parent types

##### Transfer Fungible Token

*TransactionOrder*.*Payload*.*Type* = "transFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transFTokenAttributes/ [
    /NewBearer/                    h'',
    /Value/                        5,
    /Nonce/                        h'',
    /Backlink/                     h'',
    /TypeID/                       h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. NewBearer - the initial bearer predicate of the new token
2. Value - the value to transfer
3. Nonce - optional nonce
4. Backlink - the backlink to the previous transaction with this token
5. TypeID - identifies the type of the token;
6. InvariantPredicateSignatures - inputs to satisfy the token type invariant predicates down the inheritance chain

##### Split Fungible Token

*TransactionOrder*.*Payload*.*Type* = "splitFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/splitFTokenAttributes/ [
    /NewBearer/                    h'00',
    /TargetValue/                  600,
    /Nonce/                        h'',
    /Backlink/                     h'',
    /TypeID/                       h'',
    /RemainingValue/               400,
    /InvariantPredicateSignatures/ [h'53']
]
```

1. NewBearer - the bearer predicate of the new token;
2. TargetValue - the value of the new token
3. Nonce - optional nonce
4. Backlink - the backlink to the previous transaction with this token
5. TypeID - identifies the type of the token;
6. RemainingValue - new value of the source token
7. InvariantPredicateSignatures - inputs to satisfy the token type invariant predicates down the inheritance chain

##### Burn Fungible Token

*TransactionOrder*.*Payload*.*Type* = "burnFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/burnFTokenAttributes/ [
    /TypeID/                       h'',
    /Value/                        999,
    /Nonce/                        h'',
    /Backlink/                     h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. TypeID - identifies the type of the token to burn;
2. Value - the value to burn
3. Nonce - optional nonce
4. Backlink - the backlink to the previous transaction with this token
5. InvariantPredicateSignatures - inputs to satisfy the token type invariant predicates down the inheritance chain

##### Join Fungible Tokens

*TransactionOrder*.*Payload*.*Type* = "joinFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/joinFTokenAttributes/ [
    /BurnTransactions/             [/TODO/],
    /Proofs/                       [/TODO/],
    /Backlink/                     h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. BurnTransactions - the transactions that burned the source tokens
2. Proofs - block proofs for burn transactions
3. Backlink - the backlink to the previous transaction with this token
4. InvariantPredicateSignatures - inputs to satisfy the token type invariant predicates down the inheritance chain

##### Add Fee Credit

Same as the [Add Fee Credit](#add-fee-credit) transaction in the money
partition.

##### Close Fee Credit

Same as the [Close Fee Credit](#close-fee-credit) transaction in the
money partition.

## Examples

The raw hex encoded transactions in these examples can be inspected
with online CBOR decoders[^3][^4]. The same tools can also encode the
Extended Diagnostic Notation to raw hex encoded CBOR format.

### Transfer Bill

Raw hex encoded transaction:
```
83854400000000657472616e735820000000000000000000000000000000000000000000000000000000000000000183582a5376a8014f01411dbb429d60228cacdea90d4b5f1c0b022d7f03d9cb9e5042be3f74b7a8c23a8769ac011b0de0b6851c6c10fc5820f4c65d760da53f0f6d43e06ead2aa6095ccf702a751286fa97ec958afa08583983190540015820a0227ac5202427db551b8abe08645378347a3c5f70e0e5734f147ad45cbc1ba558675354016860a15d094e3ec42c133cb090682c28252b0c6299ad267255f6fb06832107b26286ea6096a0a6b1aaafbd73a748f7e7b1cf6e3ef3db3091925c236ba3c94ff30055010227e874b800ca319b7bc384dfc63f915e098417b549edb43525aee511a6dbbd9cf6
```

Extended Diagnostic Notation with annotations:
```
/TransactionOrder/ [
    /Payload/ [
        /SystemIdentifier/ h'00000000',
        /Type/             "trans",
        /UnitID/           h'0000000000000000000000000000000000000000000000000000000000000001',
        /transAttributes/ [
            /TargetOwner/ h'5376A8014F01411DBB429D60228CACDEA90D4B5F1C0B022D7F03D9CB9E5042BE3F74B7A8C23A8769AC01',
            /TargetValue/ 999999800099999996,
            /Backlink/    h'F4C65D760DA53F0F6D43E06EAD2AA6095CCF702A751286FA97EC958AFA085839'
        ],
        /ClientMetadata/ [
            /Timeout/           1344,
            /MaxTransactionFee/ 1,
            /FeeCreditRecordID/ h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA5'
        ]
    ],
    /OwnerProof/ h'5354016860A15D094E3EC42C133CB090682C28252B0C6299AD267255F6FB06832107B26286EA6096A0A6B1AAAFBD73A748F7E7B1CF6E3EF3DB3091925C236BA3C94FF30055010227E874B800CA319B7BC384DFC63F915E098417B549EDB43525AEE511A6DBBD9C',
    /FeeProof/   null
]
```

[
    [
        h'00000000',
        "split",
        h'0000000000000000000000000000000000000000000000000000000000000001', [500000000, h'5376A8014F01411DBB429D60228CACDEA90D4B5F1C0B022D7F03D9CB9E5042BE3F74B7A8C23A8769AC01', 999999999399999996, h'52F43127F58992B6FCFA27A64C980E70D26C2CDE0281AC93435D10EB8034B695'], [23, 1, h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA5']], h'53540184A1455D7D3A3F4E7EC9B0CAA67C416874EC2F0B5116BEAD0F5DA1094E0B50D212CA66B0DE771F4E3F9B5B5DA307FD2C856D673FD88EE3509FD7C84CC3E380300155010227E874B800CA319B7BC384DFC63F915E098417B549EDB43525AEE511A6DBBD9C', null]

### Split Bill

Raw hex encoded transaction:
```
838544000000006573706c697458200000000000000000000000000000000000000000000000000000000000000001841b0000001742810700582a5376a8014f0162c5594a5f1e83d7c5611b041999cfb9c67cff2482aefa72e8b636ccf79bbb1d8769ac011b0de0b69c5eed17fc58202c8e1f55fc20a44687ab5d18d11f5e3544d2989dffbb8250aa6eba5ef4cec319831889015820a0227ac5202427db551b8abe08645378347a3c5f70e0e5734f147ad45cbc1ba558675354019e5d3927a7dce3655105277fffe6602d6d9ef2456346c379a702847356b4f7dd6b987a26434b925d66361f396fc0dc927192a48cd394f590bc3cd3bc96a0e0b50155010227e874b800ca319b7bc384dfc63f915e098417b549edb43525aee511a6dbbd9cf6
```

Extended Diagnostic Notation with annotations:
```
/TransactionOrder/ [
    /Payload/ [
        /SystemIdentifier/ h'00000000',
        /Type/             "split",
        /UnitID/           h'0000000000000000000000000000000000000000000000000000000000000001',
        /splitAttributes/ [
            /Amount/         99900000000,
            /TargetBearer/   h'5376A8014F0162C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D8769AC01',
            /RemainingValue/ 999999899999999996,
            /Backlink/       h'2C8E1F55FC20A44687AB5D18D11F5E3544D2989DFFBB8250AA6EBA5EF4CEC319'
        ],
        /ClientMetadata/ [
            /Timeout/           137,
            /MaxTransactionFee/ 1,
            /FeeCreditRecordID/ h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA5'
        ]
    ],
    /OwnerProof/ h'5354019E5D3927A7DCE3655105277FFFE6602D6D9EF2456346C379A702847356B4F7DD6B987A26434B925D66361F396FC0DC927192A48CD394F590BC3CD3BC96A0E0B50155010227E874B800CA319B7BC384DFC63F915E098417B549EDB43525AEE511A6DBBD9C',
    /FeeProof/   null
]
```

## TODO

- Change the order of data items in *Payload* to *SystemIdentifier*, *UnitID*, *Type*, *Attributes*, *ClientMetadata*.
- Keep TargetOwner and TargetValue attributes in the same order for all transaction types.
- Change tokens partition *SystemIdentifier* now that h'00000001' is unused after removing VD partition?
- Add a version field to *Payload* (or *Attributes*/*TransactionOrder*)?
- Specify optionality of data items. OwnerProof/FeeProof is required for some transactions, optional or prohibited for others.
- Some data items that we call 'Nonce' are not actually nonces. Rename.
- Mention libraries used for CBOR encoding/decoding, especially those used by validators.
  * BE: https://github.com/fxamacker/cbor
  * FE: https://github.com/kriszyp/cbor-x
- Describe dependencies between transactions, which transactions need to be executed in which order to get some job done.
- More examples, like *transFC* and *addFC*.

## References

[^1]: https://www.rfc-editor.org/rfc/rfc8949
[^2]: https://www.rfc-editor.org/rfc/rfc8610#appendix-G
[^3]: https://cbor.me/
[^4]: https://cbor.nemo157.com/
