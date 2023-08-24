package backend

import (
	"context"
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

func TestWalletBackend_BillsCanBeIndexedByPredicates(t *testing.T) {
	// create wallet backend with mock abclient
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	billId1 := newUnitID(1)
	billId2 := newUnitID(2)
	pubkey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	bearer1 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey1))
	bearer2 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey2))
	fcbID := newUnitID(101)
	fcb := &Bill{Id: fcbID, Value: 100}

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

	err := storage.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

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
		bills, nextKey, err := storage.Do().GetBills(bearer1, nil, 100)
		require.NoError(t, err)
		require.Nil(t, nextKey)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)

	// serve block with transaction to new pubkey
	abclient.SetMaxBlockNumber(2)

	// verify new bill is indexed by pubkey
	require.Eventually(t, func() bool {
		bills, nextKey, err := storage.Do().GetBills(bearer2, nil, 100)
		require.NoError(t, err)
		require.Nil(t, nextKey)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)
}

func TestGetBills_OK(t *testing.T) {
	txValue := uint64(100)
	pubkey := make([]byte, 32)
	bearer := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey))
	tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(&moneytx.TransferAttributes{
		TargetValue: txValue,
		NewBearer:   bearer,
	}))
	txHash := tx.Hash(gocrypto.SHA256)

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
	bills, nextKey, err := service.GetBills(pubkey, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
	require.Nil(t, nextKey)
}

func TestGetBills_SHA512_OK(t *testing.T) {
	pubkey := test.RandomBytes(32)
	bearer := script.PredicatePayToPublicKeyHash(script.HashAlgSha512, hash.Sum512(pubkey), script.SigSchemeSecp256k1)
	store := createTestBillStore(t)

	// add sha512 owner condition bill to service
	service := &WalletBackend{store: store}
	b := &Bill{
		Id:             []byte{1},
		Value:          100,
		OwnerPredicate: bearer,
	}
	err := store.Do().SetBill(b, nil)
	require.NoError(t, err)

	// verify bill can be queried by pubkey
	bills, nextKey, err := service.GetBills(pubkey, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
	require.Nil(t, nextKey)
}

func TestGetBills_Paging(t *testing.T) {
	pubkey := test.RandomBytes(32)
	bearerSHA256 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey))
	bearerSHA512 := script.PredicatePayToPublicKeyHash(script.HashAlgSha512, hash.Sum512(pubkey), script.SigSchemeSecp256k1)
	store := createTestBillStore(t)
	service := &WalletBackend{store: store}

	// add 10 sha256 bills
	var billsSHA256 []*Bill
	for i := 0; i < 10; i++ {
		b := newBillWithValueAndOwner(uint64(i), bearerSHA256)
		billsSHA256 = append(billsSHA256, b)
		err := store.Do().SetBill(b, nil)
		require.NoError(t, err)
	}

	// add 10 sha512 bills
	var billsSHA512 []*Bill
	for i := 10; i < 20; i++ {
		b := newBillWithValueAndOwner(uint64(i), bearerSHA512)
		billsSHA512 = append(billsSHA512, b)
		err := store.Do().SetBill(b, nil)
		require.NoError(t, err)
	}

	var allBills []*Bill
	allBills = append(allBills, billsSHA256...)
	allBills = append(allBills, billsSHA512...)

	// verify all bills can be queried by pubkey
	bills, nextKey, err := service.GetBills(pubkey, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 20)
	require.Equal(t, allBills, bills)
	require.Nil(t, nextKey)

	// verify more sha256 bills than limit; return next sha256 key
	bills, nextKey, err = service.GetBills(pubkey, nil, 5)
	require.NoError(t, err)
	require.Len(t, bills, 5)
	require.Equal(t, billsSHA256[:5], bills)
	require.Equal(t, billsSHA256[5].Id, nextKey)

	// verify sha256 bills equal to limit; return next sha512 key
	bills, nextKey, err = service.GetBills(pubkey, nil, 10)
	require.NoError(t, err)
	require.Len(t, bills, 10)
	require.Equal(t, billsSHA256, bills)
	require.Equal(t, billsSHA512[0].Id, nextKey)

	// verify limit exceeds sha256 bills; return all sha256 bills and some sha512 bills
	bills, nextKey, err = service.GetBills(pubkey, nil, 15)
	require.NoError(t, err)
	require.Len(t, bills, 15)
	require.Equal(t, allBills[0:10], bills[0:10])
	require.Equal(t, allBills[10:15], bills[10:15])
	require.Equal(t, allBills[15].Id, nextKey)

	// verify limit equals exact bill count; return all bills and nextKey is nil
	bills, nextKey, err = service.GetBills(pubkey, nil, 20)
	require.NoError(t, err)
	require.Len(t, bills, 20)
	require.Equal(t, allBills[0:10], bills[0:10])
	require.Equal(t, allBills[10:20], bills[10:20])
	require.Nil(t, nextKey)
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
