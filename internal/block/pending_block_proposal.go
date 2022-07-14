package block

import "github.com/alphabill-org/alphabill/internal/txsystem"

type PendingBlockProposal struct {
	RoundNumber  uint64
	PrevHash     []byte
	StateHash    []byte
	Transactions []txsystem.GenericTransaction
}
