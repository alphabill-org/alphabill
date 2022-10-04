package tokens

import (
	"bytes"
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const (
	protobufTypeUrlPrefix                       = "type.googleapis.com/alphabill.tokens.v1."
	typeURLCreateNonFungibleTokenTypeAttributes = protobufTypeUrlPrefix + "CreateNonFungibleTokenTypeAttributes"
	typeURLMintNonFungibleTokenAttributes       = protobufTypeUrlPrefix + "MintNonFungibleTokenAttributes"
	typeURLTransferNonFungibleTokenAttributes   = protobufTypeUrlPrefix + "TransferNonFungibleTokenAttributes"
)

type (
	wrapper struct {
		transaction *txsystem.Transaction
		hashFunc    crypto.Hash
		hashValue   []byte
	}

	createNonFungibleTokenTypeWrapper struct {
		wrapper
		attributes *CreateNonFungibleTokenTypeAttributes
	}

	mintNonFungibleTokenWrapper struct {
		wrapper
		attributes *MintNonFungibleTokenAttributes
	}

	transferNonFungibleTokenWrapper struct {
		wrapper
		attributes *TransferNonFungibleTokenAttributes
	}
)

func NewGenericTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	switch tx.TransactionAttributes.TypeUrl {
	case typeURLCreateNonFungibleTokenTypeAttributes:
		pb := &CreateNonFungibleTokenTypeAttributes{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid tx attributes")
		}
		return &createNonFungibleTokenTypeWrapper{
			wrapper:    wrapper{transaction: tx},
			attributes: pb,
		}, nil
	case typeURLMintNonFungibleTokenAttributes:
		pb := &MintNonFungibleTokenAttributes{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid tx attributes")
		}
		return &mintNonFungibleTokenWrapper{
			wrapper:    wrapper{transaction: tx},
			attributes: pb,
		}, nil
	case typeURLTransferNonFungibleTokenAttributes:
		pb := &TransferNonFungibleTokenAttributes{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid tx attributes")
		}
		return &transferNonFungibleTokenWrapper{
			wrapper:    wrapper{transaction: tx},
			attributes: pb,
		}, nil
	default:
		return nil, errors.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}

func (w *wrapper) UnitID() *uint256.Int              { return uint256.NewInt(0).SetBytes(w.transaction.UnitId) }
func (w *wrapper) Timeout() uint64                   { return w.transaction.Timeout }
func (w *wrapper) SystemID() []byte                  { return w.transaction.SystemId }
func (w *wrapper) OwnerProof() []byte                { return w.transaction.OwnerProof }
func (w *wrapper) ToProtoBuf() *txsystem.Transaction { return w.transaction }

func (w *wrapper) sigBytes(b *bytes.Buffer) {
	b.Write(w.transaction.SystemId)
	b.Write(w.transaction.UnitId)
	b.Write(util.Uint64ToBytes(w.transaction.Timeout))
}

func (w *wrapper) hashComputed(hashFunc crypto.Hash) bool {
	return w.hashFunc == hashFunc && w.hashValue != nil
}

func (w *wrapper) addTransactionFieldsToHasher(hasher hash.Hash) {
	hasher.Write(w.transaction.SystemId)
	hasher.Write(w.transaction.UnitId)
	hasher.Write(w.transaction.OwnerProof)
	hasher.Write(util.Uint64ToBytes(w.transaction.Timeout))
}

func (c *createNonFungibleTokenTypeWrapper) ParentTypeID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(c.attributes.ParentTypeId)
}

func (c *mintNonFungibleTokenWrapper) NFTTypeID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(c.attributes.NftType)
}

func (c *createNonFungibleTokenTypeWrapper) Hash(hashFunc crypto.Hash) []byte {
	if c.wrapper.hashComputed(hashFunc) {
		return c.wrapper.hashValue
	}
	hasher := hashFunc.New()
	c.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write([]byte(c.attributes.Symbol))
	hasher.Write(c.attributes.ParentTypeId)
	hasher.Write(c.attributes.SubTypeCreationPredicate)
	hasher.Write(c.attributes.TokenCreationPredicate)
	hasher.Write(c.attributes.InvariantPredicate)
	hasher.Write(c.attributes.DataUpdatePredicate)
	hasher.Write(c.attributes.SubTypeCreationPredicateSignature)
	c.wrapper.hashValue = hasher.Sum(nil)
	c.wrapper.hashFunc = hashFunc
	return c.wrapper.hashValue
}

func (c *createNonFungibleTokenTypeWrapper) SigBytes() []byte {
	var b bytes.Buffer
	c.wrapper.sigBytes(&b)
	b.Write([]byte(c.attributes.Symbol))
	b.Write(c.attributes.ParentTypeId)
	b.Write(c.attributes.SubTypeCreationPredicate)
	b.Write(c.attributes.TokenCreationPredicate)
	b.Write(c.attributes.InvariantPredicate)
	b.Write(c.attributes.DataUpdatePredicate)
	return b.Bytes()
}

func (c *mintNonFungibleTokenWrapper) Hash(hashFunc crypto.Hash) []byte {
	if c.wrapper.hashComputed(hashFunc) {
		return c.wrapper.hashValue
	}
	hasher := hashFunc.New()
	c.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(c.attributes.Bearer)
	hasher.Write(c.attributes.NftType)
	hasher.Write([]byte(c.attributes.Uri))
	hasher.Write(c.attributes.Data)
	hasher.Write(c.attributes.DataUpdatePredicate)
	c.wrapper.hashValue = hasher.Sum(nil)
	c.wrapper.hashFunc = hashFunc
	return c.wrapper.hashValue
}

func (c *mintNonFungibleTokenWrapper) SigBytes() []byte {
	var b bytes.Buffer
	c.wrapper.sigBytes(&b)
	b.Write(c.attributes.Bearer)
	b.Write(c.attributes.NftType)
	b.Write([]byte(c.attributes.Uri))
	b.Write(c.attributes.Data)
	b.Write(c.attributes.DataUpdatePredicate)
	return b.Bytes()
}

func (t *transferNonFungibleTokenWrapper) Hash(hashFunc crypto.Hash) []byte {
	if t.wrapper.hashComputed(hashFunc) {
		return t.wrapper.hashValue
	}
	hasher := hashFunc.New()
	t.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(t.attributes.NewBearer)
	hasher.Write(t.attributes.Nonce)
	hasher.Write(t.attributes.Backlink)
	hasher.Write(t.attributes.InvariantPredicateSignature)
	t.wrapper.hashValue = hasher.Sum(nil)
	t.wrapper.hashFunc = hashFunc
	return t.wrapper.hashValue
}

func (t *transferNonFungibleTokenWrapper) SigBytes() []byte {
	var b bytes.Buffer
	t.wrapper.sigBytes(&b)
	b.Write(t.attributes.NewBearer)
	b.Write(t.attributes.Nonce)
	b.Write(t.attributes.Backlink)
	return b.Bytes()
}
