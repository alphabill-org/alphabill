package txsystem

import (
	"errors"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
)

var ErrStateContainsUncommittedChanges = errors.New("state contains uncommitted changes")

type (
	// TransactionSystem is a set of rules and logic for defining units and performing transactions with them.
	// The following sequence of methods is executed for each block: BeginBlock,
	// Execute (called once for each transaction in the block), EndBlock, and Commit (consensus round was successful) or
	// Revert (consensus round was unsuccessful).
	TransactionSystem interface {

		// StateSummary returns the summary of the current state of the transaction system or an ErrStateContainsUncommittedChanges if
		// current state contains uncommitted changes.
		StateSummary() (StateSummary, error)

		// State returns the current state of the transaction system.
		State() *state.State

		// BeginBlock signals the start of a new block and is invoked before any Execute method calls.
		BeginBlock(uint64) error

		// Execute method executes the transaction order. An error must be returned if the transaction order execution
		// was not successful.
		Execute(order *types.TransactionOrder) (*types.ServerMetadata, error)

		// EndBlock signals the end of the block and is called after all transactions have been delivered to the
		// transaction system.
		EndBlock() (StateSummary, error)

		// Revert signals the unsuccessful consensus round. When called the transaction system must revert all the changes
		// made during the BeginBlock, EndBlock, and Execute method calls.
		Revert()

		// Commit signals the successful consensus round. Called after the block was approved by the root chain. When called
		// the transaction system must commit all the changes made during the BeginBlock,
		// EndBlock, and Execute method calls.
		Commit() error

		// StateStorage returns clone of transaction system state
		StateStorage() UnitAndProof
	}

	// StateSummary represents the root hash and summary value of the transaction system.
	StateSummary interface {
		// Root returns the root hash of the TransactionSystem.
		Root() []byte
		// Summary returns the summary value of the state.
		Summary() []byte
	}

	// UnitAndProof read access to state to access unit and unit proofs
	UnitAndProof interface {
		// GetUnit - access tx system unit state
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
		// CreateUnitStateProof - create unit proofs
		CreateUnitStateProof(id types.UnitID, logIndex int, uc *types.UnicityCertificate) (*types.UnitStateProof, error)
	}

	// stateSummary is the default implementation of StateSummary interface.
	stateSummary struct {
		rootHash []byte
		summary  []byte
	}
)

func NewStateSummary(rootHash []byte, summary []byte) stateSummary {
	return stateSummary{
		rootHash: rootHash,
		summary:  summary,
	}
}

func (s stateSummary) Root() []byte {
	return s.rootHash
}

func (s stateSummary) Summary() []byte {
	return s.summary
}
