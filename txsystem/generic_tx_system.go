package txsystem

import (
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	abfc "github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"go.opentelemetry.io/otel/metric"
)

var _ TransactionSystem = (*GenericTxSystem)(nil)

type (
	GenericTxSystem struct {
		pdr                 types.PartitionDescriptionRecord
		hashAlgorithm       crypto.Hash
		state               *state.State
		currentRoundNumber  uint64
		handlers            txtypes.TxExecutors
		trustBase           types.RootTrustBase
		fees                txtypes.FeeCreditModule
		beginBlockFunctions []func(roundNumber uint64) error
		endBlockFunctions   []func(roundNumber uint64) error
		roundCommitted      bool
		log                 *slog.Logger
		pr                  predicates.PredicateRunner
		unitIDValidator     func(types.UnitID) error
		etBuffer            *ETBuffer // executed transactions buffer
		fcrTypeID           uint32
	}

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		Logger() *slog.Logger
		RoundLogger(curRound func() uint64) *slog.Logger
	}
)

func NewGenericTxSystem(shardConf types.PartitionDescriptionRecord, trustBase types.RootTrustBase, modules []txtypes.Module, observe Observability, opts ...Option) (*GenericTxSystem, error) {
	if err := shardConf.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid Partition Description: %w", err)
	}
	if observe == nil {
		return nil, errors.New("observability must not be nil")
	}

	options, err := DefaultOptions(observe)
	if err != nil {
		return nil, fmt.Errorf("invalid default options: %w", err)
	}
	for _, option := range opts {
		if err := option(options); err != nil {
			return nil, fmt.Errorf("invalid option: %w", err)
		}
	}

	txs := &GenericTxSystem{
		pdr:                 shardConf,
		hashAlgorithm:       options.hashAlgorithm,
		state:               options.state,
		trustBase:           trustBase,
		unitIDValidator:     shardConf.UnitIDValidator(shardConf.ShardID),
		beginBlockFunctions: options.beginBlockFunctions,
		endBlockFunctions:   options.endBlockFunctions,
		handlers:            make(txtypes.TxExecutors),
		pr:                  options.predicateRunner,
		fees:                options.feeCredit,
		etBuffer:            NewETBuffer(WithExecutedTxs(options.executedTransactions)),
	}
	txs.log = observe.RoundLogger(txs.CurrentRound)
	txs.beginBlockFunctions = append([]func(roundNo uint64) error{txs.pruneState, txs.rInit}, txs.beginBlockFunctions...)

	for _, module := range modules {
		if err := txs.handlers.Add(module.TxHandlers()); err != nil {
			return nil, fmt.Errorf("registering transaction handler: %w", err)
		}
	}
	// if fees are collected, then register fee tx handlers
	if options.feeCredit != nil {
		if err := txs.handlers.Add(options.feeCredit.TxHandlers()); err != nil {
			return nil, fmt.Errorf("registering fee credit transaction handler: %w", err)
		}

	}
	if err := txs.initMetrics(observe.Meter("txsystem"), shardConf.ShardID); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}

	return txs, nil
}

func (m *GenericTxSystem) IsPermissionedMode() bool {
	return m.fees.IsPermissionedMode()
}

func (m *GenericTxSystem) IsFeelessMode() bool {
	return m.fees.IsFeelessMode()
}

func (m *GenericTxSystem) StateSize() (uint64, error) {
	committed, err := m.state.IsCommitted()
	if err != nil {
		return 0, fmt.Errorf("unable to check state committed status: %w", err)
	}
	if !committed {
		return 0, ErrStateContainsUncommittedChanges
	}
	return m.state.Size()
}

func (m *GenericTxSystem) StateSummary() (*StateSummary, error) {
	committed, err := m.state.IsCommitted()
	if err != nil {
		return nil, fmt.Errorf("unable to check state committed status: %w", err)
	}
	if !committed {
		return nil, ErrStateContainsUncommittedChanges
	}
	return m.getStateSummary()
}

func (m *GenericTxSystem) getStateSummary() (*StateSummary, error) {
	sv, hash, err := m.state.CalculateRoot()
	if err != nil {
		return nil, err
	}
	etHash, err := m.etBuffer.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to caluclate executed transactions hash: %w", err)
	}
	return NewStateSummary(hash, util.Uint64ToBytes(sv), etHash), nil
}

func (m *GenericTxSystem) BeginBlock(roundNo uint64) error {
	m.currentRoundNumber = roundNo
	m.roundCommitted = false
	for _, function := range m.beginBlockFunctions {
		if err := function(roundNo); err != nil {
			return fmt.Errorf("begin block function call failed: %w", err)
		}
	}
	return nil
}

