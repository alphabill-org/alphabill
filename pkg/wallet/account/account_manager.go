package account

import (
	"errors"
	"path"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
)

type (
	// Manager manages accounts
	Manager interface {
		GetAll() []Account
		CreateKeys(mnemonic string) error
		AddAccount() (uint64, []byte, error)
		GetMnemonic() (string, error)
		GetAccountKey(uint64) (*AccountKey, error)
		GetAccountKeys() ([]*AccountKey, error)
		GetMaxAccountIndex() (uint64, error)
		GetPublicKey(accountIndex uint64) ([]byte, error)
		GetPublicKeys() ([][]byte, error)
		IsEncrypted() (bool, error)
		Close()
	}

	managerImpl struct {
		db       Db
		accounts *accounts
		dir      string
		password string
	}
)

var (
	ErrInvalidPassword = errors.New("invalid password")
)

func NewManager(dir string, password string, create bool) (Manager, error) {
	return newManager(dir, password, create)
}

func newManager(dir string, password string, create bool) (*managerImpl, error) {
	db, err := getDb(dir, create, password)
	if err != nil {
		return nil, err
	}
	ok, err := db.Do().VerifyPassword()
	if err != nil {
		return nil, err
	}
	if !ok {
		db.Close()
		return nil, ErrInvalidPassword
	}

	accountKeys, err := db.Do().GetAccountKeys()
	if err != nil {
		db.Close()
		return nil, err
	}
	accs := make([]Account, len(accountKeys))
	for idx, val := range accountKeys {
		accs[idx] = Account{
			AccountIndex: uint64(idx),
			AccountKeys:  *val.PubKeyHash,
		}
	}
	return &managerImpl{db: db, accounts: &accounts{accounts: accs}, password: password, dir: dir}, nil
}

func (m *managerImpl) CreateKeys(mnemonic string) error {
	keys, err := NewKeys(mnemonic)
	if err != nil {
		return err
	}
	err = m.saveKeys(keys)
	if err != nil {
		return err
	}

	m.accounts.add(&Account{
		AccountIndex: 0,
		AccountKeys:  *keys.AccountKey.PubKeyHash,
	})
	return nil
}

func (m *managerImpl) GetAccountKey(accountIndex uint64) (*AccountKey, error) {
	return m.db.Do().GetAccountKey(accountIndex)
}

func (m *managerImpl) GetAccountKeys() ([]*AccountKey, error) {
	return m.db.Do().GetAccountKeys()
}

// GetPublicKey returns public key of the wallet (compressed secp256k1 key 33 bytes)
func (m *managerImpl) GetPublicKey(accountIndex uint64) ([]byte, error) {
	key, err := m.GetAccountKey(accountIndex)
	if err != nil {
		return nil, err
	}
	return key.PubKey, nil
}

// GetPublicKeys returns public keys of the wallet, indexed by account indexes
func (m *managerImpl) GetPublicKeys() ([][]byte, error) {
	accKeys, err := m.GetAccountKeys()
	if err != nil {
		return nil, err
	}
	pubKeys := make([][]byte, len(accKeys))
	for accIdx, accKey := range accKeys {
		pubKeys[accIdx] = accKey.PubKey
	}
	return pubKeys, nil
}

func (m *managerImpl) GetMaxAccountIndex() (uint64, error) {
	return m.db.Do().GetMaxAccountIndex()
}

// GetMnemonic returns mnemonic seed of the wallet
func (m *managerImpl) GetMnemonic() (string, error) {
	return m.db.Do().GetMnemonic()
}

// AddAccount adds the next account in account key series to the wallet.
// New accounts are indexed only from the time of creation and not backwards in time.
// Returns newManager account's index and public key.
func (m *managerImpl) AddAccount() (uint64, []byte, error) {
	masterKeyString, err := m.db.Do().GetMasterKey()
	if err != nil {
		return 0, nil, err
	}
	masterKey, err := hdkeychain.NewKeyFromString(masterKeyString)
	if err != nil {
		return 0, nil, err
	}

	accountIndex, err := m.db.Do().GetMaxAccountIndex()
	if err != nil {
		return 0, nil, err
	}
	accountIndex += 1

	derivationPath := NewDerivationPath(accountIndex)
	accountKey, err := NewAccountKey(masterKey, derivationPath)
	if err != nil {
		return 0, nil, err
	}
	err = m.db.WithTransaction(func(tx TxContext) error {
		err := tx.AddAccount(accountIndex, accountKey)
		if err != nil {
			return err
		}
		err = tx.SetMaxAccountIndex(accountIndex)
		if err != nil {
			return err
		}
		m.accounts.add(&Account{AccountIndex: accountIndex, AccountKeys: *accountKey.PubKeyHash})
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return accountIndex, accountKey.PubKey, nil
}

// IsEncrypted returns true if wallet exists and is encrypted and or false if wallet exists and is not encrypted,
// returns error if wallet does not exist. Closes DB upon completion.
func (m *managerImpl) IsEncrypted() (bool, error) {
	defer m.Close()
	return m.db.Do().IsEncrypted()
}

func (m *managerImpl) GetAll() []Account {
	return m.accounts.getAll()
}

func (m *managerImpl) Close() {
	if m.db != nil {
		m.db.Close()
	}
}

func getDb(dir string, create bool, pw string) (Db, error) {
	if create {
		return createNewDb(dir, pw)
	}
	dbFilePath := path.Join(dir, AccountFileName)
	return openDb(dbFilePath, pw, false)
}

func (m *managerImpl) saveKeys(keys *Keys) error {
	return m.db.WithTransaction(func(tx TxContext) error {
		err := tx.SetEncrypted(m.password != "")
		if err != nil {
			return err
		}
		err = tx.SetMnemonic(keys.Mnemonic)
		if err != nil {
			return err
		}
		err = tx.SetMasterKey(keys.MasterKey.String())
		if err != nil {
			return err
		}
		err = tx.AddAccount(0, keys.AccountKey)
		if err != nil {
			return err
		}
		return tx.SetMaxAccountIndex(0)
	})
}
