package partition

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
)

// TransactionSystem is a set of rules and logic for defining units and performing transactions with them.
// The following sequence of methods is executed for each block: RInit, Execute (called once for each transaction in
// the block), and RCompl. If nodes do not reach a consensus then Revert method will be called.
type TransactionSystem interface {
	// RInit signals the start of a new block and is invoked before any Execute method calls.
	RInit()

	// Execute method executes the transaction. An error must be returned if the transaction execution was not
	// successful.
	Execute(tx *transaction.Transaction) error

	// RCompl signals the end of the block and is called after all transactions have been delivered to the
	// transaction system.
	RCompl() ([]byte, state.SummaryValue)

	// Revert signals the unsuccessful consensus round. When called the transaction system must revert all the changes
	// made during the RInit, RCompl, and Execute method calls.
	Revert()

	// TODO add a "Commit" method that signals the end of a successful consensus round?
	// TODO return error in case of RInit, RCompl, or Revert method execution fails?
	// TODO rename idea: RInit => BeginBlock (needs a spec change)?
	// TODO rename idea: RCompl => EndBlock (needs a spec change)?
}
