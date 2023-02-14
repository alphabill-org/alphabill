package twb

type (
	Storage interface {
		Close() error
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error

		SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator PubKey) error
		SaveTokenType(data *TokenUnitType, proof *Proof) error
		GetTokenType(id TokenTypeID) (*TokenUnitType, error)
		QueryTokenType(kind Kind, creator PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error)

		SaveToken(data *TokenUnit, proof *Proof) error
		GetToken(id TokenID) (*TokenUnit, error)
		QueryTokens(kind Kind, owner Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error)
	}
)