// rInit generic round initialization procedure common for all transaction systems:
//  1. Prune the state change history for all units that were targeted by transactions in the previous round (done in state pruner)
//  2. Delete all unlocked fee credit records with zero remaining balance and expired lifetime
//  3. Delete all expired units
func (m *GenericTxSystem) rInit(roundNumber uint64) error {
	var expiredFCRs []types.UnitID
	var expiredUnits []types.UnitID
	err := m.state.Traverse(state.NewInorderTraverser(func(unitID types.UnitID, unit state.Unit) error {
		unitV1, err := state.ToUnitV1(unit)
		if err != nil {
			return fmt.Errorf("failed to extract unit v1: %w", err)
		}
		unitType, err := m.pdr.ExtractUnitType(unitID)
		if err != nil {
			return fmt.Errorf("failed to extract unit type: %w", err)
		}
		if unitType == m.fees.FeeCreditRecordUnitType() {
			fcr, ok := unit.Data().(*fc.FeeCreditRecord)
			if !ok {
				// sanity check, should never happen
				return fmt.Errorf("unit data type is not a fee credit record")
			}
			if fcr.IsExpired(roundNumber) {
				expiredFCRs = append(expiredFCRs, unitID)
			}
		} else if unitV1.IsExpired(roundNumber) {
			expiredUnits = append(expiredUnits, unitID)
		}
		// TODO move state_pruner here?
		return nil
	}))
	if err != nil {
		return fmt.Errorf("failed to traverse the state tree: %w", err)
	}
	if err := m.deleteUnits(expiredFCRs); err != nil {
		return fmt.Errorf("failed to delete fcr units: %w", err)
	}
	if err := m.deleteUnits(expiredUnits); err != nil {
		return fmt.Errorf("failed to delete ordinary units: %w", err)
	}
	return nil
}

// deleteUnits deletes provided units, the unitIDs must be sorted lexicographically
func (m *GenericTxSystem) deleteUnits(unitIDs []types.UnitID) error {
	if len(unitIDs) == 0 {
		return nil
	}
	// delete all units in a single batch to minimise concurrency overhead
	var actions []state.Action
	for _, unitID := range unitIDs {
		actions = append(actions, state.DeleteUnit(unitID))
	}
	if err := m.state.Apply(actions...); err != nil {
		return fmt.Errorf("failed to delete units: %w", err)
	}
	return nil
}

func (m *GenericTxSystem) pruneState(roundNo uint64) error {
	return m.state.Prune()
}

// snFees charge storage and network fees.
func (m *GenericTxSystem) snFees(_ *types.TransactionOrder, execCxt txtypes.ExecutionContext) error {
	return execCxt.SpendGas(abfc.GeneralTxCostGasUnits)
}

func (m *GenericTxSystem) Execute(tx *types.TransactionOrder) (tr *types.TransactionRecord, err error) {
	// discard tx if it is a duplicate
	txHash, err := tx.Hash(m.hashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to hash transaction: %w", err)
	}
	// encode tx hash to hex string as the transaction buffer is included
	// in the state file and encoded as CBOR which requires string values to be UTF-8 encoded
	txID := hex.EncodeToString(txHash)
	_, f := m.etBuffer.Get(txID)
	if f {
		return nil, errors.New("transaction already executed")
	}
	defer func() {
		if tr != nil {
			m.etBuffer.Add(txID, tx.Timeout())
		}
	}()

	// First, check transaction credible and that there are enough fee credits on the FCR?
	// buy gas according to the maximum tx fee allowed by client -
	// if fee proof check fails, function will exit tx and tx will not be added to block
	exeCtx := txtypes.NewExecutionContext(m, m.fees, m.trustBase, tx.MaxFee())
	// 2. If P.α != S.α ∨ fSH(P.ι) != S.σ ∨ S .n ≥ P.T 0 then return ⊥
	// 3. If not P.MC .ι f = ⊥ = P.s f then return ⊥
	if err := m.validateGenericTransaction(tx); err != nil {
		return nil, fmt.Errorf("invalid transaction: %w", err)
	}
	// only handle fees if there is a fee module
	if err := m.snFees(tx, exeCtx); err != nil {
		return nil, fmt.Errorf("error transaction snFees: %w", err)
	}
	// all transactions that get this far will go into bock even if they fail and cost is credited from user FCR
	m.log.Debug(fmt.Sprintf("execute %d", tx.Type), logger.UnitID(tx.GetUnitID()), logger.Data(tx))
	// execute fee credit transactions
	if m.fees.IsFeeCreditTx(tx) {
		tr, err = m.executeFc(tx, exeCtx)
		if err != nil {
			return nil, fmt.Errorf("execute fc error: %w", err)
		}
		return tr, nil
	}
	// execute rest ordinary transactions
	if err := m.fees.IsCredible(exeCtx, tx); err != nil {
		// not credible, means that no fees can be charged, so just exit with error tx will not be added to block
		return nil, fmt.Errorf("error transaction not credible: %w", err)
	}
	tr, err = m.doExecute(tx, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("execute error: %w", err)
	}
	return tr, nil
}

