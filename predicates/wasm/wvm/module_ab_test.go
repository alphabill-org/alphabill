package wvm

import (
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
)

func Test_amountTransferredSum(t *testing.T) {
	// trustbase which "successfully" verifies all tx proofs (ie says they're valid)
	trustBaseOK := &mockRootTrustBase{
		// need VerifyQuorumSignatures for verifying tx proofs
		verifyQuorumSignatures: func(data []byte, signatures map[string][]byte) (error, []error) { return nil, nil },
	}
	tbSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	// public key hashes
	pkhA := []byte{3, 8, 0, 1, 2, 4, 5}
	pkhB := []byte{3, 8, 0, 1, 2, 4, 0}
	// create mix of tx types
	// add an invalid proof record - just tx record, proof is missing
	proofs := []types.TxRecordProof{{TxRecord: &types.TransactionRecord{}, TxProof: nil}}
	// valid money transfer
	txPayment := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID: money.DefaultSystemID,
			Type:     money.PayloadTypeTransfer,
		},
	}
	txPayment.Payload.SetAttributes(money.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKeyHash(pkhA),
		TargetValue: 100,
	})

	txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
	proofs = append(proofs, types.TxRecordProof{
		TxRecord: txRec,
		TxProof:  testblock.CreateProof(t, txRec, tbSigner, testblock.WithSystemIdentifier(money.DefaultSystemID)),
	})

	// money transfer by split tx
	txPayment = &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID: money.DefaultSystemID,
			Type:     money.PayloadTypeSplit,
		},
	}
	txPayment.Payload.SetAttributes(money.SplitAttributes{
		TargetUnits: []*money.TargetUnit{
			{Amount: 10, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(pkhA)},
			{Amount: 50, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(pkhB)},
			{Amount: 90, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(pkhA)},
		},
		RemainingValue: 2000,
	})

	txRec = &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
	proofs = append(proofs, types.TxRecordProof{
		TxRecord: txRec,
		TxProof:  testblock.CreateProof(t, txRec, tbSigner, testblock.WithSystemIdentifier(money.DefaultSystemID)),
	})

	// because of invalid proof record we expect error but pkhA should receive
	// total of 200 (transfer=100 + split=10+90)
	sum, err := amountTransferredSum(trustBaseOK, proofs, pkhA, nil)
	require.EqualError(t, err, `record[0]: invalid input: either trustbase, tx proof, tx record or tx order is unassigned`)
	require.EqualValues(t, 200, sum)
	// pkhB should get 50 from split
	sum, err = amountTransferredSum(trustBaseOK, proofs, pkhB, nil)
	require.EqualError(t, err, `record[0]: invalid input: either trustbase, tx proof, tx record or tx order is unassigned`)
	require.EqualValues(t, 50, sum)
	// nil as pkh
	sum, err = amountTransferredSum(trustBaseOK, proofs[1:], nil, nil)
	require.NoError(t, err)
	require.Zero(t, sum)
}

