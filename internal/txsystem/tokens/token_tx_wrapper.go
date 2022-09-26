package tokens

import (
	"bytes"
	"crypto"
	"hash"

	"google.golang.org/protobuf/proto"

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
	typeURLUpdateNonFungibleTokenAttributes     = protobufTypeUrlPrefix + "UpdateNonFungibleTokenAttributes"
	typeURLCreateFungibleTokenTypeAttributes    = protobufTypeUrlPrefix + "CreateFungibleTokenTypeAttributes"
	typeURLMintFungibleTokenAttributes          = protobufTypeUrlPrefix + "MintFungibleTokenAttributes"
	typeURLTransferFungibleTokenAttributes      = protobufTypeUrlPrefix + "TransferFungibleTokenAttributes"
	typeURLSplitFungibleTokenAttributes         = protobufTypeUrlPrefix + "SplitFungibleTokenAttributes"
	typeURLBurnFungibleTokenAttributes          = protobufTypeUrlPrefix + "BurnFungibleTokenAttributes"
	typeURLJoinFungibleTokenAttributes          = protobufTypeUrlPrefix + "JoinFungibleTokenAttributes"
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

	updateNonFungibleTokenWrapper struct {
		wrapper
		attributes *UpdateNonFungibleTokenAttributes
	}

	createFungibleTokenTypeWrapper struct {
		wrapper
		attributes *CreateFungibleTokenTypeAttributes
	}

	mintFungibleTokenWrapper struct {
		wrapper
		attributes *MintFungibleTokenAttributes
	}

	transferFungibleTokenWrapper struct {
		wrapper
		attributes *TransferFungibleTokenAttributes
	}

	splitFungibleTokenWrapper struct {
		wrapper
		attributes *SplitFungibleTokenAttributes
	}

	burnFungibleTokenWrapper struct {
		wrapper
		attributes *BurnFungibleTokenAttributes
	}

	// TODO AB-347
	joinFungibleTokenWrapper struct {
		wrapper
		attributes *JoinFungibleTokenAttributes
	}
)

func NewGenericTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	switch tx.TransactionAttributes.TypeUrl {
	case typeURLCreateNonFungibleTokenTypeAttributes:
		return convertToWrapper(
			&CreateNonFungibleTokenTypeAttributes{},
			func(a *CreateNonFungibleTokenTypeAttributes) txsystem.GenericTransaction {
				return &createNonFungibleTokenTypeWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLMintNonFungibleTokenAttributes:
		return convertToWrapper(
			&MintNonFungibleTokenAttributes{},
			func(a *MintNonFungibleTokenAttributes) txsystem.GenericTransaction {
				return &mintNonFungibleTokenWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLTransferNonFungibleTokenAttributes:
		return convertToWrapper(
			&TransferNonFungibleTokenAttributes{},
			func(a *TransferNonFungibleTokenAttributes) txsystem.GenericTransaction {
				return &transferNonFungibleTokenWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLUpdateNonFungibleTokenAttributes:
		return convertToWrapper(
			&UpdateNonFungibleTokenAttributes{},
			func(a *UpdateNonFungibleTokenAttributes) txsystem.GenericTransaction {
				return &updateNonFungibleTokenWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLCreateFungibleTokenTypeAttributes:
		return convertToWrapper(
			&CreateFungibleTokenTypeAttributes{},
			func(a *CreateFungibleTokenTypeAttributes) txsystem.GenericTransaction {
				return &createFungibleTokenTypeWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLMintFungibleTokenAttributes:
		return convertToWrapper(
			&MintFungibleTokenAttributes{},
			func(a *MintFungibleTokenAttributes) txsystem.GenericTransaction {
				return &mintFungibleTokenWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLTransferFungibleTokenAttributes:
		return convertToWrapper(
			&TransferFungibleTokenAttributes{},
			func(a *TransferFungibleTokenAttributes) txsystem.GenericTransaction {
				return &transferFungibleTokenWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLSplitFungibleTokenAttributes:
		return convertToWrapper(
			&SplitFungibleTokenAttributes{},
			func(a *SplitFungibleTokenAttributes) txsystem.GenericTransaction {
				return &splitFungibleTokenWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLBurnFungibleTokenAttributes:
		return convertToWrapper(
			&BurnFungibleTokenAttributes{},
			func(a *BurnFungibleTokenAttributes) txsystem.GenericTransaction {
				return &burnFungibleTokenWrapper{
					wrapper:    wrapper{transaction: tx},
					attributes: a,
				}
			},
			tx)
	case typeURLJoinFungibleTokenAttributes:
		// TODO AB-347
		panic("not implemented")
	default:
		return nil, errors.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}

// convertToWrapper converts given tx to a generic transaction. attrType is the type of the tx attributes. createGenericTxFunc creates an instance of given generic transaction.
func convertToWrapper[A proto.Message, G txsystem.GenericTransaction](attrType A, createGenericTxFunc func(a A) G, tx *txsystem.Transaction) (g G, err error) {
	err = tx.TransactionAttributes.UnmarshalTo(attrType)
	if err != nil {
		return g, errors.Wrapf(err, "invalid tx attributes")
	}
	return createGenericTxFunc(attrType), err
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

func (u *updateNonFungibleTokenWrapper) Hash(hashFunc crypto.Hash) []byte {
	if u.wrapper.hashComputed(hashFunc) {
		return u.wrapper.hashValue
	}
	hasher := hashFunc.New()
	u.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(u.attributes.Data)
	hasher.Write(u.attributes.Backlink)
	hasher.Write(u.attributes.DataUpdateSignature)
	u.wrapper.hashValue = hasher.Sum(nil)
	u.wrapper.hashFunc = hashFunc
	return u.wrapper.hashValue
}

func (u *updateNonFungibleTokenWrapper) SigBytes() []byte {
	var b bytes.Buffer
	u.wrapper.sigBytes(&b)
	b.Write(u.attributes.Data)
	b.Write(u.attributes.Backlink)
	return b.Bytes()
}

func (c *createFungibleTokenTypeWrapper) Hash(hashFunc crypto.Hash) []byte {
	if c.wrapper.hashComputed(hashFunc) {
		return c.wrapper.hashValue
	}
	hasher := hashFunc.New()
	c.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write([]byte(c.attributes.Symbol))
	hasher.Write(c.attributes.ParentTypeId)
	hasher.Write(util.Uint32ToBytes(c.attributes.DecimalPlaces))
	hasher.Write(c.attributes.SubTypeCreationPredicate)
	hasher.Write(c.attributes.TokenCreationPredicate)
	hasher.Write(c.attributes.InvariantPredicate)
	hasher.Write(c.attributes.SubTypeCreationPredicateSignature)
	c.wrapper.hashValue = hasher.Sum(nil)
	c.wrapper.hashFunc = hashFunc
	return c.wrapper.hashValue
}

func (c *createFungibleTokenTypeWrapper) SigBytes() []byte {
	var b bytes.Buffer
	c.wrapper.sigBytes(&b)
	b.Write([]byte(c.attributes.Symbol))
	b.Write(c.attributes.ParentTypeId)
	b.Write(util.Uint32ToBytes(c.attributes.DecimalPlaces))
	b.Write(c.attributes.SubTypeCreationPredicate)
	b.Write(c.attributes.TokenCreationPredicate)
	b.Write(c.attributes.InvariantPredicate)
	return b.Bytes()
}

func (c *createFungibleTokenTypeWrapper) ParentTypeID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(c.attributes.ParentTypeId)
}

func (m *mintFungibleTokenWrapper) Hash(hashFunc crypto.Hash) []byte {
	if m.wrapper.hashComputed(hashFunc) {
		return m.wrapper.hashValue
	}
	hasher := hashFunc.New()
	m.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(m.attributes.Bearer)
	hasher.Write(m.attributes.Type)
	hasher.Write(util.Uint64ToBytes(m.attributes.Value))
	hasher.Write(m.attributes.TokenCreationPredicateSignature)
	m.wrapper.hashValue = hasher.Sum(nil)
	m.wrapper.hashFunc = hashFunc
	return m.wrapper.hashValue
}

func (m *mintFungibleTokenWrapper) SigBytes() []byte {
	var b bytes.Buffer
	m.wrapper.sigBytes(&b)
	b.Write(m.attributes.Bearer)
	b.Write(m.attributes.Type)
	b.Write(util.Uint64ToBytes(m.attributes.Value))
	return b.Bytes()
}

func (x *MintFungibleTokenAttributes) GetTokenTypeID() *uint256.Int {
	return uint256.NewInt(0).SetBytes(x.Type)
}

func (t *transferFungibleTokenWrapper) Hash(hashFunc crypto.Hash) []byte {
	if t.wrapper.hashComputed(hashFunc) {
		return t.wrapper.hashValue
	}
	hasher := hashFunc.New()
	t.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(t.attributes.NewBearer)
	hasher.Write(util.Uint64ToBytes(t.attributes.Value))
	hasher.Write(t.attributes.Nonce)
	hasher.Write(t.attributes.Backlink)
	hasher.Write(t.attributes.InvariantPredicateSignature)
	t.wrapper.hashValue = hasher.Sum(nil)
	t.wrapper.hashFunc = hashFunc
	return t.wrapper.hashValue
}

func (t *transferFungibleTokenWrapper) SigBytes() []byte {
	var b bytes.Buffer
	t.wrapper.sigBytes(&b)
	b.Write(t.attributes.NewBearer)
	b.Write(util.Uint64ToBytes(t.attributes.Value))
	b.Write(t.attributes.Nonce)
	b.Write(t.attributes.Backlink)
	return b.Bytes()
}

func (s *splitFungibleTokenWrapper) Hash(hashFunc crypto.Hash) []byte {
	if s.wrapper.hashComputed(hashFunc) {
		return s.wrapper.hashValue
	}
	hasher := hashFunc.New()
	s.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(s.attributes.NewBearer)
	hasher.Write(util.Uint64ToBytes(s.attributes.Value))
	hasher.Write(s.attributes.Nonce)
	hasher.Write(s.attributes.Backlink)
	hasher.Write(s.attributes.InvariantPredicateSignature)
	s.wrapper.hashValue = hasher.Sum(nil)
	s.wrapper.hashFunc = hashFunc
	return s.wrapper.hashValue
}

func (s *splitFungibleTokenWrapper) SigBytes() []byte {
	var b bytes.Buffer
	s.wrapper.sigBytes(&b)
	b.Write(s.attributes.NewBearer)
	b.Write(util.Uint64ToBytes(s.attributes.Value))
	b.Write(s.attributes.Nonce)
	b.Write(s.attributes.Backlink)
	return b.Bytes()
}

func (bw *burnFungibleTokenWrapper) Hash(hashFunc crypto.Hash) []byte {
	if bw.wrapper.hashComputed(hashFunc) {
		return bw.wrapper.hashValue
	}
	hasher := hashFunc.New()
	bw.wrapper.addTransactionFieldsToHasher(hasher)
	hasher.Write(bw.attributes.Type)
	hasher.Write(util.Uint64ToBytes(bw.attributes.Value))
	hasher.Write(bw.attributes.Nonce)
	hasher.Write(bw.attributes.Backlink)
	hasher.Write(bw.attributes.InvariantPredicateSignature)
	bw.wrapper.hashValue = hasher.Sum(nil)
	bw.wrapper.hashFunc = hashFunc
	return bw.wrapper.hashValue
}

func (bw *burnFungibleTokenWrapper) SigBytes() []byte {
	var b bytes.Buffer
	bw.wrapper.sigBytes(&b)
	b.Write(bw.attributes.Type)
	b.Write(util.Uint64ToBytes(bw.attributes.Value))
	b.Write(bw.attributes.Nonce)
	b.Write(bw.attributes.Backlink)
	return b.Bytes()
}
