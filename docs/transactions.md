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
   transaction. Partitions can have different types of units, but they
   are all identified by this data item.

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
   transaction. Fee credit records are created with [Transfer to Fee
   Credit](#transfer-to-fee-credit) and [Add Fee
   Credit](#add-fee-credit) transactions.

### Transaction Types

Each partition defines its own unit types. For each unit type, a set
of valid transaction types is defined. And for each transaction type,
an array of valid attributes is defined.

A common attribute for many transaction types is *Backlink*, which
links the transaction back to the previous transaction with the same
unit and thus makes the order of transactions unambiguous. *Backlink*
is calculated as the hash of the raw CBOR encoded bytes of the
*TransactionOrder* data item. Hash algorithm is defined by each
partiton.

#### Money Partition

Money partiton has two types of units: bills and fee credit records.

Hash algorithm: SHA-256\
Required data items for all transactions:

*TransactionOrder*.*Payload*.*SystemIdentifier* = h'00000000'

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

This transaction splits a bill in two, creating a new bill with its
own owner condition (*TargetOwner*) and value (*TargetValue*). The
value of the bill being split is reduced by the value of the new bill
and is specified in the *RemainingValue* attribute. The sum of values
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
4. *Backlink* (byte string) is the backlink to the previous transaction
   with the bill.

##### Transfer Bill to Dust Collector

This transaction transfers a bill to a special owner - Dust Collector
(DC). After transferring multiple bills to DC, a single, larger-value
bill can be obtained from DC with the [Swap Bills With Dust
Collector](#swap-bills-with-dust-collector) transaction. The set of
bills being swapped needs to be decided beforehand, as the *UnitID* of
the new bill obtained from DC is calculated from the *UnitID*s of the
bills being swapped, and specified in the *Nonce* attribute of this
transaction.

Dust is not defined, any bills can be transferred to DC and swapped
for a larger-value bill.

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
4. *Backlink* (byte string) is the backlink to the previous transaction
   with the bill.

##### Swap Bills With Dust Collector

This transaction swaps the bills previously [transferred to
DC](#transfer-bill-to-dust-collector) for a new, larger-value bill.

*TransactionOrder*.*Payload*.*Type* = "swapDC"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/swapDCAttributes/ [
    /TargetOwner/      h'',
    /BillIdentifiers/  [h''],
    /DcTransfers/      [/omitted/],
    /DcTransferProofs/ [/omitted/],
    /TargetValue/      3
]
```

1. *TargetOwner* (byte string) is the owner condition of the new bill.
2. *BillIdentifiers* (array of byte strings) is a variable length
   array of bill identifiers that are being swapped.
   > TODO: Order is not important?
3. *DcTransfers* (array) is an array of [Transfer Bill to Dust
   Collector](#transfer-bill-to-dust-collector) transaction records
   ordered in strictly increasing order of bill identifiers.
4. *DcTransferProofs* (array) is an array of [Transfer Bill to Dust
   Collector](#transfer-bill-to-dust-collector) transaction proofs.
   The order of this array must match the order of *DcTransfers*
   array, so that a transaction and its corresponding proof have the
   same index.
5. *TargetValue* (unsigned integer) is the value of the new bill and
   must be equal to the sum of the values of the bills transferred to
   DC for this swap.

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
   transaction with the bill.

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
    /Amount/            100000000,
    /FeeCreditRecordID/ h'A0227AC5202427DB551B8ABE08645378347A3C5F70E0E5734F147AD45CBC1BA5',
    /Nonce/             null
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

This transaction reclaims the fee credit previously closed with the
[Close Fee Credit](#close-fee-credit) transaction in a target
partition to an existing bill in the money partition.

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

1. *CloseFeeCredit* (array) is the transaction record of the [Close
   Fee Credit](#close-fee-credit) transaction. It is constructed by a
   node when the transaction is executed and included in a
   block. Necessary for the money partition to verify the amount
   closed as fee credit in the target partition.
2. *CloseFeeCreditProof* (array) is the proof of execution of the
    transaction provided in *CloseFeeCredit* attribute. Necessary for
    the target partition to verify the amount closed as fee credit in
    the target partiton.
3. *Backlink* (byte string) is the backlink to the previous
   transaction with the bill receiving the reclaimed fee credit.

#### Tokens Partition

Tokens partiton has five types of units: fungible and non-fungible
token types, fungible and non-fungible tokens, and fee credit records.

Hash algorithm: SHA-256.
Required data items for all transactions:

*TransactionOrder*.*Payload*.*SystemIdentifier* = h'00000002'

##### Create Non-fungible Token Type

This transaction creates a non-fungible token type.

*TransactionOrder*.*Payload*.*Type* = "createNType"\
*TransactionOrder*.*Payload*.*Attributes* contains:
```
/createNTypeAttributes/ [
    /Symbol/                             "symbol",
    /Name/                               "long name",
    /Icon/                               [/Type/ "image/png", /Data/ h''],
    /ParentTypeID/                       h'00',
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
   that this type derives from. A byte with value 0 indicates there is
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
9. SubTypeCreationPredicateSignatures (array of byte strings) is an
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

This transaction creates a fungible token type.

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

1. *Symbol* (text string) is the symbol (short name) of this token
   type. Symbols are not guaranteed to be unique.
2. *Name* (text string) is the long name of this token type.
3. *Icon* (array) is the icon of this token type. Consists of two data
   items:
    1. *Type* (text string) is the MIME content type of the image in *Data*.
    2. *Data* (byte string) is the image in the format specified by *Type*.
4. *ParentTypeID* (byte string) is the *UnitID* of the parent type
   that this type derives from. A byte with value 0 indicates there is
   no parent type.
5. *DecimalPlaces* (unsigned integer) is the number of decimal places to
   display for values of tokens of the this type.
6. *SubTypeCreationPredicate* (byte string) is the predicate clause that
   controls defining new subtypes of this type.
7. *TokenCreationPredicate* (byte string) is the predicate clause that
   controls creating new tokens of this type.
8. *InvariantPredicate* (byte string) is the invariant predicate
   clause that all tokens of this type (and of subtypes) inherit into
   their owner condition.
9. SubTypeCreationPredicateSignatures (array of byte strings) is an
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
7. *TokenCreationPredicateSignatures* (array of byte string) is an
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
fungible token with its own owner condition (*TargetOwner*) and value
(*TargetValue*). The value of the token being split is reduced by the
value of the new token and is specified in the *RemainingValue*
attribute. The sum of values specified by *TargetValue* and
*RemainingValue* attributes must be equal to the value of the token
before the split.

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
6. *RemainingValue* (unsigned integer) is new remaining value of the
   token being split.
6. *InvariantPredicateSignatures* (array of byte strings) is an array
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
    /Nonce/                        h'',
    /Backlink/                     h'',
    /InvariantPredicateSignatures/ [h'']
]
```

1. *TypeID* (byte string) is the type of the token.
2. *Value* (unsigned integer) is the value of the token.
3. *Nonce* (byte string) is the current state hash of the target
   token.
   > TODO:
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

4. *DcTransferProofs* (array) is an array of [Transfer Bill to Dust
   Collector](#transfer-bill-to-dust-collector) transaction proofs.
   The order of this array must match the order of *DcTransfers*
   array, so that a proof and the corresponding transfer have the same
   index.

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
- Rename Transfer to Fee Credit -> Transfer Bill to Fee Credit for consistency?
- More examples, like *transFC* and *addFC*.
- Explain owner conditions (predicates) somewhere

## References

[^1]: https://www.rfc-editor.org/rfc/rfc8949
[^2]: https://www.rfc-editor.org/rfc/rfc8610#appendix-G
[^3]: https://cbor.me/
[^4]: https://cbor.nemo157.com/
