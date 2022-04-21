package wallet

import (
	"encoding/hex"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"github.com/btcsuite/btcutil/hdkeychain"
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

	mnemonic, err := w.db.GetMnemonic()
	require.NoError(t, err)
	require.True(t, bip39.IsMnemonicValid(mnemonic))

	masterKeyString, err := w.db.GetMasterKey()
	require.NoError(t, err)
	masterKey, err := hdkeychain.NewKeyFromString(masterKeyString)
	require.NoError(t, err)

	ac, err := w.db.GetAccountKey()
	require.NoError(t, err)

	eac, err := newAccountKey(masterKey, testAccountKeyDerivationPath)
	require.NoError(t, err)
	require.NotNil(t, eac)
	require.EqualValues(t, eac, ac)
}

func TestExistingWalletCanBeLoaded(t *testing.T) {
	walletDbPath, err := CopyWalletDBFile()
	require.NoError(t, err)

	w, err := LoadExistingWallet(Config{DbPath: walletDbPath})
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

	// test errInvalidPubKey
	err := w.Send(invalidPubKey, amount)
	require.ErrorIs(t, err, errInvalidPubKey)

	// test errInsufficientBalance
	err = w.Send(validPubKey, amount)
	require.ErrorIs(t, err, errInsufficientBalance)

	// test abclient returns error
	b := bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.SetBill(&b)
	require.NoError(t, err)
	mockClient.txResponse = &transaction.TransactionResponse{Ok: false, Message: "some error"}
	err = w.Send(validPubKey, amount)
	require.Error(t, err, "payment returned error code: some error")
	mockClient.txResponse = nil

	// test errSwapInProgress
	nonce := calculateExpectedDcNonce(t, w)
	setDcMetadata(t, w, nonce, &dcMetadata{DcValueSum: 101, DcTimeout: dcTimeoutBlockCount})
	err = w.Send(validPubKey, amount)
	require.ErrorIs(t, err, errSwapInProgress)
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

func TestBlockProcessing(t *testing.T) {
	w, _ := CreateTestWallet(t)

	k, err := w.db.GetAccountKey()
	require.NoError(t, err)

	blocks := []*alphabill.Block{
		{
			BlockNo:       1,
			PrevBlockHash: hash.Sum256([]byte{}),
			Transactions: []*transaction.Transaction{
				// random dust transfer can be processed
				{
					UnitId:                hash.Sum256([]byte{0x00}),
					TransactionAttributes: createRandomDustTransferTx(),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentEmpty(),
				},
				// receive transfer of 100 bills
				{
					UnitId:                hash.Sum256([]byte{0x01}),
					TransactionAttributes: createBillTransferTx(k.PubKeyHashSha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive split of 100 bills
				{
					UnitId:                hash.Sum256([]byte{0x02}),
					TransactionAttributes: createBillSplitTx(k.PubKeyHashSha256, 100, 100),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
				// receive swap of 100 bills
				{
					UnitId:                hash.Sum256([]byte{0x03}),
					TransactionAttributes: createRandomSwapTransferTx(k.PubKeyHashSha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: &certificates.UnicityCertificate{},
		},
	}

	height, err := w.db.GetBlockHeight()
	require.EqualValues(t, 0, height)
	require.NoError(t, err)
	balance, err := w.db.GetBalance()
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)

	for _, block := range blocks {
		err := w.processBlock(block)
		require.NoError(t, err)
	}

	height, err = w.db.GetBlockHeight()
	require.EqualValues(t, 1, height)
	require.NoError(t, err)
	balance, err = w.db.GetBalance()
	require.EqualValues(t, 300, balance)
	require.NoError(t, err)
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

func verifyTestWallet(t *testing.T, w *Wallet) {
	mnemonic, err := w.db.GetMnemonic()
	require.NoError(t, err)
	require.Equal(t, testMnemonic, mnemonic)

	mk, err := w.db.GetMasterKey()
	require.Equal(t, testMasterKeyBase58, mk)

	ac, err := w.db.GetAccountKey()
	require.NoError(t, err)
	require.Equal(t, testPubKeyHex, hex.EncodeToString(ac.PubKey))
	require.Equal(t, testPrivKeyHex, hex.EncodeToString(ac.PrivKey))
	require.Equal(t, testPubKeyHashSha256Hex, hex.EncodeToString(ac.PubKeyHashSha256))
	require.Equal(t, testPubKeyHashSha512Hex, hex.EncodeToString(ac.PubKeyHashSha512))
}
