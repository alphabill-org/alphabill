package transaction

import (
	"bytes"
	"crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
)

type registerDataWrapper struct {
	wrapper
	reg *RegisterData
}

func NewVerifiableDataTx(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	switch tx.TransactionAttributes.TypeUrl {
	case protobufTypeUrlPrefix + "RegisterData":
		pb := &RegisterData{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &registerDataWrapper{
			wrapper: wrapper{transaction: tx},
			reg:     pb,
		}, nil
	default:
		return nil, errors.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}

func (r *registerDataWrapper) Hash(hashFunc crypto.Hash) []byte {
	if r.wrapper.hashComputed(hashFunc) {
		return r.wrapper.hashValue
	}
	hasher := hashFunc.New()
	r.wrapper.addTransactionFieldsToHasher(hasher)
	r.wrapper.hashValue = hasher.Sum(nil)
	r.wrapper.hashFunc = hashFunc
	return r.wrapper.hashValue
}

func (r *registerDataWrapper) SigBytes() []byte {
	var b bytes.Buffer
	r.wrapper.sigBytes(b)
	return b.Bytes()
}
