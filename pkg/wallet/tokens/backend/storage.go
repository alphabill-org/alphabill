package twb

import wtokens "github.com/alphabill-org/alphabill/pkg/wallet/tokens"

type Storage interface {
	Close() error
	GetBlockNumber() (uint64, error)
	SetBlockNumber(blockNumber uint64) error

	SaveTokenType(data *wtokens.TokenUnitType) error
	GetTokenType(id []byte) (*wtokens.TokenUnitType, error)

	SaveTokenUnit(data *wtokens.TokenUnit) error
}
