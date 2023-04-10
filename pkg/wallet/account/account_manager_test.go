package account

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/stretchr/testify/require"
	"github.com/tyler-smith/go-bip39"
)

const (
	walletPass = "default-wallet-pass"

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

func TestEncryptedWalletCanBeCreated(t *testing.T) {
	am, err := newManager(t.TempDir(), walletPass, true)
	require.NoError(t, err)

	isEncrypted, err := am.db.Do().IsEncrypted()
	require.NoError(t, err)
	require.True(t, isEncrypted)
	am.Close()

	am, err = newManager(am.dir, walletPass, false)
	require.NoError(t, err)

	require.NoError(t, am.CreateKeys(testMnemonic))
	mnemonic, err := am.db.Do().GetMnemonic()
	require.NoError(t, err)
	require.True(t, bip39.IsMnemonicValid(mnemonic))

	masterKeyString, err := am.db.Do().GetMasterKey()
	require.NoError(t, err)
	masterKey, err := hdkeychain.NewKeyFromString(masterKeyString)
	require.NoError(t, err)

	ac, err := am.GetAccountKey(0)
	require.NoError(t, err)

	eac, err := NewAccountKey(masterKey, testAccountKeyDerivationPath)
	require.NoError(t, err)
	require.NotNil(t, eac)
	require.EqualValues(t, eac, ac)
	verifyAccount(t, am)
}

func TestEncryptedWalletCanBeLoaded(t *testing.T) {
	am, err := newManager(t.TempDir(), walletPass, true)
	require.NoError(t, err)
	am.Close()

	am, err = newManager(copyWalletDB(t, am.dir), walletPass, false)
	require.NoError(t, err)
	am.Close()
}

func TestLoadingEncryptedWalletWrongPassphrase(t *testing.T) {
	am, err := newManager(t.TempDir(), walletPass, true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(testMnemonic))
	am.Close()

	am, err = newManager(am.dir, "wrong pw", false)
	require.ErrorIs(t, err, ErrInvalidPassword)
	require.Nil(t, am)
}

func TestLoadingEncryptedWalletWithoutPassphrase(t *testing.T) {
	am, err := newManager(t.TempDir(), walletPass, true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(testMnemonic))
	am.Close()

	am, err = newManager(am.dir, "", false)
	require.ErrorIs(t, err, ErrInvalidPassword)
	require.Nil(t, am)
}

func verifyAccount(t *testing.T, m *managerImpl) {
	mnemonic, err := m.db.Do().GetMnemonic()
	require.NoError(t, err)
	require.Equal(t, testMnemonic, mnemonic)
	require.True(t, bip39.IsMnemonicValid(mnemonic))

	mk, err := m.db.Do().GetMasterKey()
	require.NoError(t, err)
	require.Equal(t, testMasterKeyBase58, mk)

	ac, err := m.db.Do().GetAccountKey(0)
	require.NoError(t, err)
	require.Equal(t, testPubKey0Hex, hex.EncodeToString(ac.PubKey))
	require.Equal(t, testPrivKey0Hex, hex.EncodeToString(ac.PrivKey))
	require.Equal(t, testPubKey0HashSha256Hex, hex.EncodeToString(ac.PubKeyHash.Sha256))
	require.Equal(t, testPubKey0HashSha512Hex, hex.EncodeToString(ac.PubKeyHash.Sha512))
}

func copyWalletDB(t *testing.T, srcDir string) string {
	dstDir := t.TempDir()
	srcFile := filepath.Join(srcDir, AccountFileName)
	dstFile := filepath.Join(dstDir, AccountFileName)
	require.NoError(t, copyFile(srcFile, dstFile))
	return dstDir
}

func copyFile(src string, dst string) error {
	srcBytes, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	err = os.WriteFile(dst, srcBytes, 0700)
	if err != nil {
		return err
	}
	return nil
}
