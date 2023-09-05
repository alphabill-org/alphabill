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
  - [Split Bill](#split-bill-1)
  - [Transfer Bill](#transfer-bill-1)
- [References](#references)

## *TransactionOrder*

Alphabill transactions are encoded using a deterministic CBOR data
format[^1]. The top-level data item is *TransactionOrder*, which
instructs Alphabill to execute a transaction with a
unit. *TransactionOrder* is always encoded as an array of 3 data items
in the exact order: *Payload*, *OwnerProof* and *FeeProof*. Using the
CBOR Extended Diagnostic Notation[^2] and omitting the subcontent of
these array items, the top-level array can be expressed as:

```
/TransactionOrder/ [
    /Payload/    [/omitted/],
    /OwnerProof/ h'',
    /FeeProof/   h''
]
```

Data items in the top-level array:

1. *Payload* (array) is described in section [*Payload*](#payload).

2. *OwnerProof* (byte string) contains the arguments to satisfy the
owner condition of the unit specified by *Payload*.*UnitID*. The most
common example of *OwnerProof* is a digital signature signing the CBOR
encoded *Payload*.

3. *FeeProof* (byte string) contains the arguments to satisfy the
owner condition of the fee credit record specified by
*Payload*.*ClientMetadata*.*FeeCreditRecordID*. The most common
example of *FeeProof* is a digital signature signing the CBOR encoded
*Payload*. *FeeProof* can be set to ```null``` (CBOR simple value 22)
in case *OwnerProof* also satisfies the owner condition of the fee
credit record.

### *Payload*

*Payload* is an array of data items which is usually covered by
signature and consists of the following (with example values):

```
/Payload/ [
    /SystemIdentifier/ h'00000000',
    /Type/             "trans",
    /UnitID/           h'000000000000000000000000000000000000000000000000000000000000000100',
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

3. *UnitID* (byte string) uniquely identifies the unit involved in the
   transaction. Partitions can have different types of units and each
   *UnitID* consists of two concatenated parts: the unit part and the
   type part. The length of each part is constant within a partition
   and thus the overall length of *UnitID* is also constant within a
   partition.

4. *Attributes* (array) is an array of transaction attributes that
depends on the transaction type and are described in section
[Transaction Types](#transaction-types).

5. *ClientMetadata* (array) is described in section
[*ClientMetadata*](#clientmetadata).

#### *ClientMetadata*

*ClientMetadata* is an array of data items that sets the conditions
for the execution of the transaction. It consists of the following
(with example values):

```
/ClientMetadata/ [
    /Timeout/           1344,
    /MaxTransactionFee/ 1,
    /FeeCreditRecordID/ h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA50F'
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
   transaction. Fee credit records are created with [Transfer to Fee
   Credit](#transfer-to-fee-credit) and [Add Fee
   Credit](#add-fee-credit) transactions.

### Transaction Types

Each partition defines its own unit types. For each unit type, a set
of valid transaction types is defined. And for each transaction type,
an array of valid attributes is defined.

A common attribute for many transaction types is *Backlink*, which
links a transaction back to the previous transaction with the same
unit and thus makes the order of transactions unambiguous. *Backlink*
is calculated as the hash of the raw CBOR encoded bytes of the
*TransactionOrder* data item. Hash algorithm is defined by each
partition.

#### Money Partition

System identifier: h'00000000'

*UnitID* length: 32 bytes unit part + 1 byte type part

Valid type parts in *UnitID* and the corresponding unit types: 
- *h'00'* - bill
- *h'0f'* - fee credit record

Hash algorithm: SHA-256

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
3. *Backlink* (byte string) is the backlink to the previous
   transaction with the bill.

##### Split Bill

This transaction splits a bill in two, creating a new bill with a new
owner condition (*TargetOwner*) and value (*TargetValue*). The value
of the bill being split is reduced by the value of the new bill and is
specified in the *RemainingValue* attribute. The sum of *TargetValue*
and *RemainingValue* must be equal to the value of the bill before the
split.

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
4. *Backlink* (byte string) is the backlink to the previous
   transaction with the bill being split.

##### Transfer Bill to Dust Collector

This transaction transfers a bill to a special owner - Dust Collector
(DC). After transferring multiple bills to DC, the transferred bills 
can be joined into an existing bill DC with the [Swap Bills With Dust
Collector](#swap-bills-with-dust-collector) transaction. The target bill 
must be chosen beforehand and should not be used between the transactions.

Dust is not defined, any bills can be transferred to DC and joined into
a larger-value bill.

*TransactionOrder*.*Payload*.*Type* = "transDC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transDCAttributes/ [
    /Value/              999999899999999996,
    /TargetUnitID/       h'',
    /TargetUnitBacklink/ h'',
    /Backlink/           h'2C8E1F55FC20A44687AB5D18D11F5E3544D2989DFFBB8250AA6EBA5EF4CEC319'
]
```

1. *Value* (unsigned integer) is the value of the bill
   transferred to DC with this transaction.
2. *TargetUnitID* (byte string) is the *UnitID* of the target bill for the
   [Swap Bills With Dust Collector](#swap-bills-with-dust-collector) 
   transaction.
3. *TargetUnitBacklink* (byte string) is the *Backlink* of the target bill 
   for the [Swap Bills With Dust Collector](#swap-bills-with-dust-collector) 
   transaction.
4. *Backlink* (byte string) is the backlink to the previous transaction
   with the bill.

##### Swap Bills With Dust Collector

This transaction joins the bills previously [transferred to
DC](#transfer-bill-to-dust-collector) into a target bill.

*TransactionOrder*.*Payload*.*Type* = "swapDC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/swapDCAttributes/ [
    /OwnerCondition/   h'',
    /DcTransfers/      [/omitted/],
    /DcTransferProofs/ [/omitted/],
    /TargetValue/      3
]
```

1. *OwnerCondition* (byte string) is the new owner condition of the target bill.
2. *DcTransfers* (array) is an array of [Transfer Bill to Dust
   Collector](#transfer-bill-to-dust-collector) transaction records
   ordered in strictly increasing order of bill identifiers.
3. *DcTransferProofs* (array) is an array of [Transfer Bill to Dust
   Collector](#transfer-bill-to-dust-collector) transaction proofs.
   The order of this array must match the order of *DcTransfers*
   array, so that a transaction and its corresponding proof have the
   same index.
4. *TargetValue* (unsigned integer) is the value added to the target bill 
   and must be equal to the sum of the values of the bills transferred to
   DC for this swap.

##### Transfer to Fee Credit

This transaction reserves money on the money partition to be paid as
fees on the target partition. Money partition can also be the target
partition. A bill can be transferred to fee credit partially.

To bootstrap a fee credit record on the money partition, the fee for
this transaction is handled outside the fee credit system. That is,
the value of the bill used to make this transfer is reduced by the
*Amount* transferred **and** the transaction fee. If the remaining
value is 0, the bill is deleted.

Note that an [Add Fee Credit](#add-fee-credit) transaction must be
executed on the target partition after each [Transfer to Fee
Credit](#transfer-to-fee-credit) transaction, because the *TargetUnitBacklink*
attribute in this transaction contains the backlink to the last [Add
Fee Credit](#add-fee-credit) transaction.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "transFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transFCAttributes/ [
    /Amount/                 100000000,
    /TargetSystemIdentifier/ h'00000002',
    /TargetRecordID/         h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA52F',
    /EarliestAdditionTime/   13,
    /LatestAdditionTime/     23,
    /TargetUnitBacklink/     null,
    /Backlink/               h'52F43127F58992B6FCFA27A64C980E70D26C2CDE0281AC93435D10EB8034B695'
]
```

1. *Amount* (unsigned integer) is the amount of money to reserve for
   paying fees in the target partition. Has to be less than the value
   of the bill +
   *TransactionOrder*.*Payload*.*ClientMetadata*.*MaxTransactionFee*.
2. *TargetSystemIdentifier* (byte string) is the system identifier of
   the target partition where the *Amount* can be spent on fees.
3. *TargetRecordID* (byte string) is the target fee credit record
   identifier (*FeeCreditRecordID* of the corresponding [Add Fee
   Credit](#add-fee-credit) transaction).
4. *EarliestAdditionTime* (unsigned integer) is the earliest round
   when the corresponding [Add Fee Credit](#add-fee-credit)
   transaction can be executed in the target partition (usually
   current round number).
5. *LatestAdditionTime* (unsigned integer) is the latest round when
   the corresponding [Add Fee Credit](#add-fee-credit) transaction can
   be executed in the target partition (usually current round number +
   some timeout).
6. *TargetUnitBacklink* (byte string) is the hash of the last [Add Fee
   Credit](#add-fee-credit) transaction executed for the
   *TargetRecordID* in the target partition, or `null` if it does not
   exist yet.
7. *Backlink* (byte string) is the backlink to the previous
   transaction with the bill.

##### Add Fee Credit

This transaction creates or updates a fee credit record on the target
partition (the partition this transaction is executed on), by
presenting a proof of fee credit reserved in the money partition with
the [Transfer to Fee Credit](#transfer-to-fee-credit) transaction. As
a result, execution of other fee-paying transactions becomes possible.

The fee for this transaction will also be paid from the fee credit
record being created/updated.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "addFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/addFCAttributes/ [
    /TargetOwner/            h'5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01',
    /FeeCreditTransfer/      [/omitted/],
    /FeeCreditTransferProof/ [/omitted/]
]
```

1. *TargetOwner* (byte string, optional) is the owner
   condition for the created fee credit record. It needs to be
   satisfied by the *TransactionOrder*.*FeeProof* data item of the
   transactions using the record to pay fees.
2. *FeeCreditTransfer* (array) is a record of the [Transfer to Fee
   Credit](#transfer-to-fee-credit) transaction. Necessary for the
   target partition to verify the amount reserved as fee credit in the
   money partition.
3. *FeeCreditTransferProof* (array) is the proof of execution of the
    transaction provided in *FeeCreditTransfer* attribute. Necessary
    for the target partition to verify the amount reserved as fee
    credit in the money partition.

##### Close Fee Credit

This transaction closes a fee credit record and makes it possible to
reclaim the money with the [Reclaim Fee Credit](#reclaim-fee-credit)
transaction on the money partition.

Note that fee credit records cannot be closed partially. 

This transaction must be followed by a [Reclaim Fee
Credit](#reclaim-fee-credit) transaction to avoid losing the closed
fee credit. The *TargetUnitBacklink* attribute fixes the current state of the bill
used to reclaim the closed fee credit, and any other transaction with
the bill would invalidate that backlink.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "closeFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/closeFCAttributes/ [
    /Amount/             100000000,
    /TargetUnitID/       h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA500',
    /TargetUnitBacklink/ h''
]
```

1. *Amount* (unsigned integer) is the current balance of the fee
   credit record.
2. *TargetUnitID* (byte string) is the *UnitID* of the existing bill
   in the money partition that is used to reclaim the fee credit.
3. *TargetUnitBacklink* (byte string) is the backlink to the previous
   transaction with the bill in the money partition that is used to
   reclaim the fee credit.

##### Reclaim Fee Credit

This transaction reclaims the fee credit, previously closed with a
[Close Fee Credit](#close-fee-credit) transaction in a target
partition, to an existing bill in the money partition.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "reclFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/reclFCAttributes/ [
    /CloseFeeCredit/      [/TransactionRecord/],
    /CloseFeeCreditProof/ [/TransactionProof/],
    /Backlink/            h''
]
```

1. *CloseFeeCredit* (array) is a record of the [Close Fee
   Credit](#close-fee-credit) transaction. Necessary for the money
   partition to verify the amount closed as fee credit in the target
   partition.
2. *CloseFeeCreditProof* (array) is the proof of execution of the
    transaction provided in *CloseFeeCredit* attribute. Necessary for
    the money partition to verify the amount closed as fee credit in
    the target partition.
3. *Backlink* (byte string) is the backlink to the previous
   transaction with the bill receiving the reclaimed fee credit.

#### Tokens Partition

System identifier: *h'00000002'*

*UnitID* length: 32 bytes unit part + 1 byte type part

Valid type parts in *UnitID* and the corresponding unit types: 
- *h'20'* - fungible token type
- *h'21'* - fungible token
- *h'22'* - non-fungible token type
- *h'23'* - non-fungible token
- *h'2f'* - fee credit record

Hash algorithm: SHA-256

##### Create Non-fungible Token Type

This transaction creates a non-fungible token type.

*TransactionOrder*.*Payload*.*Type* = "createNType"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createNTypeAttributes/ [
    /Symbol/                             "symbol",
    /Name/                               "long name",
    /Icon/                               [/Type/ "image/png", /Data/ h''],
    /ParentTypeID/                       null,
    /SubTypeCreationPredicate/           h'535101',
    /TokenCreationPredicate/             h'5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01',
    /InvariantPredicate/                 h'535101',
    /DataUpdatePredicate/                h'535101',
    /SubTypeCreationPredicateSignatures/ [h'53']
]
```

1. *Symbol* (text string) is the symbol (short name) of this token
   type. Symbols are not guaranteed to be unique.
2. *Name* (text string) is the long name of this token type.
3. *Icon* (array) is the icon of this token type. Consists of two data
   items:
    1. *Type* (text string) is the MIME content type of the image in *Data*.
    2. *Data* (byte string) is the image in the format specified by *Type*.
4. *ParentTypeID* (byte string) is the *UnitID* of the parent type
   that this type derives from. `null` value indicates that there is
   no parent type.
5. *SubTypeCreationPredicate* (byte string) is the predicate clause that
   controls defining new subtypes of this type.
6. *TokenCreationPredicate* (byte string) is the predicate clause that
   controls creating new tokens of this type.
7. *InvariantPredicate* (byte string) is the invariant predicate
   clause that all tokens of this type (and of subtypes) inherit into
   their owner condition.
8. *DataUpdatePredicate* (byte string) is the clause that all tokens
   of this type (and of subtypes) inherit into their data update
   predicates.
9. *SubTypeCreationPredicateSignatures* (array of byte strings) is an
   array of inputs to satisfy the subtype creation predicates of all
   parents.

##### Create Non-fungible Token

This transaction creates a new non-fungible token.

*TransactionOrder*.*Payload*.*Type* = "createNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createNTokenAttributes/ [
    /OwnerCondition/                   h'',
    /TypeID/                           h'',
    /Name/                             "",
    /URI/                              "",
    /Data/                             h'',
    /DataUpdatePredicate/              h'',
    /TokenCreationPredicateSignatures/ [h'']
]
```

1. *OwnerCondition* (byte string) is the initial owner condition of
   the new token.
2. *TypeID* (byte string) is the *UnitID* of the type of the new
   token.
3. *Name* (text string) is the name of the new token.
4. *URI* (text string) is the optional URI of an external resource
   associated with the new token.
5. *Data* (byte string) is the optional data associated with the new
   token.
6. *DataUpdatePredicate* (byte string) is the data update predicate of
   the new token.
7. *TokenCreationPredicateSignatures* (array of byte string) is an
   array of inputs to satisfy the token creation predicates of all
   parent types.

##### Transfer Non-fungible Token

This transaction transfers a non-fungible token to a new owner.

*TransactionOrder*.*Payload*.*Type* = "transNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transNTokenAttributes/ [
    /TargetOwner/                  h'',
    /Nonce/                        h'',
    /Backlink/                     h'',
    /TypeID/                       h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. *TargetOwner* (byte string) is the new owner condition of the
   token.
2. *Nonce* (byte string) is an optional nonce.
3. *Backlink* (byte string) is the backlink to the previous
   transaction with the token.
4. *TypeID* (byte string) is the type of the token.
5. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Update Non-fungible Token

This transaction updates the data of a non-fungible token.

*TransactionOrder*.*Payload*.*Type* = "updateNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/updateNTokenAttributes/ [
    /Data/                 h'',
    /Backlink/             h'',
    /DataUpdateSignatures/ [h'']
]
```

1. *Data* (byte string) is the new data to replace the data currently
   associated with the token.
2. *Backlink* (byte string) is the backlink to the previous transaction
   with the token.
3. *DataUpdateSignatures* (array of byte strings) is an array of inputs
   to satisfy the token data update predicates down the inheritance
   chain.

##### Create Fungible Token Type

This transaction creates a fungible token type.

*TransactionOrder*.*Payload*.*Type* = "createFType"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createFTypeAttributes/ [
    /Symbol/                             "symbol",
    /Name/                               "long name",
    /Icon/                               [/Type/ "image/png", /Data/ h''],
    /ParentTypeID/                       null,
    /DecimalPlaces/                      8,
    /SubTypeCreationPredicate/           h'535101',
    /TokenCreationPredicate/             h'5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01',
    /InvariantPredicate/                 h'535101',
    /SubTypeCreationPredicateSignatures/ [h'53']
]
```

1. *Symbol* (text string) is the symbol (short name) of this token
   type. Symbols are not guaranteed to be unique.
2. *Name* (text string) is the long name of this token type.
3. *Icon* (array) is the icon of this token type. Consists of two data
   items:
    1. *Type* (text string) is the MIME content type of the image in *Data*.
    2. *Data* (byte string) is the image in the format specified by *Type*.
4. *ParentTypeID* (byte string) is the *UnitID* of the parent type
   that this type derives from. `null` value indicates that there is
   no parent type.
5. *DecimalPlaces* (unsigned integer) is the number of decimal places
   to display for values of tokens of this type.
6. *SubTypeCreationPredicate* (byte string) is the predicate clause that
   controls defining new subtypes of this type.
7. *TokenCreationPredicate* (byte string) is the predicate clause that
   controls creating new tokens of this type.
8. *InvariantPredicate* (byte string) is the invariant predicate
   clause that all tokens of this type (and of subtypes) inherit into
   their owner condition.
9. *SubTypeCreationPredicateSignatures* (array of byte strings) is an
   array of inputs to satisfy the subtype creation predicates of all
   parents.

##### Create Fungible Token

This transaction creates a new fungible token.

*TransactionOrder*.*Payload*.*Type* = "createFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createFTokenAttributes/ [
    /TargetOwner/                      h'',
    /TypeID/                           h'',
    /TargetValue/                      1000,
    /TokenCreationPredicateSignatures/ [h'']
]
```

1. *TargetOwner* (byte string) is the initial owner condition of
   the new token.
2. *TypeID* (byte string) is the *UnitID* of the type of the new
   token.
3. *TargetValue* (unsinged integer) is the value of the new token.
4. *TokenCreationPredicateSignatures* (array of byte string) is an
   array of inputs to satisfy the token creation predicates of all
   parent types.

##### Transfer Fungible Token

This transaction transfers a fungible token to a new owner. The value
of the transferred token is unchanged.

*TransactionOrder*.*Payload*.*Type* = "transFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transFTokenAttributes/ [
    /TargetOwner/                  h'',
    /TargetValue/                  5,
    /Nonce/                        h'',
    /Backlink/                     h'',
    /TypeID/                       h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. *TargetOwner* (byte string) is the new owner condition of the
   token.
2. *TargetValue* (unsigned integer) must be equal to the value of the
   token. The reason for including the value of the token in the
   transaction order is to enable the recipient of the transaction to
   learn the received amount without having to look up the token.
3. *Nonce* (byte string) is an optional nonce.
4. *Backlink* (byte string) is the backlink to the previous
   transaction with the token.
5. *TypeID* (byte string) is the type of the token.
6. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Split Fungible Token

This transaction splits a fungible token in two, creating a new
fungible token with a new owner condition (*TargetOwner*) and value
(*TargetValue*). The value of the token being split is reduced by the
value of the new token and is specified in the *RemainingValue*
attribute. The sum of *TargetValue* and *RemainingValue* must be equal
to the value of the token before the split.

*TransactionOrder*.*Payload*.*Type* = "splitFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/splitFTokenAttributes/ [
    /TargetOwner/                  h'00',
    /TargetValue/                  600,
    /Nonce/                        h'',
    /Backlink/                     h'',
    /TypeID/                       h'',
    /RemainingValue/               400,
    /InvariantPredicateSignatures/ [h'53']
]
```

1. *TargetOwner* (byte string) is the owner condition of the new
   token.
2. *TargetValue* (unsigned integer) is the value of the new token.
3. *Nonce* (byte string) is an optional nonce.
4. *Backlink* (byte string) is the backlink to the previous
   transaction with the token being split.
5. *TypeID* (byte string) is the type of the token.
6. *RemainingValue* (unsigned integer) is the remaining value of the
   token being split.
7. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Burn Fungible Token

This transaction "burns" (deletes) a fungible token to be later joined
into a larger-value fungible token with the [Join Fungible
Token](#join-fungible-tokens) transaction.

*TransactionOrder*.*Payload*.*Type* = "burnFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/burnFTokenAttributes/ [
    /TypeID/                       h'',
    /Value/                        999,
    /TargetBacklink/               h'',
    /Backlink/                     h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. *TypeID* (byte string) is the type of the token.
2. *Value* (unsigned integer) is the value of the token.
3. *TargetBacklink* (byte string) is the backlink to the previous
   transaction with the fungible token that this burn is to be [joined
   into](#join-fungible-tokens).
4. *Backlink* (byte string) is the backlink to the previous
   transaction with the token.
5. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Join Fungible Tokens

This transaction joins the values of [burned
tokens](#burn-fungible-token) into a target token of the same type.

*TransactionOrder*.*Payload*.*Type* = "joinFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/joinFTokenAttributes/ [
    /Burns/                        [/omitted/],
    /BurnProofs/                   [/omitted/],
    /Backlink/                     h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. *Burns* (array) is an array of [Burn Fungible
   Token](#burn-fungible-token) transaction records.
2. *BurnProofs* (array) is an array of [Burn Fungible
   Token](#burn-fungible-token) transaction proofs. The order of this
   array must match the order of *Burns* array, so that a transaction
   and its corresponding proof have the same index.
3. *Backlink* (byte string) is the backlink to the previous
   transaction with the target token.
4. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

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

### Split Bill

Raw hex encoded transaction:
```
838544000000006573706c6974582000000000ffcc5c5d01ca3eff65c2087db3aefd3d58b20f074d52bb664c24ffae841a0bebc200582a5376a8014f0162c5594a5f1e83d7c5611b041999cfb9c67cff2482aefa72e8b636ccf79bbb1d8769ac011a11e1a30058204ddc4678fd0eeefdd6868d99e644c0b43c20ed38292aee3893e4542f0a62aae68318250158208db6886a5d0fa64c2544e65b3f296c07a29f427db8d15517f70f7ff6f825e3dd58675354013f5d8fa1c59cb5b69fea2e82da14fb9f4579e4b49bfb963a3670c1a0d1215669547d1b38418a8b30bf89945ecdaa04adb879496c8ec55d7a274cf4f6f7bfde8f0055010225fd546b19683bed7663a83f97b1a1545a52f180f432ee1748fc3b51090eba5cf6
```

Same hex encoded data with annotations:
```
83                                      # array(3)
   85                                   # array(5)
      44                                # bytes(4)
         00000000                       # "\u0000\u0000\u0000\u0000"
      65                                # text(5)
         73706C6974                     # "split"
      58 20                             # bytes(32)
         00000000FFCC5C5D01CA3EFF65C2087DB3AEFD3D58B20F074D52BB664C24FFAE # "\u0000\u0000\u0000\u0000\xFF\xCC\\]\u0001\xCA>\xFFe\xC2\b}\xB3\xAE\xFD=X\xB2\u000F\aMR\xBBfL$\xFF\xAE"
      84                                # array(4)
         1A 0BEBC200                    # unsigned(200000000)
         58 2A                          # bytes(42)
            5376A8014F0162C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D8769AC01 # "Sv\xA8\u0001O\u0001b\xC5YJ_\u001E\x83\xD7\xC5a\e\u0004\u0019\x99Ϲ\xC6|\xFF$\x82\xAE\xFAr\xE8\xB66\xCC\xF7\x9B\xBB\u001D\x87i\xAC\u0001"
         1A 11E1A300                    # unsigned(300000000)
         58 20                          # bytes(32)
            4DDC4678FD0EEEFDD6868D99E644C0B43C20ED38292AEE3893E4542F0A62AAE6 # "M\xDCFx\xFD\u000E\xEE\xFDֆ\x8D\x99\xE6D\xC0\xB4< \xED8)*\xEE8\x93\xE4T/\nb\xAA\xE6"
      83                                # array(3)
         18 25                          # unsigned(37)
         01                             # unsigned(1)
         58 20                          # bytes(32)
            8DB6886A5D0FA64C2544E65B3F296C07A29F427DB8D15517F70F7FF6F825E3DD # "\x8D\xB6\x88j]\u000F\xA6L%D\xE6[?)l\a\xA2\x9FB}\xB8\xD1U\u0017\xF7\u000F\u007F\xF6\xF8%\xE3\xDD"
   58 67                                # bytes(103)
      5354013F5D8FA1C59CB5B69FEA2E82DA14FB9F4579E4B49BFB963A3670C1A0D1215669547D1B38418A8B30BF89945ECDAA04ADB879496C8EC55D7A274CF4F6F7BFDE8F0055010225FD546B19683BED7663A83F97B1A1545A52F180F432EE1748FC3B51090EBA5C # "ST\u0001?]\x8F\xA1Ŝ\xB5\xB6\x9F\xEA.\x82\xDA\u0014\xFB\x9FEy䴛\xFB\x96:6p\xC1\xA0\xD1!ViT}\e8A\x8A\x8B0\xBF\x89\x94^ͪ\u0004\xAD\xB8yIl\x8E\xC5]z'L\xF4\xF6\xF7\xBFޏ\u0000U\u0001\u0002%\xFDTk\u0019h;\xEDvc\xA8?\x97\xB1\xA1TZR\xF1\x80\xF42\xEE\u0017H\xFC;Q\t\u000E\xBA\\"
   F6                                   # primitive(22)
```

Extended Diagnostic Notation with annotations:
```
/TransactionOrder/ [
    /Payload/ [
        /SystemIdentifier/ h'00000000',
        /Type/             "split",
        /UnitID/           h'00000000FFCC5C5D01CA3EFF65C2087DB3AEFD3D58B20F074D52BB664C24FFAE',
        /splitAttributes/ [
            /Amount/         200000000,
            /TargetOwner/    h'5376A8014F0162C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D8769AC01',
            /RemainingValue/ 300000000,
            /Backlink/       h'4DDC4678FD0EEEFDD6868D99E644C0B43C20ED38292AEE3893E4542F0A62AAE6'
        ],
        /ClientMetadata/ [
            /Timeout/           37,
            /MaxTransactionFee/ 1,
            /FeeCreditRecordID/ h'8DB6886A5D0FA64C2544E65B3F296C07A29F427DB8D15517F70F7FF6F825E3DD'
        ]
    ],
    /OwnerProof/ h'5354013F5D8FA1C59CB5B69FEA2E82DA14FB9F4579E4B49BFB963A3670C1A0D1215669547D1B38418A8B30BF89945ECDAA04ADB879496C8EC55D7A274CF4F6F7BFDE8F0055010225FD546B19683BED7663A83F97B1A1545A52F180F432EE1748FC3B51090EBA5C',
    /FeeProof/   null
]
```

### Transfer Bill

Raw hex encoded transaction:
```
83854400000000657472616e73582000000000324af6d6bb8cb598d62e4a5feb09690762c3fa2da39c3050094ffeda83582a5376a8014f01b327e2d37f0bfb6babf6acc758a101c6d8eb03991abe7f137c62b253c5a5cfa08769ac011a0bebc2005820fd198e4b1b5fc67a750a5fcfc6cf9d0ef52af2001d0ca6fb45204a9f3c3bccdf83182801582030f77f7d84ed358247b944644bea3e285f33c441b34d0f915547b3c39e6cfe545867535401a50f4275c9c086dc1e90d6cae58cd7150548d54bbd889c34d5d395b698c3eba35e914267ab21baba49e9e2ed8a13e2389d6f31929af4dc35af0ff04ae42af152015501036b05d39ed407d002c18e9942abf835d12a7bfbe589a35d688933bd0243bf5724f6
```

Same hex encoded data with annotations:
```
83                                      # array(3)
   85                                   # array(5)
      44                                # bytes(4)
         00000000                       # "\u0000\u0000\u0000\u0000"
      65                                # text(5)
         7472616E73                     # "trans"
      58 20                             # bytes(32)
         00000000324AF6D6BB8CB598D62E4A5FEB09690762C3FA2DA39C3050094FFEDA # "\u0000\u0000\u0000\u00002J\xF6ֻ\x8C\xB5\x98\xD6.J_\xEB\ti\ab\xC3\xFA-\xA3\x9C0P\tO\xFE\xDA"
      83                                # array(3)
         58 2A                          # bytes(42)
            5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01 # "Sv\xA8\u0001O\u0001\xB3'\xE2\xD3\u007F\v\xFBk\xAB\xF6\xAC\xC7X\xA1\u0001\xC6\xD8\xEB\u0003\x99\u001A\xBE\u007F\u0013|b\xB2SťϠ\x87i\xAC\u0001"
         1A 0BEBC200                    # unsigned(200000000)
         58 20                          # bytes(32)
            FD198E4B1B5FC67A750A5FCFC6CF9D0EF52AF2001D0CA6FB45204A9F3C3BCCDF # "\xFD\u0019\x8EK\e_\xC6zu\n_\xCF\xC6ϝ\u000E\xF5*\xF2\u0000\u001D\f\xA6\xFBE J\x9F<;\xCC\xDF"
      83                                # array(3)
         18 28                          # unsigned(40)
         01                             # unsigned(1)
         58 20                          # bytes(32)
            30F77F7D84ED358247B944644BEA3E285F33C441B34D0F915547B3C39E6CFE54 # "0\xF7\u007F}\x84\xED5\x82G\xB9DdK\xEA>(_3\xC4A\xB3M\u000F\x91UG\xB3Þl\xFET"
   58 67                                # bytes(103)
      535401A50F4275C9C086DC1E90D6CAE58CD7150548D54BBD889C34D5D395B698C3EBA35E914267AB21BABA49E9E2ED8A13E2389D6F31929AF4DC35AF0FF04AE42AF152015501036B05D39ED407D002C18E9942ABF835D12A7BFBE589A35D688933BD0243BF5724 # "ST\u0001\xA5\u000FBu\xC9\xC0\x86\xDC\u001E\x90\xD6\xCA\xE5\x8C\xD7\u0015\u0005H\xD5K\xBD\x88\x9C4\xD5ӕ\xB6\x98\xC3\xEB\xA3^\x91Bg\xAB!\xBA\xBAI\xE9\xE2\xED\x8A\u0013\xE28\x9Do1\x92\x9A\xF4\xDC5\xAF\u000F\xF0J\xE4*\xF1R\u0001U\u0001\u0003k\u0005Ӟ\xD4\a\xD0\u0002\xC1\x8E\x99B\xAB\xF85\xD1*{\xFB剣]h\x893\xBD\u0002C\xBFW$"
   F6                                   # primitive(22)
```

Extended Diagnostic Notation with annotations:
```
/TransactionOrder/ [
    /Payload/ [
        /SystemIdentifier/ h'00000000',
        /Type/             "trans",
        /UnitID/           h'00000000324AF6D6BB8CB598D62E4A5FEB09690762C3FA2DA39C3050094FFEDA',
        /transAttributes/ [
            /TargetOwner/ h'5376A8014F01B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA08769AC01',
            /TargetValue/ 200000000,
            /Backlink/    h'FD198E4B1B5FC67A750A5FCFC6CF9D0EF52AF2001D0CA6FB45204A9F3C3BCCDF'
        ],
        /ClientMetadata/ [
            /Timeout/           40,
            /MaxTransactionFee/ 1,
            /FeeCreditRecordID/ h'30F77F7D84ED358247B944644BEA3E285F33C441B34D0F915547B3C39E6CFE54'
        ]
    ],
    /OwnerProof/ h'535401A50F4275C9C086DC1E90D6CAE58CD7150548D54BBD889C34D5D395B698C3EBA35E914267AB21BABA49E9E2ED8A13E2389D6F31929AF4DC35AF0FF04AE42AF152015501036B05D39ED407D002C18E9942ABF835D12A7BFBE589A35D688933BD0243BF5724',
    /FeeProof/   null
]
```

## References

[^1]: https://www.rfc-editor.org/rfc/rfc8949
[^2]: https://www.rfc-editor.org/rfc/rfc8610#appendix-G
[^3]: https://cbor.me/
[^4]: https://cbor.nemo157.com/