func (m *GenericTxSystem) doExecute(tx *types.TransactionOrder, exeCtx *txtypes.TxExecutionContext) (txr *types.TransactionRecord, retErr error) {
	var txExecErr error
	txBytes, err := tx.MarshalCBOR()
	if err != nil {
		return nil, fmt.Errorf("marshalling transaction: %w", err)
	}
	txr = &types.TransactionRecord{
		Version:          1,
		TransactionOrder: txBytes,
		ServerMetadata:   &types.ServerMetadata{SuccessIndicator: types.TxStatusSuccessful},
	}
	savepointID, err := m.state.Savepoint()
	if err != nil {
		return nil, fmt.Errorf("savepoint error: %w", err)
	}
	defer func() {
		// set the correct success indicator
		if txExecErr != nil {
			m.log.Warn("transaction execute failed", logger.Error(txExecErr), logger.UnitID(tx.GetUnitID()))
			// will set correct error status and clean up target units
			txr.ServerMetadata.SetError(txExecErr)
			// transaction execution failed. revert every change made by the transaction order
			m.state.RollbackToSavepoint(savepointID)
		}
		// Handle fees! NB! The "transfer to fee credit" and "reclaim fee credit" transactions in the money partition
		// and the "lock fee credit", "unlock fee credit", "add fee credit" and "close fee credit" transactions in all
		// application partitions are special cases: fees are handled intrinsically in those transactions.
		// charge user according to gas used
		txr.ServerMetadata.ActualFee = exeCtx.CalculateCost()
		if txr.ServerMetadata.ActualFee > 0 {
			// credit the cost from
			feeCreditRecordID := tx.FeeCreditRecordID()
			if err := m.state.Apply(unit.DecrCredit(feeCreditRecordID, txr.ServerMetadata.ActualFee)); err != nil {
				// Tx must not be added to block - FCR could not be credited.
				// Otherwise, Tx would be for free, and there are no funds taken to pay validators
				m.state.RollbackToSavepoint(savepointID)
				// clear metadata
				txr = nil
				retErr = fmt.Errorf("handling transaction fee: %w", err)
				return
			}
			// add fee credit record unit log
			txr.ServerMetadata.TargetUnits = append(txr.ServerMetadata.TargetUnits, feeCreditRecordID)
		}

		// update unit log's
		for _, targetID := range txr.ServerMetadata.TargetUnits {
			txrHash, err := txr.Hash(m.hashAlgorithm)
			if err != nil {
				m.state.RollbackToSavepoint(savepointID)
				txr = nil
				retErr = fmt.Errorf("hashing transaction record: %w", err)
				return
			}
			// add log for each target unit
			if err := m.state.AddUnitLog(targetID, txrHash); err != nil {
				// If the unit log update fails, the Tx must not be added to block - there is no way to provide full ledger.
				// The problem is that a lot of work has been done. If this can be triggered externally, it will become
				// an attack vector.
				m.state.RollbackToSavepoint(savepointID)
				// clear metadata
				txr = nil
				retErr = fmt.Errorf("adding unit log: %w", err)
				return
			}
		}
		// transaction execution succeeded
		m.state.ReleaseToSavepoint(savepointID)
	}()

	// unmarshal tx
	attr, authProof, targetUnits, err := m.handlers.UnmarshalTx(tx, exeCtx)
	if err != nil {
		txExecErr = fmt.Errorf("failed to unmarshal transaction: %w", err)
		return txr, nil
	}
	// check conditional state unlock and release it, either roll back or execute the pending tx
	for _, targetUnit := range targetUnits {
		sm, err := m.handleUnlockUnitState(tx, targetUnit, exeCtx)
		txr.ServerMetadata = appendServerMetadata(txr.ServerMetadata, sm)
		if err != nil {
			txExecErr = fmt.Errorf("unit state unlock error: %w", err)
			return txr, nil
		}
	}
	// perform transaction-system-specific validation and owner predicate check
	if err := m.handlers.Validate(tx, attr, authProof, exeCtx); err != nil {
		txExecErr = fmt.Errorf("transaction validation error (type=%d): %w", tx.Type, err)
		return txr, nil
	}
	// either state lock or execute tx
	if tx.HasStateLock() {
		// handle conditional lock of units
		sm, err := m.executeLockUnitState(tx, txBytes, targetUnits, exeCtx)
		txr.ServerMetadata = appendServerMetadata(txr.ServerMetadata, sm)
		if err != nil {
			txExecErr = fmt.Errorf("unit state lock error: %w", err)
		}
		return txr, nil
	}
	// proceed with the transaction execution, if not state lock
	sm, err := m.handlers.ExecuteWithAttr(tx, attr, authProof, exeCtx)
	txr.ServerMetadata = appendServerMetadata(txr.ServerMetadata, sm)
	if err != nil {
		txExecErr = fmt.Errorf("execute error: %w", err)
	}
	return txr, nil
}

