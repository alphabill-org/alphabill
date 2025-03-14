package txsystem

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	abfc "github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
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
	}

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		Logger() *slog.Logger
		RoundLogger(curRound func() uint64) *slog.Logger
	}
)

func NewGenericTxSystem(pdr types.PartitionDescriptionRecord, shardID types.ShardID, trustBase types.RootTrustBase, modules []txtypes.Module, observe Observability, opts ...Option) (*GenericTxSystem, error) {
	if err := pdr.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid Partition Description: %w", err)
	}
	if err := pdr.IsValidShard(shardID); err != nil {
		return nil, fmt.Errorf("invalid shard ID: %w", err)
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
		pdr:                 pdr,
		hashAlgorithm:       options.hashAlgorithm,
		state:               options.state,
		trustBase:           trustBase,
		unitIDValidator:     pdr.UnitIDValidator(shardID),
		beginBlockFunctions: options.beginBlockFunctions,
		endBlockFunctions:   options.endBlockFunctions,
		handlers:            make(txtypes.TxExecutors),
		pr:                  options.predicateRunner,
		fees:                options.feeCredit,
	}
	txs.log = observe.RoundLogger(txs.CurrentRound)
	txs.beginBlockFunctions = append(txs.beginBlockFunctions, txs.pruneState)

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
	if err := txs.initMetrics(observe.Meter("txsystem"), shardID); err != nil {
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

func (m *GenericTxSystem) StateSummary() (StateSummary, error) {
	committed, err := m.state.IsCommitted()
	if err != nil {
		return nil, fmt.Errorf("unable to check state committed status: %w", err)
	}
	if !committed {
		return nil, ErrStateContainsUncommittedChanges
	}
	return m.getStateSummary()
}

func (m *GenericTxSystem) getStateSummary() (StateSummary, error) {
	sv, hash, err := m.state.CalculateRoot()
	if err != nil {
		return nil, err
	}
	return NewStateSummary(hash, util.Uint64ToBytes(sv)), nil
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

func (m *GenericTxSystem) pruneState(roundNo uint64) error {
	return m.state.Prune()
}

func (m *GenericTxSystem) snFees(_ *types.TransactionOrder, execCxt txtypes.ExecutionContext) error {
	return execCxt.SpendGas(abfc.GeneralTxCostGasUnits)
}

func (m *GenericTxSystem) Execute(tx *types.TransactionOrder) (*types.TransactionRecord, error) {
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
		sm, err := m.executeFc(tx, exeCtx)
		if err != nil {
			return nil, fmt.Errorf("execute fc error: %w", err)
		}
		return sm, nil
	}
	// execute rest ordinary transactions
	if err := m.fees.IsCredible(exeCtx, tx); err != nil {
		// not credible, means that no fees can be charged, so just exit with error tx will not be added to block
		return nil, fmt.Errorf("error transaction not credible: %w", err)
	}
	tr, err := m.doExecute(tx, exeCtx)
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
	// check conditional state unlock and release it, either roll back or execute the pending Tx
	unlockSm, err := m.handleUnlockUnitState(tx, exeCtx)
	txr.ServerMetadata = appendServerMetadata(txr.ServerMetadata, unlockSm)
	if err != nil {
		txExecErr = fmt.Errorf("unit state lock error: %w", err)
		return txr, nil
	}
	// perform transaction-system-specific validation and owner predicate check
	attr, authProof, err := m.handlers.Validate(tx, exeCtx)
	if err != nil {
		txExecErr = fmt.Errorf("transaction validation error (type=%d): %w", tx.Type, err)
		return txr, nil
	}
	// either state lock or execute Tx
	if tx.HasStateLock() {
		// handle conditional lock of units
		sm, err := m.executeLockUnitState(tx, exeCtx)
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
	// 4. If P.C , ⊥ then return ⊥ – discard P if it is conditional
	if tx.StateLock != nil {
		return nil, fmt.Errorf("error fc transaction contains state lock")
	}
	// will not check 5. S.N[P.ι].L != ⊥ and S .N[P.ι].L.Ppend.τ != nop then return ⊥,
	// if we do no handle state locking and unlocking then lock state must be impossible state
	// 6. If S.N[P.ι] != ⊥ and not EvalPred(S.N[P.ι].φ; S, P, P.s; &MS) - will be done during VerifyPsi
	// 7. If not VerifyPsi(S, P; &MS)
	// perform transaction-system-specific validation and owner predicate check
	attr, authProof, err := m.handlers.Validate(tx, exeCtx)
	if err != nil {
		return nil, err
	}
	// 8. b ← SNFees(S, P; &MS) - is at the moment done for all tx before this call
	// 9. TakeSnapshot
	savepointID, err := m.state.Savepoint()
	if err != nil {
		return nil, fmt.Errorf("savepoint error: %w", err)
	}
	// skip step 10 b ← Unlock(S, P; &MS) - nothing to unlock if state lock is disabled in step 4?
	// skip 11 If S .N[P.ι].L != ⊥ - unlock fail, as currently no attempt is made to unlock
	// proceed with the transaction execution
	sm, err := m.handlers.ExecuteWithAttr(tx, attr, authProof, exeCtx)
	if err != nil {
		m.state.RollbackToSavepoint(savepointID)
		return nil, fmt.Errorf("execute error: %w", err)
	}
	txBytes, err := tx.MarshalCBOR()
	if err != nil {
		m.state.RollbackToSavepoint(savepointID)
		return nil, fmt.Errorf("marshalling transaction: %w", err)
	}
	trx := &types.TransactionRecord{
		Version:          1,
		TransactionOrder: txBytes,
		ServerMetadata:   sm,
	}
	trHash, err := trx.Hash(m.hashAlgorithm)
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
	return trx, nil
}

/*
validateGenericTransaction does the tx validation common to all tx systems.

See Yellowpaper chapter 4.6 "Valid Transaction Orders".
The (final) step "ψτ(P,S) – type-specific validity condition holds" must be
implemented by the tx handler.
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

func (m *GenericTxSystem) EndBlock() (StateSummary, error) {
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
}

func (m *GenericTxSystem) Commit(uc *types.UnicityCertificate) error {
	err := m.state.Commit(uc)
	if err == nil {
		m.roundCommitted = true
	}
	return err
}

func (m *GenericTxSystem) CommittedUC() *types.UnicityCertificate {
	return m.state.CommittedUC()
}

func (m *GenericTxSystem) SerializeState(writer io.Writer, committed bool) error {
	return m.state.Serialize(writer, committed)
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
