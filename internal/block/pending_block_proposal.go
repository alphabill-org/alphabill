package block

import "gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

type PendingBlockProposal struct {
	RoundNumber  uint64
	PrevHash     []byte
	StateHash    []byte
	Transactions []txsystem.GenericTransaction
}
