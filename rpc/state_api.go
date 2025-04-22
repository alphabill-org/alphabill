package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

type (
	StateAPI struct {
		node       partitionNode
		ownerIndex partition.IndexReader

		pdr          *types.PartitionDescriptionRecord
		withGetUnits bool

		requestLimiter    *RequestLimiter
		responseItemLimit int

		updMetrics    func(ctx context.Context, method string, start time.Time, apiErr error)
		updTxReceived func(ctx context.Context, txType uint16, apiErr error)
	}

	partitionNode interface {
		NetworkID() types.NetworkID
		PartitionID() types.PartitionID
		PartitionTypeID() types.PartitionTypeID
		ShardID() types.ShardID
		SubmitTx(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetBlock(ctx context.Context, blockNr uint64) (*types.Block, error)
		LatestBlockNumber() (uint64, error)
		GetTransactionRecordProof(ctx context.Context, hash []byte) (*types.TxRecordProof, error)
		CurrentRoundInfo(ctx context.Context) (*partition.RoundInfo, error)
		TransactionSystemState() txsystem.StateReader
		SerializeState(w io.Writer) error
		Validators() peer.IDSlice
		RegisterValidatorAssignmentRecord(v *partitions.ValidatorAssignmentRecord) error
		GetTrustBase(epochNumber uint64) (types.RootTrustBase, error)
		IsPermissionedMode() bool
		IsFeelessMode() bool
	}

	Unit[T any] struct {
		NetworkID   types.NetworkID       `json:"networkId"`
		PartitionID types.PartitionID     `json:"partitionId"`
		UnitID      types.UnitID          `json:"unitId"`
		Data        T                     `json:"data"`
		StateProof  *types.UnitStateProof `json:"stateProof,omitempty"`
		StateLockTx hex.Bytes             `json:"stateLockTx,omitempty"`
	}

	TransactionRecordAndProof struct {
		TxRecordProof hex.Bytes `json:"txRecordProof"` // hex encoded CBOR of types.TxRecordProof
	}
)

func NewStateAPI(node partitionNode, obs Observability, opts ...StateAPIOption) *StateAPI {
	m := obs.Meter(metricsScopeJRPCAPI)
	log := obs.Logger()
	options := defaultStateAPIOptions()
	for _, opt := range opts {
		opt(options)
	}

	requestLimiter := NewRequestLimiter(
		options.rateLimit,
		[]RequestTokenCost{
			{"getRoundInfo", 1},
			{"getUnit", 20},
			{"getUnitsByOwnerID", 100},
			{"getUnits", 100},
			{"sendTransaction", 1},
			{"getTransactionProof", 1},
			{"getBlock", 1},
			{"getTrustBase", 1},
		},
		log,
	)

	return &StateAPI{
		node:              node,
		ownerIndex:        options.ownerIndex,
		pdr:               options.pdr,
		withGetUnits:      options.withGetUnits,
		updMetrics:        metricsUpdater(m, node, log),
		updTxReceived:     metricsUpdaterTxReceived(m, node, log),
		requestLimiter:    requestLimiter,
		responseItemLimit: options.responseItemLimit,
	}
}

// GetRoundInfo returns the current round number and epoch as seen by the node.
func (s *StateAPI) GetRoundInfo(ctx context.Context) (_ *partition.RoundInfo, retErr error) {
	defer func(start time.Time) { s.updMetrics(ctx, "getRoundInfo", start, retErr) }(time.Now())
	if err := s.requestLimiter.CheckRequestAllowed("getRoundInfo"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}
	return s.node.CurrentRoundInfo(ctx)
}

// GetUnit returns unit data and optionally the state proof for the given unitID.
func (s *StateAPI) GetUnit(unitID types.UnitID, includeStateProof bool) (_ *Unit[any], retErr error) {
	defer func(start time.Time) { s.updMetrics(context.Background(), "getUnit", start, retErr) }(time.Now())
	if err := s.requestLimiter.CheckRequestAllowed("getUnit"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}

	st := s.node.TransactionSystemState()
	unit, err := st.GetUnit(unitID, true)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	unitV1, err := state.ToUnitV1(unit)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unit to v1: %w", err)
	}

	resp := &Unit[any]{
		NetworkID:   s.node.NetworkID(),
		PartitionID: s.node.PartitionID(),
		UnitID:      unitID,
		Data:        unit.Data(),
		StateProof:  nil,
		StateLockTx: unitV1.StateLockTx(),
	}

	if includeStateProof {
		u, err := state.ToUnitV1(unit)
		if err != nil {
			return nil, fmt.Errorf("failed to convert unit to version 1: %w", err)
		}
		stateProof, err := st.CreateUnitStateProof(unitID, u.LastLogIndex())
		if err != nil {
			return nil, fmt.Errorf("failed to generate unit state proof: %w", err)
		}
		resp.StateProof = stateProof
	}

	return resp, nil
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (s *StateAPI) GetUnitsByOwnerID(ownerID hex.Bytes, sinceUnitID *types.UnitID, limit *int) (_ []types.UnitID, retErr error) {
	defer func(start time.Time) { s.updMetrics(context.Background(), "getUnitsByOwnerID", start, retErr) }(time.Now())
	if s.ownerIndex == nil {
		return nil, errors.New("owner indexer is disabled")
	}
	if err := s.requestLimiter.CheckRequestAllowed("getUnitsByOwnerID"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}

	ownerUnitIDs, err := s.ownerIndex.GetOwnerUnits(ownerID, sinceUnitID)
	if err != nil {
		return nil, fmt.Errorf("failed to load owner units: %w", err)
	}
	responseLimit := s.responseLimit(limit)

	endIndex := endIndex(0, responseLimit, ownerUnitIDs)
	return ownerUnitIDs[:endIndex], nil
}

// GetUnits returns list of unit identifiers, optionally filtered by the given unit type identifier.
func (s *StateAPI) GetUnits(unitTypeID *uint32, sinceUnitID *types.UnitID, limit *int) (_ []types.UnitID, retErr error) {
	defer func(start time.Time) { s.updMetrics(context.Background(), "getUnits", start, retErr) }(time.Now())
	if !s.withGetUnits {
		return nil, errors.New("state_getUnits is disabled")
	}
	if err := s.requestLimiter.CheckRequestAllowed("getUnits"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}
	units, err := s.node.TransactionSystemState().GetUnits(unitTypeID, s.pdr)
	if err != nil {
		return nil, fmt.Errorf("failed to get units: %w", err)
	}
	startIndex := startIndex(sinceUnitID, units)
	if startIndex >= len(units) {
		return []types.UnitID{}, nil
	}
	responseLimit := s.responseLimit(limit)
	endIndex := endIndex(startIndex, responseLimit, units)

	return units[startIndex:endIndex], nil
}

// SendTransaction broadcasts the given transaction to the network, returns the submitted transaction hash.
func (s *StateAPI) SendTransaction(ctx context.Context, txBytes hex.Bytes) (_ hex.Bytes, retErr error) {
	defer func(start time.Time) { s.updMetrics(ctx, "sendTransaction", start, retErr) }(time.Now())
	if err := s.requestLimiter.CheckRequestAllowed("sendTransaction"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}
	var tx *types.TransactionOrder
	if err := types.Cbor.Unmarshal(txBytes, &tx); err != nil {
		s.updTxReceived(ctx, 0, err)
		return nil, fmt.Errorf("failed to decode transaction: %w", err)
	}

	defer func() { s.updTxReceived(ctx, tx.Type, retErr) }()
	txHash, err := s.node.SubmitTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction to the network: %w", err)
	}
	return txHash, nil
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
func (s *StateAPI) GetTransactionProof(ctx context.Context, txHash hex.Bytes) (_ *TransactionRecordAndProof, retErr error) {
	defer func(start time.Time) { s.updMetrics(ctx, "getTransactionProof", start, retErr) }(time.Now())
	if err := s.requestLimiter.CheckRequestAllowed("getTransactionProof"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}
	txRecordProof, err := s.node.GetTransactionRecordProof(ctx, txHash)
	if err != nil {
		if errors.Is(err, partition.ErrIndexNotFound) || errors.Is(err, types.ErrBlockIsNil) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load tx record: %w", err)
	}
	txRecordProofCBOR, err := types.Cbor.Marshal(txRecordProof)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx record: %w", err)
	}
	return &TransactionRecordAndProof{
		TxRecordProof: txRecordProofCBOR,
	}, nil
}

