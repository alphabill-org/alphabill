package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
)

type BlockProcessor interface {
	ProcessBlock(b *block.Block) error
	PostProcessBlock(b *block.Block) error
}

type Storage interface {
	GetAccountKey() (*AccountKey, error)
	SetAccountKey(key *AccountKey) error

	GetMasterKey() (string, error)
	SetMasterKey(masterKey string) error

	GetMnemonic() (string, error)
	SetMnemonic(mnemonic string) error

	GetBlockHeight() (uint64, error)
	SetBlockHeight(blockHeight uint64) error

	IsEncrypted() (bool, error)
	SetEncrypted(encrypted bool) error
	VerifyPassword() (bool, error)
}
