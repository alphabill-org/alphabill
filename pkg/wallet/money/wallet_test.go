package money

import (
	"context"
	"crypto"
	"encoding/hex"
	"os"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"github.com/tyler-smith/go-bip39"
)

const (
	testMnemonic                 = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"
	testMasterKeyBase58          = "xprv9s21ZrQH143K4ZSw4N2P35FTs9PNiLAuufvQWoodWoneZ71o52jTL4VJuEXHej21BPUF9dQm5u3curjcem5zsARtq1MKP9mrbbq1qKqyuFX"
	testPubKey0Hex               = "03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	testPubKey1Hex               = "02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
	testPubKey2Hex               = "02f6cbeacfd97ebc9b657081eb8b6c9ed3a588646d618ddbd03e198290af94c9d2"
	testPrivKey0Hex              = "70096ea8536cfba71203a959ed7de2a5900c5547762606f73b2aa078a66e355f"
	testPubKey0HashSha256Hex     = "f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"
	testPubKey0HashSha512Hex     = "9254c2cb897e4b458ba51eb200015e301d578cd5572b75068dfd1332f8200d15b2642f623d5a5d8e8174508e93b22c75801ea95bf9be479de90aaf2776171251"
	testAccountKeyDerivationPath = "m/44'/634'/0'/0/0"
)

func TestWalletCanBeCreated(t *testing.T) {
	w, _ := CreateTestWallet(t)

	balance, err := w.GetBalance(0)
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)

	mnemonic, err := w.db.Do().GetMnemonic()
	require.NoError(t, err)
	require.True(t, bip39.IsMnemonicValid(mnemonic))

	masterKeyString, err := w.db.Do().GetMasterKey()
	require.NoError(t, err)
	masterKey, err := hdkeychain.NewKeyFromString(masterKeyString)
	require.NoError(t, err)

	ac, err := w.db.Do().GetAccountKey(0)
	require.NoError(t, err)

	eac, err := wallet.NewAccountKey(masterKey, testAccountKeyDerivationPath)
	require.NoError(t, err)
	require.NotNil(t, eac)
	require.EqualValues(t, eac, ac)
}

func TestExistingWalletCanBeLoaded(t *testing.T) {
	walletDbPath, err := CopyWalletDBFile(t)
	require.NoError(t, err)

	w, err := LoadExistingWallet(WalletConfig{DbPath: walletDbPath})
	require.NoError(t, err)
	t.Cleanup(func() {
		w.Shutdown()
	})

	verifyTestWallet(t, w)
}

func TestWalletCanBeCreatedFromSeed(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)
	verifyTestWallet(t, w)
}

func TestWalletSendFunction(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	invalidPubKey := make([]byte, 32)
	validPubKey := make([]byte, 33)
	amount := uint64(50)

	// test ErrInvalidPubKey
	err := w.Send(invalidPubKey, amount, 0)
	require.ErrorIs(t, err, ErrInvalidPubKey)

	// test ErrInsufficientBalance
	err = w.Send(validPubKey, amount, 0)
	require.ErrorIs(t, err, ErrInsufficientBalance)

	// test abclient returns error
	b := bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.Do().SetBill(0, &b)
	require.NoError(t, err)
	mockClient.SetTxResponse(&txsystem.TransactionResponse{Ok: false, Message: "some error"})
	err = w.Send(validPubKey, amount, 0)
	require.ErrorContains(t, err, "payment returned error code: some error")
	mockClient.SetTxResponse(nil)

	// test ErrSwapInProgress
	nonce := calculateExpectedDcNonce(t, w)
	setDcMetadata(t, w, nonce, &dcMetadata{DcValueSum: 101, DcTimeout: dcTimeoutBlockCount})
	err = w.Send(validPubKey, amount, 0)
	require.ErrorIs(t, err, ErrSwapInProgress)
	setDcMetadata(t, w, nonce, nil)

	// test ok response
	err = w.Send(validPubKey, amount, 0)
	require.NoError(t, err)

	// test another account
	_, _, _ = w.AddAccount()
	_ = w.db.Do().SetBill(1, &bill{Id: uint256.NewInt(55555), Value: 50})
	err = w.Send(validPubKey, amount, 1)
	require.NoError(t, err)
}

func TestWallet_GetPublicKey(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)
	pubKey, err := w.GetPublicKey(0)
	require.NoError(t, err)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKey))
}

func TestWallet_GetPublicKeys(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)
	_, _, _ = w.AddAccount()

	pubKeys, err := w.GetPublicKeys()
	require.NoError(t, err)
	require.Len(t, pubKeys, 2)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKeys[0]))
	require.EqualValues(t, "0x"+testPubKey1Hex, hexutil.Encode(pubKeys[1]))
}

