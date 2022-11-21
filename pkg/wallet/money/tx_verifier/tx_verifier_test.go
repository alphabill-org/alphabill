package txverifier

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/stretchr/testify/require"
)

func TestTxVerifier(t *testing.T) {
	pubkey := make([]byte, 32)
	tx := &txsystem.Transaction{
		SystemId:              []byte{0, 0, 0, 0},
		UnitId:                make([]byte, 32),
		TransactionAttributes: testtransaction.CreateBillTransferTx(hash.Sum256(pubkey)),
	}
	gtx, _ := testtransaction.ConvertNewGenericMoneyTx(tx)

	tests := []struct {
		name    string
		gtx     txsystem.GenericTransaction
		key     *wallet.KeyHashes
		wantErr string
	}{
		{name: "tx is nil", gtx: nil, key: nil, wantErr: "tx is nil"},
		{name: "key is nil", gtx: testtransaction.RandomGenericBillTransfer(t), key: nil, wantErr: "key is nil"},
		{name: "transfer invalid bearer predicate", gtx: testtransaction.RandomGenericBillTransfer(t), key: wallet.NewKeyHash([]byte{}), wantErr: "invalid bearer predicate"},
		{name: "transfer ok", gtx: gtx, key: wallet.NewKeyHash(pubkey), wantErr: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyTxP2PKHOwner(tt.gtx, tt.key)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrVerificationFailed)
				require.ErrorContains(t, err, "p2pkh predicate verification failed: "+tt.wantErr)
			}
		})
	}
}
