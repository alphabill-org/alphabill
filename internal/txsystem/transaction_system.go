package txsystem

import (
	"errors"

	"github.com/alphabill-org/alphabill/internal/types"
)

var ErrStateContainsUncommittedChanges = errors.New("state contains uncommitted changes")

type (
	// TransactionSystem is a set of rules and logic for defining units and performing transactions with them.
	// The following sequence of methods is executed for each block: BeginBlock, ValidatorGeneratedTransactions,
	// Execute (called once for each transaction in the block), EndBlock, and Commit (consensus round was successful) or
	// Revert (consensus round was unsuccessful).
	TransactionSystem interface {

		// StateSummary returns the current state of the transaction system or an ErrStateContainsUncommittedChanges if
		// current state contains uncommitted changes.
		StateSummary() (State, error)

		// BeginBlock signals the start of a new block and is invoked before any Execute method calls.
		BeginBlock(uint64) error

		// SystemGeneratedTransactions returns a list of validator generated transactions.
		SystemGeneratedTransactions() ([]*types.TransactionRecord, error)

		// Execute method executes the transaction order. An error must be returned if the transaction order execution
		// was not successful.
		Execute(order *types.TransactionOrder) (*types.ServerMetadata, error)

		// EndBlock signals the end of the block and is called after all transactions have been delivered to the
		// transaction system.
		EndBlock() (State, error)

		// Revert signals the unsuccessful consensus round. When called the transaction system must revert all the changes
		// made during the BeginBlock, ValidatorGeneratedTransactions, EndBlock, and Execute method calls.
		Revert()

		// Commit signals the successful consensus round. Called after the block was approved by the root chain. When called
		// the transaction system must commit all the changes made during the BeginBlock, ValidatorGeneratedTransactions,
		// EndBlock, and Execute method calls.
		Commit() error
	}

	// State represents the root hash and summary value of the transaction system.
	State interface {
		// Root returns the root hash of the TransactionSystem.
		Root() []byte
		// Summary returns the summary value of the state.
		Summary() []byte
	}

	// StateSummary is a default implementation of State interface.
	StateSummary struct {
		rootHash []byte
		summary  []byte
	}
)

func NewStateSummary(rootHash []byte, summary []byte) StateSummary {
	return StateSummary{
		rootHash: rootHash,
		summary:  summary,
	}
}

func (s StateSummary) Root() []byte {
	return s.rootHash
}

func (s StateSummary) Summary() []byte {
	return s.summary
}
