package block

import "github.com/alphabill-org/alphabill/internal/txsystem"

type GenericPendingBlockProposal struct {
	RoundNumber    uint64
	ProposerNodeId string
	PrevHash       []byte
	StateHash      []byte
	Transactions   []txsystem.GenericTransaction
}

type PendingBlockProposal struct {
	RoundNumber    uint64
	ProposerNodeId string
	PrevHash       []byte
	StateHash      []byte
	Transactions   []*txsystem.Transaction
}

func (x *PendingBlockProposal) ToGeneric(txConverter TxConverter) (*GenericPendingBlockProposal, error) {
	txs, err := ProtobufTxsToGeneric(x.Transactions, txConverter)
	if err != nil {
		return nil, err
	}
	return &GenericPendingBlockProposal{
		RoundNumber:    x.RoundNumber,
		PrevHash:       x.PrevHash,
		StateHash:      x.StateHash,
		ProposerNodeId: x.ProposerNodeId,
		Transactions:   txs,
	}, nil
}

// ToProtobuf converts GenericPendingBlockProposal to protobuf PendingBlockProposal
func (x *GenericPendingBlockProposal) ToProtobuf() *PendingBlockProposal {
	return &PendingBlockProposal{
		RoundNumber:    x.RoundNumber,
		PrevHash:       x.PrevHash,
		StateHash:      x.StateHash,
		ProposerNodeId: x.ProposerNodeId,
		Transactions:   GenericTxsToProtobuf(x.Transactions),
	}
}
