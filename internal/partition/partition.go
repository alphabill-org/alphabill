package partition

import (
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/unicitytree"
)

var (
	ErrTxSystemIsNil               = errors.New("transaction system is nil")
	ErrPartitionConfigurationIsNil = errors.New("configuration is nil")
)

type (
	P1Request struct {
		// TODO AB-22 PC1-I Protocol: pending P1 request waiting for UC
	}

	UnicityCertificate struct {
		RootChainBlockNumber uint64
		PreviousHash         []byte // Previous Block hash
		Hash                 []byte // Block hash
		// TODO signatures
	}

	// Block is a set of transactions, grouped together for mostly efficiency reasons. At partition
	// level a block is an ordered set of transactions and proofs (UnicityCertificate and partition certificate).
	Block struct {
		systemIdentifier       []byte
		txSystemBlockNumber    uint64
		previousBlockHash      []byte
		transactions           []*transaction.Transaction
		inputRecord            unicitytree.InputRecord
		unicityTreeCertificate unicitytree.Certificate
		unicityCertificate     UnicityCertificate
	}

	// TransactionSystem is a set of rules and logic for defining units and performing transactions with them.
	TransactionSystem interface {
		RInit()
	}

	// Partition implements an instance of a specific TransactionSystem. Partition is a distributed system, it consists of
	// either a set of shards, or one or more partition nodes.
	Partition struct {
		transactionSystem TransactionSystem
		configuration     *Configuration
		luc               *UnicityCertificate
		proposal          *Block
		pr                *P1Request
		t1                *time.Timer
		l                 uint64
	}
)

// New creates a new instance of Partition component.
func New(txSystem TransactionSystem, configuration *Configuration) (*Partition, error) {
	if txSystem == nil {
		return nil, ErrTxSystemIsNil
	}
	if configuration == nil {
		return nil, ErrPartitionConfigurationIsNil
	}
	t1 := time.NewTimer(configuration.T1Timeout)
	stopTimer(t1)

	p := &Partition{
		transactionSystem: txSystem,
		configuration:     configuration,
		t1:                t1,
	}
	return p, nil
}

func (p *Partition) StartNewRound(uc *UnicityCertificate) {
	p.transactionSystem.RInit()
	p.proposal = nil
	p.pr = nil
	p.l = p.GetLeader(uc)
	p.luc = uc
	p.resetTimer()
	if p.l == p.configuration.NodeIdentifier {
		p.process()
	} else {
		p.sendPC1IRequests()
	}
}

func (p *Partition) GetLeader(uc *UnicityCertificate) uint64 {
	// This part of the code needs to be changed in the future but is good enough for the first Testnet.
	return uc.RootChainBlockNumber % p.configuration.PartitionNodeCount
}

func (p *Partition) process() {
	// TODO AB-118 Validate and Execute Transaction Orders
}

func (p *Partition) sendPC1IRequests() {
	// TODO AB-22 PC1-I Protocol
}

func (p *Partition) resetTimer() {
	// stop timer before resetting it because "Reset" should be invoked only on stopped or expired timers with drained
	// channels.
	stopTimer(p.t1)
	p.t1.Reset(p.configuration.T1Timeout)
}

func stopTimer(t *time.Timer) {
	if !t.Stop() {
		// TODO drain channel. call "<-t.C" from the main event loop.
		//<-t.C
	}
}
