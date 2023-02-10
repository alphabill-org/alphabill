package money

import (
	"context"
	"crypto"
	"encoding/hex"
	"fmt"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
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

	balance, err := w.GetBalance(GetBalanceCmd{})
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
	balance, err := w.GetBalance(GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 0, balance)
}

func TestWallet_GetBalances(t *testing.T) {
	w, _ := CreateTestWalletFromSeed(t)
	_ = w.db.Do().SetBill(0, &Bill{Id: uint256.NewInt(0), Value: 1})
	_ = w.db.Do().SetBill(0, &Bill{Id: uint256.NewInt(1), Value: 1})

	_, _, _ = w.AddAccount()
	_ = w.db.Do().SetBill(1, &Bill{Id: uint256.NewInt(2), Value: 2})
	_ = w.db.Do().SetBill(1, &Bill{Id: uint256.NewInt(3), Value: 2})

	balances, err := w.GetBalances(GetBalanceCmd{})
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
			SystemIdentifier:  w.SystemID(),
			PreviousBlockHash: hash.Sum256([]byte{}),
			Transactions: []*txsystem.Transaction{
				// random dust transfer can be processed
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x00}),
					TransactionAttributes: moneytesttx.CreateRandomDustTransferTx(),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentEmpty(),
				},
				// receive transfer of 100 bills
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x01}),
					TransactionAttributes: moneytesttx.CreateBillTransferTx(k.PubKeyHash.Sha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive split of 100 bills
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x02}),
					TransactionAttributes: moneytesttx.CreateBillSplitTx(k.PubKeyHash.Sha256, 100, 100),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive swap of 100 bills
				{
					SystemId:              w.SystemID(),
					UnitId:                hash.Sum256([]byte{0x03}),
					TransactionAttributes: moneytesttx.CreateRandomSwapTransferTx(k.PubKeyHash.Sha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
		},
	}

	// verify block number 0 before processing
	blockNumber, err := w.db.Do().GetBlockNumber()
	require.EqualValues(t, 0, blockNumber)
	require.NoError(t, err)

	// verify balance 0 before processing
	balance, err := w.db.Do().GetBalance(GetBalanceCmd{})
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
	balance, err = w.db.Do().GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 300, balance)
	require.NoError(t, err)
}

func TestBlockProcessing_OldBlockDoesNotOverwriteNewerBills(t *testing.T) {
	w, _ := CreateTestWallet(t)

	k, err := w.db.Do().GetAccountKey(0)
	require.NoError(t, err)

	// create block with each type of tx
	b := &block.Block{
		SystemIdentifier:  w.SystemID(),
		PreviousBlockHash: hash.Sum256([]byte{}),
		Transactions: []*txsystem.Transaction{
			// random dust transfer can be processed
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x00}),
				TransactionAttributes: moneytesttx.CreateRandomDustTransferTx(),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentEmpty(),
			},
			// receive transfer of 100 bills
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x01}),
				TransactionAttributes: moneytesttx.CreateBillTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive split of 100 bills
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x02}),
				TransactionAttributes: moneytesttx.CreateBillSplitTx(k.PubKeyHash.Sha256, 100, 100),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive swap of 100 bills
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x03}),
				TransactionAttributes: moneytesttx.CreateRandomSwapTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
		},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 10}},
	}

	// for each tx in a block add unit to db with newer block number
	actualBlockNumber := b.UnicityCertificate.InputRecord.RoundNumber + 1
	for _, tx := range b.Transactions {
		bill := &Bill{
			Id:         uint256.NewInt(0).SetBytes(tx.UnitId),
			Value:      1,
			BlockProof: &BlockProof{BlockNumber: actualBlockNumber},
		}
		_ = w.db.Do().SetBill(0, bill)
	}

	// process block
	err = w.ProcessBlock(b)
	require.NoError(t, err)

	// verify none of the txs are processed
	bills, _ := w.db.Do().GetBills(0)
	for _, actualBill := range bills {
		require.Equal(t, actualBlockNumber, actualBill.BlockProof.BlockNumber)
	}
}

func TestBlockProcessing_InvalidSystemID(t *testing.T) {
	w, _ := CreateTestWallet(t)

	b := &block.Block{
		SystemIdentifier:   []byte{0, 0, 0, 1},
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
	}

	err := w.ProcessBlock(b)
	require.ErrorContains(t, err, "invalid system identifier")
}

func TestBlockProcessing_VerifyBlockProofs(t *testing.T) {
	w, _ := CreateTestWallet(t)
	k, _ := w.db.Do().GetAccountKey(0)

	testBlock := &block.Block{
		SystemIdentifier:  w.SystemID(),
		PreviousBlockHash: hash.Sum256([]byte{}),
		Transactions: []*txsystem.Transaction{
			// receive transfer of 100 bills
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x00}),
				TransactionAttributes: moneytesttx.CreateBillTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive dc transfer of 100 bills
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x01}),
				TransactionAttributes: moneytesttx.CreateDustTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive split of 100 bills
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x02}),
				TransactionAttributes: moneytesttx.CreateBillSplitTx(k.PubKeyHash.Sha256, 100, 100),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
			// receive swap of 100 bills
			{
				SystemId:              w.SystemID(),
				UnitId:                hash.Sum256([]byte{0x03}),
				TransactionAttributes: moneytesttx.CreateRandomSwapTransferTx(k.PubKeyHash.Sha256),
				Timeout:               1000,
				OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
			},
		},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
	}
	txc := NewTxConverter(w.SystemID())

	certifiedBlock, verifiers := testblock.CertifyBlock(t, testBlock, txc)
	err := w.ProcessBlock(certifiedBlock)
	require.NoError(t, err)

	bills, _ := w.db.Do().GetBills(0)
	require.Len(t, bills, 4)
	for _, b := range bills {
		err = b.BlockProof.Verify(b.GetID(), verifiers, crypto.SHA256, txc)
		require.NoError(t, err)
		require.Equal(t, block.ProofType_PRIM, b.BlockProof.Proof.ProofType)
	}
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
	c := WalletConfig{DbPath: t.TempDir()}
	invalidSeed := "this pond palace oblige remind glory lens popular iron decide coral"
	_, err := CreateNewWallet(invalidSeed, c)
	require.ErrorContains(t, err, "invalid mnemonic")

	// verify database is not created
	require.False(t, util.FileExists(path.Join(c.DbPath, WalletFileName)))
}

