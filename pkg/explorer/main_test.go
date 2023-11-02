package explorer

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

func TestWalletBackend_BillsCanBeIndexedByPredicates(t *testing.T) {
	// create wallet backend with mock abclient
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	billId1 := newBillID(1)
	billId2 := newBillID(2)
	pubkey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	bearer1 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey1))
	bearer2 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey2))
	fcbID := newFeeCreditRecordID(101)

	abclient := clientmock.NewMockAlphabillClient(
		clientmock.WithMaxBlockNumber(1),
		clientmock.WithBlocks(map[uint64]*types.Block{
			1: {
				UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
				Transactions: []*types.TransactionRecord{{
					TransactionOrder: &types.TransactionOrder{
						Payload: &types.Payload{
							SystemID:       moneySystemID,
							Type:           moneytx.PayloadTypeTransfer,
							UnitID:         billId1,
							Attributes:     transferTxAttr(hash.Sum256(pubkey1)),
							ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
						},
					},
					ServerMetadata: &types.ServerMetadata{ActualFee: 1},
				}},
			},
			2: {
				UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 2}},
				Transactions: []*types.TransactionRecord{{
					TransactionOrder: &types.TransactionOrder{
						Payload: &types.Payload{
							SystemID:       moneySystemID,
							Type:           moneytx.PayloadTypeTransfer,
							UnitID:         billId2,
							Attributes:     transferTxAttr(hash.Sum256(pubkey2)),
							ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
						},
					},
					ServerMetadata: &types.ServerMetadata{ActualFee: 1},
				}},
			},
		}))
	storage := createTestBillStore(t)

	getBlockNumber := func() (uint64, error) { return storage.Do().GetBlockNumber() }

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		bp, err := NewBlockProcessor(storage, moneySystemID)
		require.NoError(t, err)
		err = runBlockSync(ctx, abclient.GetBlocks, getBlockNumber, 100, bp.ProcessBlock)
		require.ErrorIs(t, err, context.Canceled)
	}()

	// verify first unit is indexed
	require.Eventually(t, func() bool {
		bills, nextKey, err := storage.Do().GetBills(bearer1, true, nil, 100)
		require.NoError(t, err)
		require.Nil(t, nextKey)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)

	// serve block with transaction to new pubkey
	abclient.SetMaxBlockNumber(2)

	// verify new bill is indexed by pubkey
	require.Eventually(t, func() bool {
		bills, nextKey, err := storage.Do().GetBills(bearer2, true, nil, 100)
		require.NoError(t, err)
		require.Nil(t, nextKey)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)
}

func Test_extractOwnerFromProof(t *testing.T) {
	sig := test.RandomBytes(65)
	pubkey := test.RandomBytes(33)
	predicate := script.PredicateArgumentPayToPublicKeyHashDefault(sig, pubkey)
	owner := extractOwnerKeyFromProof(predicate)
	require.EqualValues(t, pubkey, owner)
	k1 := "0x535401099C0E835B9C9B62CA5E321E796D445DDBD80626BA600EBBCF3C647D7FBD2FFE2758ACC7476B38783365B7274A385EFC7CFFC3EB63A696928EBDBD4D5516078F00550103D8242EE6544CC1F98EA8B8E572D089A95917800A37E3207A725DECDAD6002E52"
	k2 := "0x535401090300B7CDC7F75A9E51998EA5C52A7014979BAD2205C8DD0552BF16C4F273E0723CD87CD6C4C8AEF33E7B659EE8FD897D6E8A1CEB89E8E4C2C94ACEB2FEF7CF00550102E2CF1D2C5E0CD7EFD27DBA8B21225E6866B185E97E8A415ACC6FC9CB57743B3A"
	k1bytes, err := hexutil.Decode(k1)
	require.NoError(t, err)
	k2bytes, err := hexutil.Decode(k2)
	require.NoError(t, err)
	owner1 := extractOwnerKeyFromProof(k1bytes)
	require.NotNil(t, owner1)
	owner2 := extractOwnerKeyFromProof(k2bytes)
	require.NotNil(t, owner2)
}