func Test_transferredSum(t *testing.T) {
	// trustbase which "successfully" verifies all tx proofs (ie says they're valid)
	trustBaseOK := &mockRootTrustBase{
		// need VerifyQuorumSignatures for verifying tx proofs
		verifyQuorumSignatures: func(data []byte, signatures map[string][]byte) (error, []error) { return nil, nil },
	}

	t.Run("invalid input, required argument is nil", func(t *testing.T) {
		txRec := &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{}}

		sum, err := transferredSum(nil, txRec, &types.TxProof{}, nil, nil)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: either trustbase, tx proof, tx record or tx order is unassigned`)

		sum, err = transferredSum(trustBaseOK, nil, &types.TxProof{}, nil, nil)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: either trustbase, tx proof, tx record or tx order is unassigned`)

		sum, err = transferredSum(trustBaseOK, &types.TransactionRecord{TransactionOrder: nil}, &types.TxProof{}, nil, nil)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: either trustbase, tx proof, tx record or tx order is unassigned`)

		sum, err = transferredSum(trustBaseOK, txRec, nil, nil, nil)
		require.Zero(t, sum)
		require.EqualError(t, err, `invalid input: either trustbase, tx proof, tx record or tx order is unassigned`)
	})

	t.Run("tx for non-money txsystem", func(t *testing.T) {
		// money system ID is 1, create tx for some other txs
		txRec := &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: &types.Payload{SystemID: 2}}}
		sum, err := transferredSum(&mockRootTrustBase{}, txRec, &types.TxProof{}, nil, nil)
		require.Zero(t, sum)
		require.EqualError(t, err, `expected partition id 1 got 2`)
	})

	t.Run("ref number mismatch", func(t *testing.T) {
		// if ref-no parameter is provided it must match (nil ref-no means "do not care")
		txRec := &types.TransactionRecord{
			TransactionOrder: &types.TransactionOrder{
				Payload: &types.Payload{
					SystemID: money.DefaultSystemID,
					Type:     money.PayloadTypeTransfer,
					ClientMetadata: &types.ClientMetadata{
						ReferenceNumber: nil,
					},
				},
			},
		}
		refNo := []byte{1, 2, 3, 4, 5}
		// txRec.ReferenceNumber == nil but refNo param != nil
		sum, err := transferredSum(&mockRootTrustBase{}, txRec, &types.TxProof{}, nil, refNo)
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)

		// txRec.ReferenceNumber != refNo (we add extra zero to the end)
		txRec.TransactionOrder.Payload.ClientMetadata.ReferenceNumber = slices.Concat(refNo, []byte{0})
		sum, err = transferredSum(&mockRootTrustBase{}, txRec, &types.TxProof{}, nil, refNo)
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)
	})

	t.Run("valid input but not transfer tx", func(t *testing.T) {
		// all money tx types other than PayloadTypeSplit and PayloadTypeTransfer should
		// be ignored ie cause no error but return zero as sum
		txTypes := []string{money.PayloadTypeLock, money.PayloadTypeSwapDC, money.PayloadTypeTransDC, money.PayloadTypeUnlock}
		txRec := &types.TransactionRecord{
			TransactionOrder: &types.TransactionOrder{},
		}
		for _, txt := range txTypes {
			txRec.TransactionOrder.Payload = &types.Payload{
				SystemID: money.DefaultSystemID,
				Type:     txt,
			}
			sum, err := transferredSum(&mockRootTrustBase{}, txRec, &types.TxProof{}, nil, nil)
			require.NoError(t, err)
			require.Zero(t, sum)
		}
	})

	t.Run("txType and attributes do not match", func(t *testing.T) {
		txPayment := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: money.DefaultSystemID,
				Type:     money.PayloadTypeSplit,
			},
		}
		pkHash := []byte{3, 8, 0, 1, 2, 4, 5}
		// txType is Split but use Transfer attributes!
		txPayment.Payload.SetAttributes(money.TransferAttributes{
			NewBearer:   templates.NewP2pkh256BytesFromKeyHash(pkHash),
			TargetValue: 100,
		})
		txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}

		sum, err := transferredSum(&mockRootTrustBase{}, txRec, &types.TxProof{}, nil, nil)
		require.EqualError(t, err, `decoding split attributes: cbor: cannot unmarshal byte string into Go struct field money.SplitAttributes.TargetUnits of type []*money.TargetUnit`)
		require.Zero(t, sum)
	})

	tbSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	t.Run("transfer tx", func(t *testing.T) {
		refNo := []byte("reasons")
		txPayment := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: money.DefaultSystemID,
				Type:     money.PayloadTypeTransfer,
				ClientMetadata: &types.ClientMetadata{
					ReferenceNumber: slices.Clone(refNo),
				},
			},
		}
		pkHash := []byte{3, 8, 0, 1, 2, 4, 5}
		txPayment.Payload.SetAttributes(money.TransferAttributes{
			NewBearer:   templates.NewP2pkh256BytesFromKeyHash(pkHash),
			TargetValue: 100,
		})

		txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
		proof := testblock.CreateProof(t, txRec, tbSigner, testblock.WithSystemIdentifier(money.DefaultSystemID))

		// match without ref-no
		sum, err := transferredSum(trustBaseOK, txRec, proof, pkHash, nil)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// match with ref-no
		sum, err = transferredSum(trustBaseOK, txRec, proof, pkHash, refNo)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// different PKH, should get zero
		sum, err = transferredSum(trustBaseOK, txRec, proof, []byte{1, 1, 1, 1, 1}, refNo)
		require.NoError(t, err)
		require.EqualValues(t, 0, sum)
		// sum where ref-no is not set, should get zero as our transfer does have ref-no
		sum, err = transferredSum(trustBaseOK, txRec, proof, pkHash, []byte{})
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)
		// transfer does not verify
		errNOK := errors.New("this is bogus")
		tbNOK := &mockRootTrustBase{
			// need VerifyQuorumSignatures for verifying tx proofs
			verifyQuorumSignatures: func(data []byte, signatures map[string][]byte) (error, []error) { return errNOK, nil },
		}
		sum, err = transferredSum(tbNOK, txRec, proof, pkHash, nil)
		require.ErrorIs(t, err, errNOK)
		require.Zero(t, sum)
	})

	t.Run("split tx", func(t *testing.T) {
		refNo := []byte("reasons")
		txPayment := &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: money.DefaultSystemID,
				Type:     money.PayloadTypeSplit,
				ClientMetadata: &types.ClientMetadata{
					ReferenceNumber: slices.Clone(refNo),
				},
			},
		}
		pkHash := []byte{3, 8, 0, 1, 2, 4, 5}
		txPayment.Payload.SetAttributes(money.SplitAttributes{
			TargetUnits: []*money.TargetUnit{
				{Amount: 10, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(pkHash)},
				{Amount: 50, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash([]byte("other guy"))},
				{Amount: 90, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(pkHash)},
			},
			RemainingValue: 2000,
		})

		txRec := &types.TransactionRecord{TransactionOrder: txPayment, ServerMetadata: &types.ServerMetadata{ActualFee: 25}}
		proof := testblock.CreateProof(t, txRec, tbSigner, testblock.WithSystemIdentifier(money.DefaultSystemID))

		// match without ref-no
		sum, err := transferredSum(trustBaseOK, txRec, proof, pkHash, nil)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// match with ref-no
		sum, err = transferredSum(trustBaseOK, txRec, proof, pkHash, refNo)
		require.NoError(t, err)
		require.EqualValues(t, 100, sum)
		// PKH not in use by any units, should get zero
		sum, err = transferredSum(trustBaseOK, txRec, proof, []byte{1, 1, 1, 1, 1}, refNo)
		require.NoError(t, err)
		require.EqualValues(t, 0, sum)
		// the other guy
		sum, err = transferredSum(trustBaseOK, txRec, proof, []byte("other guy"), nil)
		require.NoError(t, err)
		require.EqualValues(t, 50, sum)
		// sum where ref-no is not set, should get zero as our transfer does have ref-no
		sum, err = transferredSum(trustBaseOK, txRec, proof, pkHash, []byte{})
		require.Zero(t, sum)
		require.EqualError(t, err, `reference number mismatch`)
		// transfer does not verify
		errNOK := errors.New("this is bogus")
		tbNOK := &mockRootTrustBase{
			// need VerifyQuorumSignatures for verifying tx proofs
			verifyQuorumSignatures: func(data []byte, signatures map[string][]byte) (error, []error) { return errNOK, nil },
		}
		sum, err = transferredSum(tbNOK, txRec, proof, pkHash, nil)
		require.ErrorIs(t, err, errNOK)
		require.Zero(t, sum)
	})
}