func TestWallet_AddKey(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)

	accIdx, accPubKey, err := w.AddAccount()
	require.NoError(t, err)
	require.EqualValues(t, 1, accIdx)
	require.EqualValues(t, "0x"+testPubKey1Hex, hexutil.Encode(accPubKey))
	accIdx, _ = w.db.Do().GetMaxAccountIndex()
	require.EqualValues(t, 1, accIdx)

	accIdx, accPubKey, err = w.AddAccount()
	require.NoError(t, err)
	require.EqualValues(t, 2, accIdx)
	require.EqualValues(t, "0x"+testPubKey2Hex, hexutil.Encode(accPubKey))
	accIdx, _ = w.db.Do().GetMaxAccountIndex()
	require.EqualValues(t, 2, accIdx)
}

func TestWallet_GetBalance(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)
	balance, err := w.GetBalance(0)
	require.NoError(t, err)
	require.EqualValues(t, 0, balance)
}

func TestWallet_GetBalances(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)
	_ = w.db.Do().SetBill(0, &bill{Id: uint256.NewInt(0), Value: 1})
	_ = w.db.Do().SetBill(0, &bill{Id: uint256.NewInt(1), Value: 1})

	_, _, _ = w.AddAccount()
	_ = w.db.Do().SetBill(1, &bill{Id: uint256.NewInt(2), Value: 2})
	_ = w.db.Do().SetBill(1, &bill{Id: uint256.NewInt(3), Value: 2})

	balances, err := w.GetBalances()
	require.NoError(t, err)
	require.EqualValues(t, 2, balances[0])
	require.EqualValues(t, 4, balances[1])
}

func TestWalletIsClosedAfterCallingIsEncrypted(t *testing.T) {
	// create and shutdown wallet
	w, _ := CreateTestWallet(t)
	w.Shutdown()

	// call IsEncrypted
	_, err := IsEncrypted(w.config)
	require.NoError(t, err)

	// when wallet is loaded
	w, err = LoadExistingWallet(w.config)
	require.NoError(t, err)

	// then using wallet db should not hang
	_, err = w.GetPublicKey(0)
	require.NoError(t, err)
}

