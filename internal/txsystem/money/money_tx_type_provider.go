package money

import "github.com/alphabill-org/alphabill/internal/txsystem"

var TxTypeProvider = &TypeProvider{}

type TypeProvider struct {
}

func (p *TypeProvider) IsPrimary(_ *txsystem.Transaction) bool {
	return true
}
