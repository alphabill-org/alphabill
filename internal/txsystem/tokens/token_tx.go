package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

// token tx type interfaces
type (
	CreateNonFungibleTokenType interface {
		txsystem.GenericTransaction
		ParentTypeID() []byte
		Symbol() string
		SubTypeCreationPredicate() []byte
		TokenCreationPredicate() []byte
		InvariantPredicate() []byte
		DataUpdatePredicate() []byte
		SubTypeCreationPredicateSignatures() [][]byte
	}

	MintNonFungibleToken interface {
		txsystem.GenericTransaction
		NFTTypeID() []byte
		NFTTypeIDInt() *uint256.Int
		Bearer() []byte
		URI() string
		Data() []byte
		DataUpdatePredicate() []byte
		TokenCreationPredicateSignatures() [][]byte
	}

	TransferNonFungibleToken interface {
		txsystem.GenericTransaction
		NFTTypeID() []byte
		NewBearer() []byte
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignatures() [][]byte
	}

	UpdateNonFungibleToken interface {
		txsystem.GenericTransaction
		Data() []byte
		Backlink() []byte
		DataUpdateSignatures() [][]byte
	}

	CreateFungibleTokenType interface {
		txsystem.GenericTransaction
		ParentTypeID() []byte
		Symbol() string
		DecimalPlaces() uint32
		SubTypeCreationPredicate() []byte
		TokenCreationPredicate() []byte
		InvariantPredicate() []byte
		SubTypeCreationPredicateSignatures() [][]byte
	}

	MintFungibleToken interface {
		txsystem.GenericTransaction
		TypeIDInt() *uint256.Int
		TypeID() []byte
		Value() uint64
		Bearer() []byte
		TokenCreationPredicateSignatures() [][]byte
	}

	TransferFungibleToken interface {
		txsystem.GenericTransaction
		TypeID() []byte
		NewBearer() []byte
		Value() uint64
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignatures() [][]byte
	}

	SplitFungibleToken interface {
		txsystem.GenericTransaction
		TypeID() []byte
		HashForIDCalculation(hashFunc crypto.Hash) []byte
		NewBearer() []byte
		TargetValue() uint64
		RemainingValue() uint64
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignatures() [][]byte
	}

	BurnFungibleToken interface {
		txsystem.GenericTransaction
		TypeID() []byte
		Value() uint64
		Nonce() []byte
		Backlink() []byte
		InvariantPredicateSignatures() [][]byte
	}

	JoinFungibleToken interface {
		txsystem.GenericTransaction
		BurnTransactions() []BurnFungibleToken
		BlockProofs() []*block.BlockProof
		Backlink() []byte
		InvariantPredicateSignatures() [][]byte
	}
)

func (x *CreateFungibleTokenTypeAttributes) SetSubTypeCreationPredicateSignatures(sigs [][]byte) {
	x.SubTypeCreationPredicateSignatures = sigs
}

func (x *CreateNonFungibleTokenTypeAttributes) SetSubTypeCreationPredicateSignatures(sigs [][]byte) {
	x.SubTypeCreationPredicateSignatures = sigs
}

func (x *MintFungibleTokenAttributes) SetBearer(b []byte) {
	x.Bearer = b
}

func (x *MintFungibleTokenAttributes) SetTokenCreationPredicateSignatures(sigs [][]byte) {
	x.TokenCreationPredicateSignatures = sigs
}

func (x *MintNonFungibleTokenAttributes) SetBearer(b []byte) {
	x.Bearer = b
}

func (x *MintNonFungibleTokenAttributes) SetTokenCreationPredicateSignatures(sigs [][]byte) {
	x.TokenCreationPredicateSignatures = sigs
}

func (x *TransferNonFungibleTokenAttributes) SetInvariantPredicateSignatures(sigs [][]byte) {
	x.InvariantPredicateSignatures = sigs
}

func (x *TransferFungibleTokenAttributes) SetInvariantPredicateSignatures(sigs [][]byte) {
	x.InvariantPredicateSignatures = sigs
}

func (x *SplitFungibleTokenAttributes) SetInvariantPredicateSignatures(sigs [][]byte) {
	x.InvariantPredicateSignatures = sigs
}

func (x *BurnFungibleTokenAttributes) SetInvariantPredicateSignatures(sigs [][]byte) {
	x.InvariantPredicateSignatures = sigs
}

func (x *JoinFungibleTokenAttributes) SetInvariantPredicateSignatures(sigs [][]byte) {
	x.InvariantPredicateSignatures = sigs
}