func TestBlockProcessing(t *testing.T) {
	w, _ := CreateTestWallet(t)

	k, err := w.db.Do().GetAccountKey(0)
	require.NoError(t, err)

	blocks := []*block.Block{
		{
			SystemIdentifier:  alphabillMoneySystemId,
			BlockNumber:       1,
			PreviousBlockHash: hash.Sum256([]byte{}),
			Transactions: []*txsystem.Transaction{
				// random dust transfer can be processed
				{
					SystemId:              alphabillMoneySystemId,
					UnitId:                hash.Sum256([]byte{0x00}),
					TransactionAttributes: testtransaction.CreateRandomDustTransferTx(),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentEmpty(),
				},
				// receive transfer of 100 bills
				{
					SystemId:              alphabillMoneySystemId,
					UnitId:                hash.Sum256([]byte{0x01}),
					TransactionAttributes: testtransaction.CreateBillTransferTx(k.PubKeyHash.Sha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive split of 100 bills
				{
					SystemId:              alphabillMoneySystemId,
					UnitId:                hash.Sum256([]byte{0x02}),
					TransactionAttributes: testtransaction.CreateBillSplitTx(k.PubKeyHash.Sha256, 100, 100),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive swap of 100 bills
				{
					SystemId:              alphabillMoneySystemId,
					UnitId:                hash.Sum256([]byte{0x03}),
					TransactionAttributes: testtransaction.CreateRandomSwapTransferTx(k.PubKeyHash.Sha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: &certificates.UnicityCertificate{},
		},
	}

	// verify block number 0 before processing
	blockNumber, err := w.db.Do().GetBlockNumber()
	require.EqualValues(t, 0, blockNumber)
	require.NoError(t, err)

	// verify balance 0 before processing
	balance, err := w.db.Do().GetBalance(0)
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)

	// process blocks
	for _, b := range blocks {
		err = w.ProcessBlock(b)
		require.NoError(t, err)
	}

	// verify block number after block processing
	blockNumber, err = w.db.Do().GetBlockNumber()
	require.EqualValues(t, 1, blockNumber)
	require.NoError(t, err)

	// verify balance after block processing
	balance, err = w.db.Do().GetBalance(0)
	require.EqualValues(t, 300, balance)
	require.NoError(t, err)
}

func TestBlockProcessing_InvalidSystemID(t *testing.T) {
	w, _ := CreateTestWallet(t)

	b := &block.Block{
		SystemIdentifier:   []byte{0, 0, 0, 1},
		BlockNumber:        1,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}

	err := w.ProcessBlock(b)
	require.ErrorContains(t, err, "invalid system identifier")
}

func TestBlockProcessing_VerifyBlockProofs(t *testing.T) {
	w, _ := CreateTestWallet(t)
	k, _ := w.db.Do().GetAccountKey(0)

	testBlock := &block.Block{
		SystemIdentifier:  alphabillMoneySystemId,
		BlockNumber:       1,
		PreviousBlockHash: hash.Sum256([]byte{}),
		Transactions: []*txsystem.Transaction{
			// receive transfer of 100 bills
			{
				SystemId:              alphabillMoneySystemId,
				UnitId:                hash.Sum256([]byte{0x00}),
				TransactionAttributes: testtransaction.CreateBillTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive dc transfer of 100 bills
			{
				SystemId:              alphabillMoneySystemId,
				UnitId:                hash.Sum256([]byte{0x01}),
				TransactionAttributes: testtransaction.CreateDustTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive split of 100 bills
			{
				SystemId:              alphabillMoneySystemId,
				UnitId:                hash.Sum256([]byte{0x02}),
				TransactionAttributes: testtransaction.CreateBillSplitTx(k.PubKeyHash.Sha256, 100, 100),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive swap of 100 bills
			{
				SystemId:              alphabillMoneySystemId,
				UnitId:                hash.Sum256([]byte{0x03}),
				TransactionAttributes: testtransaction.CreateRandomSwapTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
		},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}

	err := w.ProcessBlock(testBlock)
	require.NoError(t, err)

	bills, err := w.db.Do().GetBills(0)
	require.NoError(t, err)
	require.Len(t, bills, 4)

	for _, b := range bills {
		require.NotNil(t, b.BlockProof)
	}
}

func TestWholeBalanceIsSentUsingBillTransferOrder(t *testing.T) {
	// create wallet with single bill
	w, mockClient := CreateTestWallet(t)
	addBill(t, w, 100)
	receiverPubKey := make([]byte, 33)

	// when whole balance is spent
	err := w.Send(receiverPubKey, 100, 0)
	require.NoError(t, err)

	// then bill transfer order should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	btTx := parseBillTransferTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 100, btTx.TargetValue)
}

func TestSyncOnClosedWalletShouldNotHang(t *testing.T) {
	w, _ := CreateTestWallet(t)
	addBill(t, w, 100)

	// when wallet is closed
	w.Shutdown()

	// and Sync is called
	err := w.Sync(context.Background())
	require.ErrorContains(t, err, "database not open")
}

func TestWalletDbIsNotCreatedOnWalletCreationError(t *testing.T) {
	// create wallet with invalid seed
	_ = DeleteWalletDb(os.TempDir())
	c := WalletConfig{DbPath: os.TempDir()}
	invalidSeed := "this pond palace oblige remind glory lens popular iron decide coral"
	_, err := CreateNewWallet(invalidSeed, c)
	require.ErrorContains(t, err, "invalid mnemonic")

	// verify database is not created
	require.False(t, util.FileExists(path.Join(os.TempDir(), walletFileName)))
}

func verifyTestWallet(t *testing.T, w *Wallet) {
	mnemonic, err := w.db.Do().GetMnemonic()
	require.NoError(t, err)
	require.Equal(t, testMnemonic, mnemonic)

	mk, err := w.db.Do().GetMasterKey()
	require.Equal(t, testMasterKeyBase58, mk)

	ac, err := w.db.Do().GetAccountKey(0)
	require.NoError(t, err)
	require.Equal(t, testPubKey0Hex, hex.EncodeToString(ac.PubKey))
	require.Equal(t, testPrivKey0Hex, hex.EncodeToString(ac.PrivKey))
	require.Equal(t, testPubKey0HashSha256Hex, hex.EncodeToString(ac.PubKeyHash.Sha256))
	require.Equal(t, testPubKey0HashSha512Hex, hex.EncodeToString(ac.PubKeyHash.Sha512))
}

func createMerkleTree(blockTxs []*txsystem.Transaction) (*mt.MerkleTree, error) {
	// create merkle tree from testBlock transactions
	txs := make([]mt.Data, len(blockTxs))
	for i, tx := range blockTxs {
		txs[i] = &mt.ByteHasher{Val: tx.Bytes()}
	}
	return mt.New(crypto.SHA256, txs)
}
