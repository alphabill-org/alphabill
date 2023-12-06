package tokens

import (
	"bytes"
	"strings"

	"github.com/alphabill-org/alphabill/types"
)

const (
	PayloadTypeCreateNFTType           = "createNType"
	PayloadTypeMintNFT                 = "createNToken"
	PayloadTypeTransferNFT             = "transNToken"
	PayloadTypeUpdateNFT               = "updateNToken"
	PayloadTypeCreateFungibleTokenType = "createFType"
	PayloadTypeMintFungibleToken       = "createFToken"
	PayloadTypeTransferFungibleToken   = "transFToken"
	PayloadTypeSplitFungibleToken      = "splitFToken"
	PayloadTypeBurnFungibleToken       = "burnFToken"
	PayloadTypeJoinFungibleToken       = "joinFToken"
	PayloadTypeLockToken               = "lockToken"
	PayloadTypeUnlockToken             = "unlockToken"
)

type (
	CreateNonFungibleTokenTypeAttributes struct {
		_                                  struct{}     `cbor:",toarray"`
		Symbol                             string       // the symbol (short name) of this token type; note that the symbols are not guaranteed to be unique;
		Name                               string       // the long name of this token type;
		Icon                               *Icon        // the icon of this token type;
		ParentTypeID                       types.UnitID // identifies the parent type that this type derives from; 0 indicates there is no parent type;
		SubTypeCreationPredicate           []byte       // the predicate clause that controls defining new sub-types of this type;
		TokenCreationPredicate             []byte       // the predicate clause that controls creating new tokens of this type
		InvariantPredicate                 []byte       // the invariant predicate clause that all tokens of this type (and of sub- types of this type) inherit into their bearer predicates;
		DataUpdatePredicate                []byte       // the clause that all tokens of this type (and of sub-types of this type) inherit into their data update predicates
		SubTypeCreationPredicateSignatures [][]byte     // inputs to satisfy the sub-type creation predicates of all parents.
	}

	MintNonFungibleTokenAttributes struct {
		_                                struct{}     `cbor:",toarray"`
		Bearer                           []byte       // the initial bearer predicate of the new token
		NFTTypeID                        types.UnitID // identifies the type of the new token;
		Name                             string       // the name of the new token
		URI                              string       // the optional URI of an external resource associated with the new token
		Data                             []byte       // the optional data associated with the new token
		DataUpdatePredicate              []byte       // the data update predicate of the new token;
		TokenCreationPredicateSignatures [][]byte     // inputs to satisfy the token creation predicates of all parent types.
	}

	TransferNonFungibleTokenAttributes struct {
		_                            struct{}     `cbor:",toarray"`
		NewBearer                    []byte       // the new bearer predicate of the token
		Nonce                        []byte       // optional nonce
		Backlink                     []byte       // the backlink to the previous transaction with the token
		NFTTypeID                    types.UnitID // identifies the type of the token;
		InvariantPredicateSignatures [][]byte     // inputs to satisfy the token type invariant predicates down the inheritance chain
	}

	UpdateNonFungibleTokenAttributes struct {
		_                    struct{} `cbor:",toarray"`
		Data                 []byte   // the new data to replace the data currently associated with the token
		Backlink             []byte   // the backlink to the previous transaction with the token
		DataUpdateSignatures [][]byte // inputs to satisfy the token data update predicates down the inheritance chain
	}

	CreateFungibleTokenTypeAttributes struct {
		_                                  struct{}     `cbor:",toarray"`
		Symbol                             string       // the symbol (short name) of this token type; note that the symbols are not guaranteed to be unique;
		Name                               string       // the long name of this token type;
		Icon                               *Icon        // the icon of this token type;
		ParentTypeID                       types.UnitID // identifies the parent type that this type derives from; 0 indicates there is no parent type;
		DecimalPlaces                      uint32       // the number of decimal places to display for values of tokens of the new type;
		SubTypeCreationPredicate           []byte       // the predicate clause that controls defining new sub-types of this type;
		TokenCreationPredicate             []byte       // the predicate clause that controls creating new tokens of this type
		InvariantPredicate                 []byte       // the invariant predicate clause that all tokens of this type (and of sub- types of this type) inherit into their bearer predicates;
		SubTypeCreationPredicateSignatures [][]byte     // inputs to satisfy the sub-type creation predicates of all parents.
	}

	Icon struct {
		_    struct{} `cbor:",toarray"`
		Type string   `json:"type"` // the MIME content type identifying an image format;
		Data []byte   `json:"data"` // the image in the format specified by type;
	}

	MintFungibleTokenAttributes struct {
		_                                struct{}     `cbor:",toarray"`
		Bearer                           []byte       // the initial bearer predicate of the new token
		TypeID                           types.UnitID // identifies the type of the new token;
		Value                            uint64       // the value of the new token;
		TokenCreationPredicateSignatures [][]byte     // inputs to satisfy the token creation predicates of all parent types.
	}

	TransferFungibleTokenAttributes struct {
		_                            struct{} `cbor:",toarray"`
		NewBearer                    []byte   // the initial bearer predicate of the new token
		Value                        uint64   // the value to transfer
		Nonce                        []byte
		Backlink                     []byte       // the backlink to the previous transaction with this token
		TypeID                       types.UnitID // identifies the type of the token;
		InvariantPredicateSignatures [][]byte     // inputs to satisfy the token type invariant predicates down the inheritance chain
	}

	SplitFungibleTokenAttributes struct {
		_                            struct{} `cbor:",toarray"`
		NewBearer                    []byte   // the bearer predicate of the new token;
		TargetValue                  uint64   // the value of the new token
		Nonce                        []byte
		Backlink                     []byte       // the backlink to the previous transaction with this token
		TypeID                       types.UnitID // identifies the type of the token;
		RemainingValue               uint64       // new value of the source token
		InvariantPredicateSignatures [][]byte     // inputs to satisfy the token type invariant predicates down the inheritance chain
	}

	BurnFungibleTokenAttributes struct {
		_                            struct{}     `cbor:",toarray"`
		TypeID                       types.UnitID // identifies the type of the token to burn;
		Value                        uint64       // the value to burn
		TargetTokenID                types.UnitID // the target token identifier in join step
		TargetTokenBacklink          []byte       // the current state hash of the target token
		Backlink                     []byte       // the backlink to the previous transaction with this token
		InvariantPredicateSignatures [][]byte     // inputs to satisfy the token type invariant predicates down the inheritance chain
	}

	JoinFungibleTokenAttributes struct {
		_                            struct{}                   `cbor:",toarray"`
		BurnTransactions             []*types.TransactionRecord // the transactions that burned the source tokens;
		Proofs                       []*types.TxProof           // block proofs for burn transactions
		Backlink                     []byte                     // the backlink to the previous transaction with this token
		InvariantPredicateSignatures [][]byte                   // inputs to satisfy the token type invariant predicates down the inheritance chain
	}

	LockTokenAttributes struct {
		_                            struct{} `cbor:",toarray"`
		LockStatus                   uint64   // status of the lock, non-zero value means locked
		Backlink                     []byte   // the backlink to the previous transaction with this token
		InvariantPredicateSignatures [][]byte // inputs to satisfy the token type invariant predicates down the inheritance chain
	}

	UnlockTokenAttributes struct {
		_                            struct{} `cbor:",toarray"`
		Backlink                     []byte   // the backlink to the previous transaction with this token
		InvariantPredicateSignatures [][]byte // inputs to satisfy the token type invariant predicates down the inheritance chain
	}
)

func (i *Icon) Copy() *Icon {
	if i == nil {
		return nil
	}
	return &Icon{
		Type: strings.Clone(i.Type),
		Data: bytes.Clone(i.Data),
	}
}