func (m *GenericTxSystem) executeFc(tx *types.TransactionOrder, exeCtx *txtypes.TxExecutionContext) (*types.TransactionRecord, error) {
	// 4. If P.C != ⊥ then return ⊥ – discard P if it is conditional
	if tx.StateLock != nil {
		return nil, errors.New("error fc transaction contains state lock")
	}
	// 5. S.N[P.ι].L != ⊥ and S.N[P.ι].L.Ppend.τ != nop then return ⊥,
	// discard P if the target unit is in a lock state with a non-trivial pending transaction.
	txOnHold, err := m.parseTxOnHold(tx.UnitID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse txOnHold: %w", err)
	}
	if txOnHold != nil && txOnHold.Type != nop.TransactionTypeNOP {
		return nil, errors.New("target unit is locked with a non-trivial (non-NOP) transaction")
	}

	// 6. If S.N[P.ι] != ⊥ and not EvalPred(S.N[P.ι].φ; S, P, P.s; &MS) - will be done during VerifyPsi
	// 7. If not VerifyPsi(S, P; &MS)
	// perform transaction-system-specific validation and owner predicate check
	attr, authProof, _, err := m.handlers.UnmarshalTx(tx, exeCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
	}
	if err := m.handlers.Validate(tx, attr, authProof, exeCtx); err != nil {
		return nil, fmt.Errorf("failed to validate transaction: %w", err)
	}
	// 8. b ← SNFees(S, P; &MS) - is at the moment done for all tx before this call

	// initialize metadata
	txBytes, err := tx.MarshalCBOR()
	if err != nil {
		return nil, fmt.Errorf("marshalling transaction: %w", err)
	}
	tr := &types.TransactionRecord{
		Version:          1,
		TransactionOrder: txBytes,
		ServerMetadata:   &types.ServerMetadata{SuccessIndicator: types.TxStatusSuccessful},
	}

	// 9. TakeSnapshot
	savepointID, err := m.state.Savepoint()
	if err != nil {
		return nil, fmt.Errorf("savepoint error: %w", err)
	}

	// 10. b ← Unlock(S, P; &MS) – try to resolve the locked state (Sec. 4.6.6); b is ignored.
	sm, err := m.handleTxOnHold(tx, txOnHold, exeCtx)
	if err != nil {
		// 11. If S.N[P.ι].L != ⊥ - if P could not resolve the locked state
		// roll the state tree back to the snapshot of line 9 and discard P.
		m.state.RollbackToSavepoint(savepointID)
		return nil, fmt.Errorf("unit state unlock error: %w", err)
	}
	tr.ServerMetadata = appendServerMetadata(tr.ServerMetadata, sm)

	// 12. MS.fa ← min{MS.fa, P.MC. fm} – adjust actual fees, if they exceed the allowed fees
	// done in appendServerMetadata?

	// 13. Execute(S, P; &MS ) – execute P (Sec. 4.6.8) - proceed with the transaction execution
	sm, err = m.handlers.ExecuteWithAttr(tx, attr, authProof, exeCtx)
	if err != nil {
		m.state.RollbackToSavepoint(savepointID)
		return nil, fmt.Errorf("execute error: %w", err)
	}
	tr.ServerMetadata = appendServerMetadata(tr.ServerMetadata, sm)

	trHash, err := tr.Hash(m.hashAlgorithm)
	if err != nil {
		m.state.RollbackToSavepoint(savepointID)
		return nil, fmt.Errorf("hashing transaction record: %w", err)
	}
	// update unit log's
	for _, targetID := range sm.TargetUnits {
		// add log for each target unit
		if err = m.state.AddUnitLog(targetID, trHash); err != nil {
			// If the unit log update fails, the Tx must not be added to block - there is no way to provide full ledger.
			// The problem is that a lot of work has been done. If this can be triggered externally, it will become
			// an attack vector.
			m.state.RollbackToSavepoint(savepointID)
			return nil, fmt.Errorf("adding unit log: %w", err)
		}
	}
	// transaction execution succeeded
	m.state.ReleaseToSavepoint(savepointID)
	return tr, nil
}

