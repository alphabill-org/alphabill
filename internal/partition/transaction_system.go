package partition

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
)

// TransactionSystem is a set of rules and logic for defining units and performing transactions with them.
//  * Init method is called when the transaction system is started, and it must return the initial state of the
// 	transaction system.
//  * The following sequence of methods is executed for each block: BeginBlock, Execute (called once for each transaction in
// the block), EndBlock, and
//		* Commit (consensus round was successful) or
//		* Revert (consensus round was unsuccessful).
type TransactionSystem interface {

	// Init signals the start of the blockchain. Is called when the transaction system is started. Returns the initial
	// state and summary value of the transaction system.
	Init() ([]byte, state.SummaryValue)

	// BeginBlock signals the start of a new block and is invoked before any Execute method calls.
	BeginBlock()

	// Execute method executes the transaction. An error must be returned if the transaction execution was not
	// successful.
	Execute(tx *transaction.Transaction) error

	// EndBlock signals the end of the block and is called after all transactions have been delivered to the
	// transaction system.
	EndBlock() ([]byte, state.SummaryValue)

	// Revert signals the unsuccessful consensus round. When called the transaction system must revert all the changes
	// made during the BeginBlock, EndBlock, and Execute method calls.
	Revert()

	// Commit signals the successful consensus round. Called after the block was approved by the root chain. When called
	// the transaction system must commit all the changes made during the BeginBlock, EndBlock, and Execute method
	// calls.
	Commit()

	// TODO return error in case of BeginBlock, EndBlock, or Revert method execution fails?
}