func TestWalletGetBills_Ok(t *testing.T) {
	w, _ := CreateTestWallet(t)
	addBill(t, w, 100)
	addBill(t, w, 200)
	bills, err := w.GetBills(0)
	require.NoError(t, err)
	require.Len(t, bills, 2)
	require.Equal(t, "0000000000000000000000000000000000000000000000000000000000000064", fmt.Sprintf("%X", bills[0].GetID()))
	require.Equal(t, "00000000000000000000000000000000000000000000000000000000000000C8", fmt.Sprintf("%X", bills[1].GetID()))
}

func TestWalletGetAllBills_Ok(t *testing.T) {
	w, _ := CreateTestWallet(t)
	_, _, _ = w.AddAccount()
	_ = w.db.Do().SetBill(0, &Bill{
		Id:     uint256.NewInt(100),
		Value:  100,
		TxHash: hash.Sum256([]byte{byte(100)}),
	})
	_ = w.db.Do().SetBill(1, &Bill{
		Id:     uint256.NewInt(200),
		Value:  200,
		TxHash: hash.Sum256([]byte{byte(200)}),
	})

	accBills, err := w.GetAllBills()
	require.NoError(t, err)
	require.Len(t, accBills, 2)

	acc0Bills := accBills[0]
	require.Len(t, acc0Bills, 1)
	require.EqualValues(t, acc0Bills[0].Value, 100)

	acc1Bills := accBills[1]
	require.Len(t, acc1Bills, 1)
	require.EqualValues(t, acc1Bills[0].Value, 200)
}

func TestWalletGetBill(t *testing.T) {
	// setup wallet with a bill
	w, _ := CreateTestWallet(t)
	b1 := addBill(t, w, 100)

	// verify getBill returns existing bill
	b, err := w.GetBill(0, b1.GetID())
	require.NoError(t, err)
	require.NotNil(t, b)

	// verify non-existent bill returns BillNotFound error
	b, err = w.GetBill(0, []byte{0})
	require.ErrorIs(t, err, errBillNotFound)
	require.Nil(t, b)
}

func TestWalletAddBill(t *testing.T) {
	// setup wallet
	w, _ := CreateTestWalletFromSeed(t)
	pubkey, _ := w.GetPublicKey(0)

	// verify nil bill
	err := w.AddBill(0, nil)
	require.ErrorContains(t, err, "bill is nil")

	// verify bill id is nil
	err = w.AddBill(0, &Bill{Id: nil})
	require.ErrorContains(t, err, "bill id is nil")

	// verify bill tx is nil
	err = w.AddBill(0, &Bill{Id: uint256.NewInt(0)})
	require.ErrorContains(t, err, "bill tx hash is nil")

	// verify bill block proof is nil
	err = w.AddBill(0, &Bill{Id: uint256.NewInt(0), TxHash: []byte{}})
	require.ErrorContains(t, err, "bill block proof is nil")

	err = w.AddBill(0, &Bill{Id: uint256.NewInt(0), TxHash: []byte{}, BlockProof: &BlockProof{}})
	require.ErrorContains(t, err, "bill block proof tx is nil")

	// verify invalid bearer predicate
	invalidPubkey := []byte{0}
	err = w.AddBill(0, &Bill{
		Id:         uint256.NewInt(0),
		TxHash:     []byte{},
		BlockProof: &BlockProof{Tx: createTransferTxForPubKey(w.SystemID(), invalidPubkey)},
	})
	require.ErrorContains(t, err, "invalid bearer predicate")

	// verify valid bill no error
	err = w.AddBill(0, &Bill{
		Id:         uint256.NewInt(0),
		TxHash:     []byte{},
		BlockProof: &BlockProof{Tx: createTransferTxForPubKey(w.SystemID(), pubkey)},
	})
	require.NoError(t, err)
}

func verifyTestWallet(t *testing.T, w *Wallet) {
	mnemonic, err := w.db.Do().GetMnemonic()
	require.NoError(t, err)
	require.Equal(t, testMnemonic, mnemonic)

	mk, err := w.db.Do().GetMasterKey()
	require.NoError(t, err)
	require.Equal(t, testMasterKeyBase58, mk)

	ac, err := w.db.Do().GetAccountKey(0)
	require.NoError(t, err)
	require.Equal(t, testPubKey0Hex, hex.EncodeToString(ac.PubKey))
	require.Equal(t, testPrivKey0Hex, hex.EncodeToString(ac.PrivKey))
	require.Equal(t, testPubKey0HashSha256Hex, hex.EncodeToString(ac.PubKeyHash.Sha256))
	require.Equal(t, testPubKey0HashSha512Hex, hex.EncodeToString(ac.PubKeyHash.Sha512))
}

func createTransferTxForPubKey(systemId, pubkey []byte) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              systemId,
		UnitId:                hash.Sum256([]byte{0x01}),
		TransactionAttributes: moneytesttx.CreateBillTransferTx(hash.Sum256(pubkey)),
		Timeout:               1000,
		OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, pubkey),
	}
}
