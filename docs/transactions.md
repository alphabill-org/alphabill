# Raw Transaction Format

- [*TransactionOrder*](#transactionorder)
  - [*Payload*](#payload)
    - [*ClientMetadata*](#clientmetadata)
  - [Transaction Types](#transaction-types)
    - [Money Partition](#money-partition)
      - [Transfer Bill](#transfer-bill)
      - [Split Bill](#split-bill)
      - [Lock Bill](#lock-bill)
      - [Unlock Bill](#unlock-bill)
      - [Transfer Bill to Dust Collector](#transfer-bill-to-dust-collector)
      - [Swap Bills With Dust Collector](#swap-bills-with-dust-collector)
      - [Lock Fee Credit](#lock-fee-credit)
      - [Unlock Fee Credit](#unlock-fee-credit)
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
      - [Lock Token](#lock-token)
      - [Unlock Token](#unlock-token)
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
owner predicate of the unit specified by *Payload*.*UnitID*. The most
common example of *OwnerProof* is a digital signature signing the CBOR
encoded *Payload*.

3. *FeeProof* (byte string) contains the arguments to satisfy the
owner predicate of the fee credit record specified by
*Payload*.*ClientMetadata*.*FeeCreditRecordID*. The most common
example of *FeeProof* is a digital signature signing the CBOR encoded
*Payload*. *FeeProof* can be set to ```null``` (CBOR simple value 22)
in case *OwnerProof* also satisfies the owner predicate of the fee
credit record.

### *Payload*

*Payload* is an array of data items which is usually covered by
signature and consists of the following (with example values):

```
/Payload/ [
    /SystemID/       1,
    /Type/           "trans",
    /UnitID/         h'000000000000000000000000000000000000000000000000000000000000000100',
    /Attributes/     [/omitted, Type dependent/],
    /ClientMetadata/ [/omitted/]
]
```

Data items in the *Payload* array:

1. *SystemID* (unsigned integer) is the identifier of the transaction
system/partition that is supposed to execute the transaction. *SystemID*s
currently in use:

    - *1* - money partition
    - *2* - tokens partition
    - *3* - EVM partition

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
    /ReferenceNumber/   h'1234567890ABCDEF'
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

4. *ReferenceNumber* (byte string, up to 32 bytes) is an optional
   byte string with user defined meaning.

### Transaction Types

Each partition defines its own unit types. For each unit type, a set
of valid transaction types is defined. And for each transaction type,
an array of valid attributes is defined.

A common attribute for many transaction types is *Counter*, which
is incremented by one for each transaction with the same
unit and thus makes the order of transactions unambiguous. *Counter*
value in a transaction must be equal to the current *Counter* value
in the state tree. Upon successful execution of the transaction, the
*Counter* value is incremented by 1.

#### Money Partition

*SystemID*: 1

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
    /Counter/     123
]
```

1. *TargetOwner* (byte string) is the new owner predicate of the bill.
2. *TargetValue* (unsigned integer) must be equal to the value of the
   bill. The reason for including the value of the bill in the
   transaction order is to enable the recipient of the transaction to
   learn the received amount without having to look up the bill.
3. *Counter* (unsigned integer) is the current counter value of this bill.

##### Split Bill

This transaction splits a bill into two or more bills, creating new
bills with new owner predicates (*TargetUnit*.*TargetOwner*) and
values (*TargetUnit*.*TargetValue*). The value of the bill being split
is reduced by the values of the new bills and is specified in the
*RemainingValue* attribute. The sums of *TargetUnit*.*TargetValue*s
and *RemainingValue* must be equal to the value of the bill before the
split.

*TransactionOrder*.*Payload*.*Type* = "split"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/splitAttributes/ [
    /TargetUnits/    [
        /TargetUnit/ [
            /TargetValue/ 99900000000,
            /TargetOwner/ h'5376A8014F0162C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D8769AC01'
        ]
    ],
    /RemainingValue/ 999999899999999996,
    /Counter/        123
]
```

1. *TargetUnits* (array) is an array of *TargetUnit* data items. Each
   *TargetUnit* is an array of two data items:
   1. *TargetValue* (unsigned integer) is the value of the new bill.
   2. *TargetOwner* (byte string) is the owner predicate of the new bill.
2. *RemainingValue* (unsigned integer) is the remaining value of the
   bill being split.
3. *Counter* (unsigned integer) is the current counter value of the 
   bill being split.

##### Lock Bill

This transaction locks the specified bill, making the bill impossible 
to spend before unlocking it first. The unlocking can happen manually
with the [Unlock](#unlock-bill) transaction or automatically on 
certain transactions e.g.
[Swap with Dust Collector](#swap-bills-with-dust-collector) or
[Reclaim Fee Credit](#reclaim-fee-credit).
Locking of the bills is optional, however, it is necessary in order to 
prevent failures due to concurrent modifications by other transactions.
The specified lock status must be non-zero value and the targeted bill
must be unlocked.

*TransactionOrder*.*Payload*.*Type* = "lock"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/lockAttributes/ [
    /LockStatus/ 1,
    /Counter/   0
]
```

1. *LockStatus* (unsigned integer) is the status of the lock, 
   must be non-zero value.
2. *Counter* (unsigned integer) is the current counter value 
   of this bill.

##### Unlock Bill

This transaction unlocks the specified bill, making the bill spendable
again. The unlocking can also happen automatically on certain transactions 
e.g. [Swap with Dust Collector](#swap-bills-with-dust-collector) or
[Reclaim Fee Credit](#reclaim-fee-credit). The targeted bill must be
in locked status.

*TransactionOrder*.*Payload*.*Type* = "unlock"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/unlockAttributes/ [
    /Counter/   123
]
```

1. *Counter* (unsigned integer) is the current counter value 
   of this bill.

##### Transfer Bill to Dust Collector

This transaction transfers a bill to a special owner - Dust Collector
(DC). After transferring multiple bills to DC, the transferred bills 
can be joined into an existing bill DC with the [Swap Bills With Dust
Collector](#swap-bills-with-dust-collector) transaction. The target bill 
must be chosen beforehand and should not be used between the transactions.
To ensure that, the target bill should be locked using a 
[Lock Bill](#lock-bill) transaction.

Dust is not defined, any bills can be transferred to DC and joined into
a larger-value bill.

*TransactionOrder*.*Payload*.*Type* = "transDC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transDCAttributes/ [
    /Value/             999999899999999996,
    /TargetUnitID/      h'',
    /TargetUnitCounter/ 123,
    /Counter/           321
]
```

1. *Value* (unsigned integer) is the value of the bill
   transferred to DC with this transaction.
2. *TargetUnitID* (byte string) is the *UnitID* of the target bill for the
   [Swap Bills With Dust Collector](#swap-bills-with-dust-collector) 
   transaction.
3. *TargetUnitCounter* (unsigned integer) is the *Counter* of the target bill 
   for the [Swap Bills With Dust Collector](#swap-bills-with-dust-collector) 
   transaction.
4. *Counter* (unsigned integer) is the current counter value of this bill.

##### Swap Bills With Dust Collector

This transaction joins the bills previously [transferred to
DC](#transfer-bill-to-dust-collector) into a target bill.
It also unlocks the target bill, if it was previously locked 
with [Lock Bill](#lock-bill) transaction.

*TransactionOrder*.*Payload*.*Type* = "swapDC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/swapDCAttributes/ [
    /OwnerPredicate/   h'',
    /DcTransfers/      [/omitted/],
    /DcTransferProofs/ [/omitted/],
    /TargetValue/      3
]
```

1. *OwnerPredicate* (byte string) is the new owner predicate of the target bill.
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

##### Lock Fee Credit

Adding and reclaiming fee credits are multistep protocols, and it’s advisable 
to lock the target unit to prevent failures due to concurrent modifications by other transactions.

More specifically, for adding fee credits:
* If the target fee credit record exists, it should be locked using a lockFC transaction in
the target partition.
* The amount to be added to fee credits should be paid using a transFC transaction in
the money partition. To prevent replay attacks, the transFC transaction must identify
the target record and its current state.
* The transferred value is added to the target record using an addFC transaction in the
target partition. As this transaction completes the fee transfer process, it also unlocks
the target record.

And for reclaiming fee credits:
* The target bill should be locked using a [Lock Bill](#lock-bill) transaction in the money partition.
* The fee credit should be closed using a [Close Fee Credit](#close-fee-credit) transaction in the target partition.
To prevent replay attacks, the [Close Fee Credit](#close-fee-credit) transaction must 
identify the target bill and its current state.
* The reclaimed value is added to the target bill using a [Reclaim Fee Credit](#reclaim-fee-credit)
transaction in the money partition. As this transaction completes the fee transfer process, it also 
unlocks the target bill.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "lockFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/lockFCAttributes/ [
    /LockStatus/ 1,
    /Backlink/   h'52F43127F58992B6FCFA27A64C980E70D26C2CDE0281AC93435D10EB8034B695'
]
```

1. *LockStatus* (unsigned integer) is the new lock status. Must be non-zero value.
2. *Backlink* (byte string) is the last hash of 
   [Lock Fee Credit](#lock-fee-credit), 
   [Unlock Fee Credit](#unlock-fee-credit),
   [Add Fee Credit](#add-fee-credit) or
   [Close Fee Credit](#close-fee-credit)
   transaction with the bill.

##### Unlock Fee Credit

This transaction unlocks the specified fee credit record. 
Note that it's not required to manually unlock the unit 
as the fee credit record is automatically unlocked on 
[Add Fee Credit](#add-fee-credit) transaction.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "unlockFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/lockFCAttributes/ [
    /Backlink/   h'52F43127F58992B6FCFA27A64C980E70D26C2CDE0281AC93435D10EB8034B695'
]
```

1. *Backlink* (byte string) is the last hash of
   [Lock Fee Credit](#lock-fee-credit),
   [Unlock Fee Credit](#unlock-fee-credit),
   [Add Fee Credit](#add-fee-credit) or
   [Close Fee Credit](#close-fee-credit) 
   transaction with the fee credit record.

##### Transfer to Fee Credit

This transaction reserves money on the money partition to be paid as
fees on the target partition. Money partition can also be the target
partition. A bill can be transferred to fee credit partially.

To bootstrap a fee credit record on the money partition, the fee for
this transaction is handled outside the fee credit system. That is,
the fee for this transaction is taken directly from the transferred
*Amount* and the amount available for the fee credit record in the
target partition is reduced
accordingly. *ClientMetadata*.*MaxTransactionFee* still applies.

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
    /Amount/               100000000,
    /TargetSystemID/       2,
    /TargetUnitID/         h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA52F',
    /EarliestAdditionTime/ 13,
    /LatestAdditionTime/   23,
    /TargetUnitBacklink/   null,
    /Counter/              123
]
```

1. *Amount* (unsigned integer) is the amount of money to reserve for
   paying fees in the target partition. A bill can be transferred to
   partially.
2. *TargetSystemID* (unsigned integer) is the system identifier of the
   target partition where the *Amount* can be spent on fees.
3. *TargetUnitID* (byte string) is the target fee credit record
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
6. *TargetUnitBacklink* (byte string) is the hash of the last fee credit 
   transaction (addFC, closeFC, lockFC, unlockFC) executed for the
   *TargetUnitID* in the target partition, or `null` if it does not exist yet.
7. *Counter* (unsigned integer) is the current counter value of this bill.

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
   predicate for the created fee credit record. It needs to be
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
fee credit. The *TargetUnitCounter* attribute fixes the current 
state of the bill used to reclaim the closed fee credit, 
and any other transaction with the bill would invalidate that 
counter.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "closeFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/closeFCAttributes/ [
    /Amount/            100000000,
    /TargetUnitID/      h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA500',
    /TargetUnitCounter/ 123
]
```

1. *Amount* (unsigned integer) is the current balance of the fee
   credit record.
2. *TargetUnitID* (byte string) is the *UnitID* of the existing bill
   in the money partition that is used to reclaim the fee credit.
3. *TargetUnitCounter* (unsigned integer) is the current counter value
   of the target bill where to reclaim fee credits in money partition.

##### Reclaim Fee Credit

This transaction reclaims the fee credit, previously closed with a
[Close Fee Credit](#close-fee-credit) transaction in a target
partition, to an existing bill in the money partition.
It also unlocks the target bill, if it was previously locked
with [Lock Bill](#lock-bill) transaction.

*TransactionOrder*.*FeeProof* = `null`\
*TransactionOrder*.*Payload*.*Type* = "reclFC"\
*TransactionOrder*.*Payload*.*ClientMetadata*.*FeeCreditRecordID* = `null`\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/reclFCAttributes/ [
    /CloseFeeCredit/      [/TransactionRecord/],
    /CloseFeeCreditProof/ [/TransactionProof/],
    /Counter/             123
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
3. *Counter* (unsigned integer) is the current counter value
   of the bill receiving the reclaimed fee credit.

#### Tokens Partition

*SystemID*: *2*

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
    /Nonce/                              0,
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
   their owner predicate.
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
    /OwnerPredicate/                   h'',
    /TypeID/                           h'',
    /Name/                             "",
    /URI/                              "",
    /Data/                             h'',
    /DataUpdatePredicate/              h'',
    /Nonce/                            0,
    /TokenCreationPredicateSignatures/ [h'']
]
```

1. *OwnerPredicate* (byte string) is the initial owner predicate of
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
7. *Nonce* (unsigned integer) optional nonce. 
8. *TokenCreationPredicateSignatures* (array of byte string) is an
   array of inputs to satisfy the token creation predicates of all
   parent types.

##### Transfer Non-fungible Token

This transaction transfers a non-fungible token to a new owner. The
token must not be in [locked](#lock-token) status.

*TransactionOrder*.*Payload*.*Type* = "transNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transNTokenAttributes/ [
    /TargetOwner/                  h'',
    /Nonce/                        h'',
    /Counter/                      0,
    /TypeID/                       h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. *TargetOwner* (byte string) is the new owner predicate of the
   token.
2. *Nonce* (byte string) is an optional nonce.
3. *Counter* (unsigned integer) is the current counter value 
   of this token.
4. *TypeID* (byte string) is the type of the token.
5. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Update Non-fungible Token

This transaction updates the data of a non-fungible token. The token
must not be in [locked](#lock-token) status.

*TransactionOrder*.*Payload*.*Type* = "updateNToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/updateNTokenAttributes/ [
    /Data/                 h'',
    /Counter/              0,
    /DataUpdateSignatures/ [h'']
]
```

1. *Data* (byte string) is the new data to replace the data currently
   associated with the token.
2. *Counter* (unsigned integer) is the current counter value
   of this token.
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
   their owner predicate.
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
    /Nonce/                            0,
    /TokenCreationPredicateSignatures/ [h'']
]
```

1. *TargetOwner* (byte string) is the initial owner predicate of
   the new token.
2. *TypeID* (byte string) is the *UnitID* of the type of the new
   token.
3. *TargetValue* (unsigned integer) is the value of the new token.
4. *Nonce* (unsigned integer) optional nonce.
5. *TokenCreationPredicateSignatures* (array of byte string) is an
   array of inputs to satisfy the token creation predicates of all
   parent types.

##### Transfer Fungible Token

This transaction transfers a fungible token to a new owner. The value
of the transferred token is unchanged. The token must not be in
[locked](#lock-token) status.

*TransactionOrder*.*Payload*.*Type* = "transFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/transFTokenAttributes/ [
    /TargetOwner/                  h'',
    /TargetValue/                  5,
    /Nonce/                        h'',
    /Counter/                      0,
    /TypeID/                       h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. *TargetOwner* (byte string) is the new owner predicate of the
   token.
2. *TargetValue* (unsigned integer) must be equal to the value of the
   token. The reason for including the value of the token in the
   transaction order is to enable the recipient of the transaction to
   learn the received amount without having to look up the token.
3. *Nonce* (byte string) is an optional nonce.
4. *Counter* (unsigned integer) is the current counter value
   of this token.
5. *TypeID* (byte string) is the type of the token.
6. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Split Fungible Token

This transaction splits a fungible token in two, creating a new
fungible token with a new owner predicate (*TargetOwner*) and value
(*TargetValue*). The value of the token being split is reduced by the
value of the new token and is specified in the *RemainingValue*
attribute. The sum of *TargetValue* and *RemainingValue* must be equal
to the value of the token before the split. The token must not be in
[locked](#lock-token) status.

*TransactionOrder*.*Payload*.*Type* = "splitFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/splitFTokenAttributes/ [
    /TargetOwner/                  h'00',
    /TargetValue/                  600,
    /Nonce/                        h'',
    /Counter/                      0,
    /TypeID/                       h'',
    /RemainingValue/               400,
    /InvariantPredicateSignatures/ [h'53']
]
```

1. *TargetOwner* (byte string) is the owner predicate of the new
   token.
2. *TargetValue* (unsigned integer) is the value of the new token.
3. *Nonce* (byte string) is an optional nonce.
4. *Counter* (unsigned integer) is the current counter value
   of this token.
5. *TypeID* (byte string) is the type of the token.
6. *RemainingValue* (unsigned integer) is the remaining value of the
   token being split.
7. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Burn Fungible Token

This transaction "burns" (deletes) a fungible token to be later joined
into a larger-value fungible token with the [Join Fungible
Token](#join-fungible-tokens) transaction. The token must not be in
[locked](#lock-token) status.

*TransactionOrder*.*Payload*.*Type* = "burnFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/burnFTokenAttributes/ [
    /TypeID/                       h'',
    /Value/                        999,
    /TargetTokenID/                h'',
    /TargetTokenCounter/           0,
    /Counter/                      0,
    /InvariantPredicateSignatures/ [h'']
]
```

1. *TypeID* (byte string) is the type of the token.
2. *Value* (unsigned integer) is the value of the token.
3. *TargetTokenID* (byte string) is the token id of the target token 
   that this burn is to be [joined into](#join-fungible-tokens).
4. *TargetTokenCounter* (unsigned integer) is the current counter value
   of the fungible token that this burn is to be [joined
   into](#join-fungible-tokens).
5. *Counter* (unsigned integer) is the current counter value
   of this token.
6. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Join Fungible Tokens

This transaction joins the values of [burned
tokens](#burn-fungible-token) into a target token of the same
type. The target token is [unlocked](#unlock-token) automatically if
it was in [locked](#lock-token) status.

*TransactionOrder*.*Payload*.*Type* = "joinFToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/joinFTokenAttributes/ [
    /BurnTransactions/             [/omitted/],
    /BurnTransactionProofs/        [/omitted/],
    /Counter/                      0,
    /InvariantPredicateSignatures/ [h'']
]
```

1. *BurnTransactions* (array) is an array of [Burn Fungible
   Token](#burn-fungible-token) transaction records.
   The transactions must be listed in strictly increasing
   order of token identifiers to ensure that no source token can be
   included multiple times.
2. *BurnTransactionProofs* (array) is an array of [Burn Fungible
   Token](#burn-fungible-token) transaction proofs. The order of this
   array must match the order of *BurnTransactions* array, so that a 
   transaction and its corresponding proof have the same index.
3. *Counter* (unsigned integer) is the current counter value
   of this token.
4. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Lock Token

This transaction locks the specified token. Locked non-fungible tokens
cannot be [transferred](#transfer-non-fungible-token) or
[updated](#update-non-fungible-token). Locked fungible tokens cannot
be [transferred](#transfer-fungible-token), [split](#split-fungible-token)
or [burned](#burn-fungible-token). Unlocking can happen manually with the
[Unlock Token](#unlock-token) transaction or automatically with the
[Join Fungible Token](#join-fungible-tokens) transaction for fungible tokens.

Locking of the tokens is optional. However, it is recommended to lock
the target fungible token while joining is in progress, to prevent
other transactions from changing its state and thus making the final
[join](#join-fungible-tokens) transaction fail. The specified lock
status must be non-zero value and the targeted token must be unlocked.

*TransactionOrder*.*Payload*.*Type* = "lockToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/lockTokenAttributes/ [
    /LockStatus/                   1,
    /Counter/                      0,
    /InvariantPredicateSignatures/ [h'']
]
```

1. *LockStatus* (unsigned integer) is the status of the lock,
   must be non-zero value.
2. *Counter* (unsigned integer) is the current counter value
   of this token.
3. *InvariantPredicateSignatures* (array of byte strings) is an array
   of inputs to satisfy the token type invariant predicates down the
   inheritance chain.

##### Unlock Token

This transaction unlocks the specified token. Unlocking can also
happen automatically when [joining](#join-fungible-tokens) fungible
tokens. The targeted token must be in locked status.

*TransactionOrder*.*Payload*.*Type* = "unlockToken"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/unlockTokenAttributes/ [
    /Counter/                      0,
    /InvariantPredicateSignatures/ [h'']
]
```

1. *Counter* (unsigned integer) is the current counter value
   of this token.
2. *InvariantPredicateSignatures* (array of byte strings) is an array
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
8385016573706c697458215a6af25f2a257c91f528e04d5c026e090ddf387a06ad9e155c009b821df602bf008381821a0bebc200582683004102582062c5594a5f1e83d7c5611b041999cfb9c67cff2482aefa72e8b636ccf79bbb1d1a11e1a3015820bc152aa13ac535563179c06e4f40ec26e4bdfe6e6252e720e794b4735ba5c6ec83181c015821411dbb429d60228cacdea90d4b5f1c0b022d7f03d9cb9e5042be3f74b7a8c23a0f586782584174faf21cfc0a37bd6d4ec3fd6a946516b4c9ab5817fe0bd1c6588ecc93d08f0220bfd1d76a26c733e0e56f99e268363226b255833f29602a849367a288432c0c0158210225fd546b19683bed7663a83f97b1a1545a52f180f432ee1748fc3b51090eba5cf6
```

Same hex encoded data with annotations:
```
83                                      # array(3)
   85                                   # array(5)
      01                                # unsigned(1)
      65                                # text(5)
         73706C6974                     # "split"
      58 21                             # bytes(33)
         5A6AF25F2A257C91F528E04D5C026E090DDF387A06AD9E155C009B821DF602BF00 # "Zj\xF2_*%|\x91\xF5(\xE0M\\\u0002n\t\r\xDF8z\u0006\xAD\x9E\u0015\\\u0000\x9B\x82\u001D\xF6\u0002\xBF\u0000"
      83                                # array(3)
         81                             # array(1)
            82                          # array(2)
               1A 0BEBC200              # unsigned(200000000)
               58 26                    # bytes(38)
                  83004102582062C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D # "\x83\u0000A\u0002X b\xC5YJ_\u001E\x83\xD7\xC5a\e\u0004\u0019\x99Ϲ\xC6|\xFF$\x82\xAE\xFAr\xE8\xB66\xCC\xF7\x9B\xBB\u001D"
         1A 11E1A301                    # unsigned(300000001)
         58 20                          # bytes(32)
            BC152AA13AC535563179C06E4F40EC26E4BDFE6E6252E720E794B4735BA5C6EC # "\xBC\u0015*\xA1:\xC55V1y\xC0nO@\xEC&\xE4\xBD\xFEnbR\xE7 甴s[\xA5\xC6\xEC"
      83                                # array(3)
         18 1C                          # unsigned(28)
         01                             # unsigned(1)
         58 21                          # bytes(33)
            411DBB429D60228CACDEA90D4B5F1C0B022D7F03D9CB9E5042BE3F74B7A8C23A0F # "A\u001D\xBBB\x9D`\"\x8C\xACީ\rK_\u001C\v\u0002-\u007F\u0003\xD9˞PB\xBE?t\xB7\xA8\xC2:\u000F"
   58 67                                # bytes(103)
      82584174FAF21CFC0A37BD6D4EC3FD6A946516B4C9AB5817FE0BD1C6588ECC93D08F0220BFD1D76A26C733E0E56F99E268363226B255833F29602A849367A288432C0C0158210225FD546B19683BED7663A83F97B1A1545A52F180F432EE1748FC3B51090EBA5C # "\x82XAt\xFA\xF2\u001C\xFC\n7\xBDmN\xC3\xFDj\x94e\u0016\xB4ɫX\u0017\xFE\v\xD1\xC6X\x8E̓Џ\u0002 \xBF\xD1\xD7j&\xC73\xE0\xE5o\x99\xE2h62&\xB2U\x83?)`*\x84\x93g\xA2\x88C,\f\u0001X!\u0002%\xFDTk\u0019h;\xEDvc\xA8?\x97\xB1\xA1TZR\xF1\x80\xF42\xEE\u0017H\xFC;Q\t\u000E\xBA\\"
   F6                                   # primitive(22)
```

Extended Diagnostic Notation with annotations:
```
/TransactionOrder/ [
    /Payload/ [
        /SystemID/ 1,
        /Type/     "split",
        /UnitID/   h'5A6AF25F2A257C91F528E04D5C026E090DDF387A06AD9E155C009B821DF602BF00',
        /splitAttributes/ [
            /TargetUnits/ [
                /TargetUnit/ [
                    /TargetValue/ 200000000,
                    /TargetOwner/ h'83004102582062C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D'
                ]
            ],
            /RemainingValue/ 300000001,
            /Counter/        123
        ],
        /ClientMetadata/ [
            /Timeout/           28,
            /MaxTransactionFee/ 1,
            /FeeCreditRecordID/ h'411DBB429D60228CACDEA90D4B5F1C0B022D7F03D9CB9E5042BE3F74B7A8C23A0F'
        ]
    ],
    /OwnerProof/ h'82584174FAF21CFC0A37BD6D4EC3FD6A946516B4C9AB5817FE0BD1C6588ECC93D08F0220BFD1D76A26C733E0E56F99E268363226B255833F29602A849367A288432C0C0158210225FD546B19683BED7663A83F97B1A1545A52F180F432EE1748FC3B51090EBA5C',
    /FeeProof/   null
]
```

### Transfer Bill

Raw hex encoded transaction:
```
838501657472616e7358212ee0c0d33035f3556609871d5fd9cb0a345f87625b26c343991102e4355eeb1c00835826830041025820b327e2d37f0bfb6babf6acc758a101c6d8eb03991abe7f137c62b253c5a5cfa01a0bebc2005820dc6b998fab136e8476c53fff42452f157dec1aa07846154b2fbece3c45f9f8b183181f01582162c5594a5f1e83d7c5611b041999cfb9c67cff2482aefa72e8b636ccf79bbb1d0f586782584130267277c3552378e7f3873c14c56b89a48e3cf79753f95775e3128be0e2aacc467762f6b9b220102738e0f1734caa8af879b259291916cadb8cf6fdf143f9f3015821036b05d39ed407d002c18e9942abf835d12a7bfbe589a35d688933bd0243bf5724f6
```

Same hex encoded data with annotations:
```
83                                      # array(3)
   85                                   # array(5)
      01                                # unsigned(1)
      65                                # text(5)
         7472616E73                     # "trans"
      58 21                             # bytes(33)
         2EE0C0D33035F3556609871D5FD9CB0A345F87625B26C343991102E4355EEB1C00 # ".\xE0\xC0\xD305\xF3Uf\t\x87\u001D_\xD9\xCB\n4_\x87b[&\xC3C\x99\u0011\u0002\xE45^\xEB\u001C\u0000"
      83                                # array(3)
         58 26                          # bytes(38)
            830041025820B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA0 # "\x83\u0000A\u0002X \xB3'\xE2\xD3\u007F\v\xFBk\xAB\xF6\xAC\xC7X\xA1\u0001\xC6\xD8\xEB\u0003\x99\u001A\xBE\u007F\u0013|b\xB2SťϠ"
         1A 0BEBC200                    # unsigned(200000000)
         58 20                          # bytes(32)
            DC6B998FAB136E8476C53FFF42452F157DEC1AA07846154B2FBECE3C45F9F8B1 # "\xDCk\x99\x8F\xAB\u0013n\x84v\xC5?\xFFBE/\u0015}\xEC\u001A\xA0xF\u0015K/\xBE\xCE<E\xF9\xF8\xB1"
      83                                # array(3)
         18 1F                          # unsigned(31)
         01                             # unsigned(1)
         58 21                          # bytes(33)
            62C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D0F # "b\xC5YJ_\u001E\x83\xD7\xC5a\e\u0004\u0019\x99Ϲ\xC6|\xFF$\x82\xAE\xFAr\xE8\xB66\xCC\xF7\x9B\xBB\u001D\u000F"
   58 67                                # bytes(103)
      82584130267277C3552378E7F3873C14C56B89A48E3CF79753F95775E3128BE0E2AACC467762F6B9B220102738E0F1734CAA8AF879B259291916CADB8CF6FDF143F9F3015821036B05D39ED407D002C18E9942ABF835D12A7BFBE589A35D688933BD0243BF5724 # "\x82XA0&rw\xC3U#x\xE7\xF3\x87<\u0014\xC5k\x89\xA4\x8E<\xF7\x97S\xF9Wu\xE3\u0012\x8B\xE0\xE2\xAA\xCCFwb\xF6\xB9\xB2 \u0010'8\xE0\xF1sL\xAA\x8A\xF8y\xB2Y)\u0019\u0016\xCAی\xF6\xFD\xF1C\xF9\xF3\u0001X!\u0003k\u0005Ӟ\xD4\a\xD0\u0002\xC1\x8E\x99B\xAB\xF85\xD1*{\xFB剣]h\x893\xBD\u0002C\xBFW$"
   F6                                   # primitive(22)
```

Extended Diagnostic Notation with annotations:
```
/TransactionOrder/ [
    /Payload/ [
        /SystemID/ 1,
        /Type/     "trans",
        /UnitID/   h'2EE0C0D33035F3556609871D5FD9CB0A345F87625B26C343991102E4355EEB1C00',
        /transAttributes/ [
            /TargetOwner/ h'830041025820B327E2D37F0BFB6BABF6ACC758A101C6D8EB03991ABE7F137C62B253C5A5CFA0',
            /TargetValue/ 200000000,
            /Counter/     123
        ],
        /ClientMetadata/ [
            /Timeout/           31,
            /MaxTransactionFee/ 1,
            /FeeCreditRecordID/ h'62C5594A5F1E83D7C5611B041999CFB9C67CFF2482AEFA72E8B636CCF79BBB1D0F'
        ]
    ],
    /OwnerProof/ h'82584130267277C3552378E7F3873C14C56B89A48E3CF79753F95775E3128BE0E2AACC467762F6B9B220102738E0F1734CAA8AF879B259291916CADB8CF6FDF143F9F3015821036B05D39ED407D002C18E9942ABF835D12A7BFBE589A35D688933BD0243BF5724'
    /FeeProof/   null
]
```

## References

[^1]: https://www.rfc-editor.org/rfc/rfc8949
[^2]: https://www.rfc-editor.org/rfc/rfc8610#appendix-G
[^3]: https://cbor.me/
[^4]: https://cbor.nemo157.com/