// GetBlock returns block for the given block number.
func (s *StateAPI) GetBlock(ctx context.Context, blockNumber hex.Uint64) (_ hex.Bytes, retErr error) {
	defer func(start time.Time) { s.updMetrics(ctx, "getBlock", start, retErr) }(time.Now())
	if err := s.requestLimiter.CheckRequestAllowed("getBlock"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}
	block, err := s.node.GetBlock(ctx, uint64(blockNumber))
	if err != nil {
		return nil, fmt.Errorf("failed to load block: %w", err)
	}
	if block == nil {
		return nil, nil
	}
	blockCbor, err := types.Cbor.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("failed to encode block: %w", err)
	}
	return blockCbor, nil
}

// GetTrustBase returns trust base for the given epoch.
func (s *StateAPI) GetTrustBase(epochNumber hex.Uint64) (_ types.RootTrustBase, retErr error) {
	defer func(start time.Time) { s.updMetrics(context.Background(), "getTrustBase", start, retErr) }(time.Now())
	if err := s.requestLimiter.CheckRequestAllowed("getTrustBase"); err != nil {
		return nil, fmt.Errorf("request not allowed: %w", err)
	}
	trustBase, err := s.node.GetTrustBase(uint64(epochNumber))
	if err != nil {
		return nil, fmt.Errorf("failed to load trust base: %w", err)
	}
	return trustBase, nil
}

// startIndex returns next index from sinceUnitID.
func startIndex(sinceUnitID *types.UnitID, ownerUnitIDs []types.UnitID) int {
	if sinceUnitID == nil {
		return 0
	}
	index := slices.IndexFunc(ownerUnitIDs, func(n types.UnitID) bool {
		return n.Compare(*sinceUnitID) == 0
	})
	return index + 1
}

func endIndex(startIndex int, limit int, ownerUnitIDs []types.UnitID) int {
	if limit == 0 {
		return len(ownerUnitIDs)
	}
	endIndex := min(startIndex+limit, len(ownerUnitIDs))
	return endIndex
}

func (s *StateAPI) responseLimit(limit *int) int {
	if limit == nil || *limit == 0 {
		return s.responseItemLimit
	}
	if s.responseItemLimit == 0 {
		return *limit
	}
	return min(s.responseItemLimit, *limit)
}
