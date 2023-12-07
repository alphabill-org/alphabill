package backend

import (
	"context"
	"crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	moneytx "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/wallet"
)

func TestWalletBackend_BillsCanBeIndexedByPredicates(t *testing.T) {
	billId1 := newBillID(1)
	billId2 := newBillID(2)
	pubkey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	bearer1 := templates.NewP2pkh256BytesFromKeyHash(util.Sum256(pubkey1))
	bearer2 := templates.NewP2pkh256BytesFromKeyHash(util.Sum256(pubkey2))
	fcbID := newFeeCreditRecordID(101)
	fcb := &Bill{Id: fcbID, Value: 100}

	storage := createTestBillStore(t)

	err := storage.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

	getBlockNumber := func() (uint64, error) { return storage.Do().GetBlockNumber() }

	blockSource := make(chan *types.Block)
	makeBlockAvailable := func(block *types.Block) {
		select {
		case blockSource <- block:
		case <-time.After(1500 * time.Millisecond):
			t.Error("block hasn't been consumed within timeout")
		}
	}

	blockLoader := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
		var blocks [][]byte
		select {
		case b := <-blockSource:
			b.UnicityCertificate.InputRecord.RoundNumber = blockNumber
			blockBytes, err := cbor.Marshal(b)
			if err != nil {
				return nil, err
			}
			blocks = [][]byte{blockBytes}
		default:
			// to signal that block is not available yet return previous BN as Max*
			blockNumber--
		}
		return &alphabill.GetBlocksResponse{
			MaxBlockNumber:      blockNumber,
			MaxRoundNumber:      blockNumber,
			Blocks:              blocks,
			BatchMaxBlockNumber: blockNumber,
		}, nil
	}

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		bp, err := NewBlockProcessor(storage, moneySystemID, logger.New(t))
		require.NoError(t, err)
		err = runBlockSync(ctx, blockLoader, getBlockNumber, 100, bp.ProcessBlock)
		require.ErrorIs(t, err, context.Canceled)
	}()

	makeBlockAvailable(&types.Block{
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
		Transactions: []*types.TransactionRecord{{
			TransactionOrder: &types.TransactionOrder{
				Payload: &types.Payload{
					SystemID:       moneySystemID,
					Type:           moneytx.PayloadTypeTransfer,
					UnitID:         billId1,
					Attributes:     transferTxAttr(util.Sum256(pubkey1)),
					ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
				},
			},
			ServerMetadata: &types.ServerMetadata{ActualFee: 1},
		}},
	})
	// verify first unit is indexed
	require.Eventually(t, func() bool {
		bills, nextKey, err := storage.Do().GetBills(bearer1, true, nil, 100)
		require.NoError(t, err)
		require.Nil(t, nextKey)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)

	// serve block with transaction to new pubkey
	makeBlockAvailable(&types.Block{
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 2}},
		Transactions: []*types.TransactionRecord{{
			TransactionOrder: &types.TransactionOrder{
				Payload: &types.Payload{
					SystemID:       moneySystemID,
					Type:           moneytx.PayloadTypeTransfer,
					UnitID:         billId2,
					Attributes:     transferTxAttr(util.Sum256(pubkey2)),
					ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
				},
			},
			ServerMetadata: &types.ServerMetadata{ActualFee: 1},
		}},
	})

	// verify new bill is indexed by pubkey
	require.Eventually(t, func() bool {
		bills, nextKey, err := storage.Do().GetBills(bearer2, true, nil, 100)
		require.NoError(t, err)
		require.Nil(t, nextKey)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)
}

func TestGetBills_OK(t *testing.T) {
	txValue := uint64(100)
	pubkey := make([]byte, 32)
	bearer := templates.NewP2pkh256BytesFromKeyHash(util.Sum256(pubkey))
	tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(&moneytx.TransferAttributes{
		TargetValue: txValue,
		NewBearer:   bearer,
	}))
	txHash := tx.Hash(crypto.SHA256)

	store := createTestBillStore(t)

	// add bill to service
	service := &WalletBackend{store: store}
	b := &Bill{
		Id:             tx.UnitID(),
		Value:          txValue,
		TxHash:         txHash,
		OwnerPredicate: bearer,
	}
	err := store.Do().SetBill(b, &wallet.Proof{
		TxRecord: &types.TransactionRecord{TransactionOrder: tx},
		TxProof:  &types.TxProof{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}}},
	})
	require.NoError(t, err)

	// verify bill can be queried by id
	bill, err := service.GetBill(b.Id)
	require.NoError(t, err)
	require.Equal(t, b, bill)

	// verify bill can be queried by pubkey
	bills, nextKey, err := service.GetBills(pubkey, true, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
	require.Nil(t, nextKey)
}

func TestGetBills_Paging(t *testing.T) {
	pubkey := test.RandomBytes(32)
	bearerSHA256 := templates.NewP2pkh256BytesFromKeyHash(util.Sum256(pubkey))
	store := createTestBillStore(t)
	service := &WalletBackend{store: store}

	// add sha256 bills
	var billsCount = 20
	var billsSHA256 []*Bill
	for i := 0; i < billsCount; i++ {
		b := newBillWithValueAndOwner(byte(i), bearerSHA256)
		billsSHA256 = append(billsSHA256, b)
		err := store.Do().SetBill(b, nil)
		require.NoError(t, err)
	}

	var allBills []*Bill
	allBills = append(allBills, billsSHA256...)

	// verify all bills can be queried by pubkey
	bills, nextKey, err := service.GetBills(pubkey, true, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, billsCount)
	require.Equal(t, allBills, bills)
	require.Nil(t, nextKey)

	// verify more sha256 bills than limit; return next sha256 key
	bills, nextKey, err = service.GetBills(pubkey, true, nil, 5)
	require.NoError(t, err)
	require.Len(t, bills, 5)
	require.Equal(t, billsSHA256[:5], bills)
	require.Equal(t, billsSHA256[5].Id, nextKey)

	// verify sha256 bills equal to limit; return next key
	bills, nextKey, err = service.GetBills(pubkey, true, nil, 10)
	require.NoError(t, err)
	require.Len(t, bills, 10)
	require.Equal(t, billsSHA256[0:10], bills)
	require.NotEmptyf(t, nextKey, "nextKey should not be empty")
	require.Equal(t, allBills[10].Id, nextKey)

	// verify limit exceeds sha256 bills; return all sha256 bills
	bills, nextKey, err = service.GetBills(pubkey, true, nil, 25)
	require.NoError(t, err)
	require.Len(t, bills, billsCount)
	require.Equal(t, allBills[0:10], bills[0:10])
	require.Equal(t, allBills[10:15], bills[10:15])
	require.Nil(t, nextKey)

	// verify limit equals exact bill count; return all bills and nextKey is nil
	bills, nextKey, err = service.GetBills(pubkey, true, nil, 20)
	require.NoError(t, err)
	require.Len(t, bills, 20)
	require.Equal(t, allBills[0:10], bills[0:10])
	require.Equal(t, allBills[10:20], bills[10:20])
	require.Nil(t, nextKey)
}

func Test_extractOwnerFromProof(t *testing.T) {
	sig := test.RandomBytes(65)
	pubkey := test.RandomBytes(33)
	predicate := templates.NewP2pkh256SignatureBytes(sig, pubkey)
	owner := extractOwnerKeyFromProof(predicate)
	require.EqualValues(t, pubkey, owner)
}
