package txsystem

import (
	"errors"
	"io"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrStateContainsUncommittedChanges = errors.New("state contains uncommitted changes")
	ErrTransactionExpired              = errors.New("transaction timeout must be greater than current block number")
	ErrInvalidSystemIdentifier         = errors.New("error invalid system identifier")
	ErrInvalidNetworkIdentifier        = errors.New("error invalid network identifier")
)

type (
	// TransactionSystem is a set of rules and logic for defining units and performing transactions with them.
	// The following sequence of methods is executed for each block: BeginBlock,
	// Execute (called once for each transaction in the block), EndBlock, and Commit (consensus round was successful) or
	// Revert (consensus round was unsuccessful).
	TransactionSystem interface {
		TransactionExecutor

		// StateSummary returns the summary of the current state of the transaction system or an ErrStateContainsUncommittedChanges if
		// current state contains uncommitted changes.
		StateSummary() (StateSummary, error)

		// BeginBlock signals the start of a new block and is invoked before any Execute method calls.
		BeginBlock(uint64) error

		// EndBlock signals the end of the block and is called after all transactions have been delivered to the
		// transaction system.
		EndBlock() (StateSummary, error)

		// Revert signals the unsuccessful consensus round. When called the transaction system must revert all the changes
		// made during the BeginBlock, EndBlock, and Execute method calls.
		Revert()

		// Commit signals the successful consensus round. Called after the block was approved by the root chain. When called
		// the transaction system must commit all the changes made during the BeginBlock,
		// EndBlock, and Execute method calls.
		Commit(uc *types.UnicityCertificate) error

		// CommittedUC returns the unicity certificate of the latest commit.
		CommittedUC() *types.UnicityCertificate

		// State returns clone of transaction system state
		State() StateReader
	}

	StateReader interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)

		CreateUnitStateProof(id types.UnitID, logIndex int) (*types.UnitStateProof, error)

		CreateIndex(state.KeyExtractor[string]) (state.Index[string], error)

		// Serialize writes the serialized state to the given writer.
		Serialize(writer io.Writer, committed bool) error
	}

	TransactionExecutor interface {
		// Execute method executes the transaction order. An error must be returned if the transaction order execution
		// was not successful.
		Execute(order *types.TransactionOrder) (*types.ServerMetadata, error)
	}

	// StateSummary represents the root hash and summary value of the transaction system.
	StateSummary interface {
		// Root returns the root hash of the TransactionSystem.
		Root() []byte
		// Summary returns the summary value of the state.
		Summary() []byte
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