/*
validateGenericTransaction verifies that the transaction is sent to the correct network/partition/shard and is not expired.

See Yellowpaper chapter 4.6 "Transaction Processing".
*/
func (m *GenericTxSystem) validateGenericTransaction(tx *types.TransactionOrder) error {
	// T.α = S.α – transaction is sent to this network
	if m.pdr.NetworkID != tx.NetworkID {
		return fmt.Errorf("invalid network id: %d (expected %d)", tx.NetworkID, m.pdr.NetworkID)
	}

	// T.β = S.β – transaction is sent to this partition
	if m.pdr.PartitionID != tx.PartitionID {
		return ErrInvalidPartitionID
	}

	// fSH(T.ι) = S.σ – target unit is in this shard
	if err := m.unitIDValidator(tx.UnitID); err != nil {
		return err
	}

	// T0 ≥ S.n – transaction has not expired
	if m.currentRoundNumber > tx.Timeout() {
		return ErrTransactionExpired
	}
	return nil
}

func (m *GenericTxSystem) State() StateReader {
	return m.state.Clone()
}

func (m *GenericTxSystem) EndBlock() (*StateSummary, error) {
	// remove expired transaction order data from ET (duplicate check buffer)
	m.etBuffer.ClearExpired(m.currentRoundNumber)

	// execute the transaction system specific completion steps
	for _, function := range m.endBlockFunctions {
		if err := function(m.currentRoundNumber); err != nil {
			return nil, fmt.Errorf("end block function call failed: %w", err)
		}
	}
	return m.getStateSummary()
}

func (m *GenericTxSystem) Revert() {
	if m.roundCommitted {
		return
	}
	m.state.Revert()
	m.etBuffer.Revert()
}

func (m *GenericTxSystem) Commit(uc *types.UnicityCertificate) error {
	err := m.state.Commit(uc)
	if err == nil {
		m.roundCommitted = true
		m.etBuffer.Commit()
	}
	return err
}

func (m *GenericTxSystem) CommittedUC() *types.UnicityCertificate {
	return m.state.CommittedUC()
}

func (m *GenericTxSystem) SerializeState(writer io.Writer) error {
	return m.state.Serialize(writer, true, m.etBuffer.executedTransactions)
}

func (m *GenericTxSystem) CurrentRound() uint64 {
	return m.currentRoundNumber
}

func (m *GenericTxSystem) TypeID() types.PartitionTypeID {
	return m.pdr.PartitionTypeID
}

func (m *GenericTxSystem) GetUnit(id types.UnitID, committed bool) (state.Unit, error) {
	return m.state.GetUnit(id, committed)
}

func (m *GenericTxSystem) initMetrics(mtr metric.Meter, shardID types.ShardID) error {
	shardAttr := observability.Shard(m.pdr.PartitionID, shardID)
	if _, err := mtr.Int64ObservableUpDownCounter(
		"unit.count",
		metric.WithDescription(`Number of units in the state.`),
		metric.WithUnit("{unit}"),
		metric.WithInt64Callback(func(ctx context.Context, io metric.Int64Observer) error {
			snc := state.NewStateNodeCounter()
			m.state.Traverse(snc)
			io.Observe(int64(snc.NodeCount()), shardAttr)
			return nil
		}),
	); err != nil {
		return fmt.Errorf("creating state unit counter: %w", err)
	}

	return nil
}

func appendServerMetadata(sm, smNew *types.ServerMetadata) *types.ServerMetadata {
	if sm == nil {
		return smNew
	}
	if smNew == nil {
		return sm
	}
	sm.ActualFee += smNew.ActualFee
	m := make(map[string]struct{})
	// put slice values into map
	for _, u := range sm.TargetUnits {
		m[string(u)] = struct{}{}
	}
	// append unique unit id's
	for _, u := range smNew.TargetUnits {
		if _, ok := m[string(u)]; !ok {
			m[string(u)] = struct{}{}
			sm.TargetUnits = append(sm.TargetUnits, u)
		}
	}
	sm.ProcessingDetails = smNew.ProcessingDetails
	return sm
}
