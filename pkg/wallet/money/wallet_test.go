package money

import (
	"bytes"
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
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
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
	testPubKeyHex                = "0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"
	testPrivKeyHex               = "a5e8bff9733ebc751a45ca4b8cc6ce8e76c8316a5eb556f738092df6232e78de"
	testPubKeyHashSha256Hex      = "35e22185db8875c75af23b334676e76b723f10eba2a314b2c64d71c0299b5e3a"
	testPubKeyHashSha512Hex      = "be7a5c70c13611ea4cdc075c50e6a1cbbca54fc7acf6e330436d04d91063eab80e8f45864895f793d5bc5fb2c57c6813e3dd8b773286d716938537f0a9794b2e"
	testAccountKeyDerivationPath = "m/44'/634'/0'/0/0"
)

func TestWalletCanBeCreated(t *testing.T) {
	w, _ := CreateTestWallet(t)

	balance, err := w.GetBalance()
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)

	mnemonic, err := w.db.Do().GetMnemonic()
	require.NoError(t, err)
	require.True(t, bip39.IsMnemonicValid(mnemonic))

	masterKeyString, err := w.db.Do().GetMasterKey()
	require.NoError(t, err)
	masterKey, err := hdkeychain.NewKeyFromString(masterKeyString)
	require.NoError(t, err)

	ac, err := w.db.Do().GetAccountKey()
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
	err := w.Send(invalidPubKey, amount)
	require.ErrorIs(t, err, ErrInvalidPubKey)

	// test ErrInsufficientBalance
	err = w.Send(validPubKey, amount)
	require.ErrorIs(t, err, ErrInsufficientBalance)

	// test abclient returns error
	b := bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.Do().SetBill(&b)
	require.NoError(t, err)
	mockClient.txResponse = &txsystem.TransactionResponse{Ok: false, Message: "some error"}
	err = w.Send(validPubKey, amount)
	require.ErrorContains(t, err, "payment returned error code: some error")
	mockClient.txResponse = nil

	// test ErrSwapInProgress
	nonce := calculateExpectedDcNonce(t, w)
	setDcMetadata(t, w, nonce, &dcMetadata{DcValueSum: 101, DcTimeout: dcTimeoutBlockCount})
	err = w.Send(validPubKey, amount)
	require.ErrorIs(t, err, ErrSwapInProgress)
	setDcMetadata(t, w, nonce, nil)

	// test ok response
	err = w.Send(validPubKey, amount)
	require.NoError(t, err)
}

func TestWallet_GetPublicKey(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)
	pubKey, err := w.GetPublicKey()
	require.NoError(t, err)
	require.EqualValues(t, "0x"+testPubKeyHex, hexutil.Encode(pubKey))
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
	_, err = w.GetPublicKey()
	require.NoError(t, err)
}

func TestBlockProcessing(t *testing.T) {
	w, _ := CreateTestWallet(t)

	k, err := w.db.Do().GetAccountKey()
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
	balance, err := w.db.Do().GetBalance()
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
	balance, err = w.db.Do().GetBalance()
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
	k, _ := w.db.Do().GetAccountKey()

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

	merkleTree, err := createMerkleTree(testBlock.Transactions)
	require.NoError(t, err)

	bills, err := w.db.Do().GetBills()
	require.NoError(t, err)
	require.Len(t, bills, 4)

	for _, b := range bills {
		// verify block proofs
		require.NotNil(t, b.BlockProof)
		merklePath := mt.FromProtobuf(b.BlockProof.MerkleProof)
		rootHash := mt.EvalMerklePath(merklePath, &mt.ByteHasher{Val: getMatchingTxForBill(b, testBlock).Bytes()}, crypto.SHA256)
		require.NotNil(t, rootHash)
		require.EqualValues(t, merkleTree.GetRootHash(), rootHash)
	}
}

func getMatchingTxForBill(bill *bill, block *block.Block) *txsystem.Transaction {
	billId := bill.getId()
	for _, tx := range block.Transactions {
		if bytes.Equal(tx.UnitId, billId) {
			return tx
		}

		// split can create a new bill id that is not equal tx.unit_id
		moneyTx, _ := moneytx.NewMoneyTx(alphabillMoneySystemId, tx)
		switch mtx := moneyTx.(type) {
		case moneytx.Split:
			splitBillId := txutil.SameShardId(mtx.UnitID(), mtx.HashForIdCalculation(crypto.SHA256)).Bytes32()
			if bytes.Equal(splitBillId[:], billId) {
				return tx
			}
		}
	}
	return nil
}

func TestWholeBalanceIsSentUsingBillTransferOrder(t *testing.T) {
	// create wallet with single bill
	w, mockClient := CreateTestWallet(t)
	addBill(t, w, 100)
	receiverPubKey := make([]byte, 33)

	// when whole balance is spent
	err := w.Send(receiverPubKey, 100)
	require.NoError(t, err)

	// then bill transfer order should be sent
	require.Len(t, mockClient.txs, 1)
	btTx := parseBillTransferTx(t, mockClient.txs[0])
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

	ac, err := w.db.Do().GetAccountKey()
	require.NoError(t, err)
	require.Equal(t, testPubKeyHex, hex.EncodeToString(ac.PubKey))
	require.Equal(t, testPrivKeyHex, hex.EncodeToString(ac.PrivKey))
	require.Equal(t, testPubKeyHashSha256Hex, hex.EncodeToString(ac.PubKeyHash.Sha256))
	require.Equal(t, testPubKeyHashSha512Hex, hex.EncodeToString(ac.PubKeyHash.Sha512))
}

func createMerkleTree(blockTxs []*txsystem.Transaction) (*mt.MerkleTree, error) {
	// create merkle tree from testBlock transactions
	txs := make([]mt.Data, len(blockTxs))
	for i, tx := range blockTxs {
		txs[i] = &mt.ByteHasher{Val: tx.Bytes()}
	}
	return mt.New(crypto.SHA256, txs)
}
