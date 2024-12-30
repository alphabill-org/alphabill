package consensus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type mockIRVerifier struct {
	verify func(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error)
}

func (x *mockIRVerifier) VerifyIRChangeReq(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error) {
	return x.verify(round, irChReq)
}

func Test_IrReqBuffer_Add(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		PartitionID: 1,
	}

	IrChReqMsg := drctypes.IRChangeReq{
		Partition:  pdr.PartitionID,
		CertReason: drctypes.Quorum,
		Requests: []*certification.BlockCertificationRequest{{
			PartitionID: pdr.PartitionID,
			NodeID:      "1",
			InputRecord: &types.InputRecord{
				Version:         1,
				PreviousHash:    []byte{1, 1, 1},
				Hash:            []byte{2, 2, 2},
				BlockHash:       []byte{3, 3, 3},
				SummaryValue:    []byte{4, 4, 4},
				RoundNumber:     5,
				SumOfEarnedFees: 6,
			},
		}},
	}

	verifier := &mockIRVerifier{
		verify: func(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error) {
			return &storage.InputData{Partition: irChReq.Partition, IR: irChReq.Requests[0].InputRecord, PDRHash: []byte{0, 0, 0, 0, 1}}, nil
		},
	}

	t.Run("nil request", func(t *testing.T) {
		reqBuffer := NewIrReqBuffer(nil, nil, time.Second, logger.New(t))
		require.EqualError(t, reqBuffer.Add(3, nil, verifier), "ir change request is nil")
		require.Empty(t, reqBuffer.irChgReqBuffer)
	})

	t.Run("timeout request", func(t *testing.T) {
		chReqTO := IrChReqMsg
		chReqTO.CertReason = drctypes.T2Timeout

		reqBuffer := NewIrReqBuffer(nil, nil, time.Second, logger.New(t))
		require.ErrorContains(t, reqBuffer.Add(3, &chReqTO, verifier),
			"invalid ir change request, timeout can only be proposed by leader issuing a new block")
		require.Empty(t, reqBuffer.irChgReqBuffer)
	})

	t.Run("duplicate request", func(t *testing.T) {
		reqBuffer := NewIrReqBuffer(nil, nil, time.Second, logger.New(t))
		require.NoError(t, reqBuffer.Add(3, &IrChReqMsg, verifier))
		require.Len(t, reqBuffer.irChgReqBuffer, 1)

		// attempt to add the same request again logs error but doesn't return it!
		require.NoError(t, reqBuffer.Add(3, &IrChReqMsg, verifier))
		require.Len(t, reqBuffer.irChgReqBuffer, 1)
	})

	t.Run("reason changed", func(t *testing.T) {
		reqBuffer := NewIrReqBuffer(nil, nil, time.Second, logger.New(t))
		require.NoError(t, reqBuffer.Add(3, &IrChReqMsg, verifier))
		require.Len(t, reqBuffer.irChgReqBuffer, 1)
		// try to change reason
		chReqTO := IrChReqMsg
		chReqTO.CertReason = drctypes.QuorumNotPossible
		require.EqualError(t, reqBuffer.Add(3, &chReqTO, verifier),
			"equivocating request for partition 00000001, reason has changed")
		require.Len(t, reqBuffer.irChgReqBuffer, 1)
	})

	t.Run("equivocating request", func(t *testing.T) {
		reqBuffer := NewIrReqBuffer(nil, nil, time.Second, logger.New(t))
		require.NoError(t, reqBuffer.Add(3, &IrChReqMsg, verifier))
		require.Len(t, reqBuffer.irChgReqBuffer, 1)
		// change IR and try to add, must be rejected as equivocating, we already have a valid request
		chReqTO := IrChReqMsg
		chReqTO.Requests[0].InputRecord = &types.InputRecord{
			Version:         1,
			PreviousHash:    []byte{1, 1, 1},
			Hash:            []byte{5, 5, 5},
			BlockHash:       []byte{3, 3, 3},
			SummaryValue:    []byte{4, 4, 4},
			RoundNumber:     5,
			SumOfEarnedFees: 6,
		}
		require.EqualError(t, reqBuffer.Add(3, &chReqTO, verifier), "equivocating request for partition 00000001")
		require.Len(t, reqBuffer.irChgReqBuffer, 1)
	})

	t.Run("request verification fails", func(t *testing.T) {
		expErr := errors.New("some error")
		ver := &mockIRVerifier{
			verify: func(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error) {
				return nil, expErr
			},
		}
		reqBuffer := NewIrReqBuffer(nil, nil, time.Second, logger.New(t))
		require.EqualError(t, reqBuffer.Add(3, &IrChReqMsg, ver), "invalid IR Change Request: some error")
		require.Empty(t, reqBuffer.irChgReqBuffer)
	})
}

func Test_IrReqBuffer_GeneratePayload(t *testing.T) {
	pdr := types.PartitionDescriptionRecord{
		PartitionID: 1,
		T2Timeout:   2 * time.Second,
	}

	getPartitions := func(rootRound uint64) ([]*types.PartitionDescriptionRecord, error) {
		return []*types.PartitionDescriptionRecord{&pdr}, nil
	}

	state := mockState{
		changeInProgress: func(types.PartitionID, types.ShardID) *types.InputRecord { return nil },
		shardInfo: func(types.PartitionID, types.ShardID) (*storage.ShardInfo, error) {
			return &storage.ShardInfo{
				LastCR: &certification.CertificationResponse{
					UC: types.UnicityCertificate{
						UnicitySeal: &types.UnicitySeal{
							RootChainRoundNumber: 3,
						},
					},
				},
			}, nil
		},
	}

	verifier := &mockIRVerifier{
		verify: func(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error) {
			return &storage.InputData{Partition: irChReq.Partition, IR: irChReq.Requests[0].InputRecord, PDRHash: []byte{0, 0, 0, 0, 1}}, nil
		},
	}

	IrChReqMsg := &drctypes.IRChangeReq{
		Partition:  pdr.PartitionID,
		CertReason: drctypes.Quorum,
		Requests: []*certification.BlockCertificationRequest{
			{
				PartitionID: pdr.PartitionID,
				NodeID:      "1",
				InputRecord: &types.InputRecord{
					Version:         1,
					PreviousHash:    []byte{1, 1, 1},
					Hash:            []byte{2, 2, 2},
					BlockHash:       []byte{3, 3, 3},
					SummaryValue:    []byte{4, 4, 4},
					RoundNumber:     5,
					SumOfEarnedFees: 6,
				},
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		reqBuffer := NewIrReqBuffer(&state, getPartitions, time.Second, logger.New(t))
		// no requests, no timeouts, generate payload
		payload, err := reqBuffer.GeneratePayload(context.Background(), 3)
		require.NoError(t, err)
		require.Empty(t, payload.Requests)
		require.Empty(t, reqBuffer.irChgReqBuffer)
		// add request
		require.NoError(t, reqBuffer.Add(3, IrChReqMsg, verifier))
		require.True(t, reqBuffer.isChangeInBuffer(IrChReqMsg.Partition, IrChReqMsg.Shard))
		// generate proposal payload, one request in buffer
		payload, err = reqBuffer.GeneratePayload(context.Background(), 3)
		require.NoError(t, err)
		require.Len(t, payload.Requests, 1)
		require.Equal(t, IrChReqMsg, payload.Requests[0])
		require.False(t, reqBuffer.isChangeInBuffer(IrChReqMsg.Partition, IrChReqMsg.Shard), "expected buffer to be reset")
		// generate payload, it must be empty again
		payload, err = reqBuffer.GeneratePayload(context.Background(), 4)
		require.NoError(t, err)
		require.Empty(t, payload.Requests)
	})

	t.Run("CR buffered but also change is pending", func(t *testing.T) {
		state := mockState{
			changeInProgress: func(pi types.PartitionID, si types.ShardID) *types.InputRecord {
				return &types.InputRecord{} // just need non-nil value
			},
		}
		reqBuffer := NewIrReqBuffer(&state, getPartitions, time.Second, logger.New(t))
		// add request
		require.NoError(t, reqBuffer.Add(3, IrChReqMsg, verifier))
		require.True(t, reqBuffer.isChangeInBuffer(IrChReqMsg.Partition, IrChReqMsg.Shard))
		// payload must be empty as there is change in progress
		payload, err := reqBuffer.GeneratePayload(context.Background(), 3)
		require.NoError(t, err)
		require.Empty(t, payload.Requests)
		require.Empty(t, reqBuffer.irChgReqBuffer)
	})

	t.Run("generate timeout", func(t *testing.T) {
		// timeout will be triggered when
		// currentRound - lastCertifiedRound >= timeoutRounds
		// where timeoutRounds = (pdr.T2Timeout / (blockRate/2))+1
		// so if blockRate=2*pdr.T2Timeout TO will be triggered by 2 rounds
		reqBuffer := NewIrReqBuffer(&state, getPartitions, 2*pdr.T2Timeout, logger.New(t))
		payload, err := reqBuffer.GeneratePayload(context.Background(), 5)
		require.NoError(t, err)
		require.Len(t, payload.Requests, 1)
		require.Equal(t, pdr.PartitionID, payload.Requests[0].Partition)
		require.Equal(t, drctypes.T2Timeout, payload.Requests[0].CertReason)
		require.Empty(t, payload.Requests[0].Requests)
	})

	t.Run("get partitions fails", func(t *testing.T) {
		getPartitions := func(rootRound uint64) ([]*types.PartitionDescriptionRecord, error) {
			return nil, errors.New("no PDRs")
		}
		reqBuffer := NewIrReqBuffer(&state, getPartitions, pdr.T2Timeout, logger.New(t))
		payload, err := reqBuffer.GeneratePayload(context.Background(), 4)
		require.EqualError(t, err, `loading PDRs of the round 4: no PDRs`)
		require.Nil(t, payload)
	})

	t.Run("no ShardInfo", func(t *testing.T) {
		// no pending changes, checking do we need to generate timeout fails
		// because loading ShardInfo of the shard fails
		state := mockState{
			changeInProgress: func(types.PartitionID, types.ShardID) *types.InputRecord { return nil },
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return nil, errors.New("no SI")
			},
		}
		reqBuffer := NewIrReqBuffer(&state, getPartitions, pdr.T2Timeout, logger.New(t))
		payload, err := reqBuffer.GeneratePayload(context.Background(), 4)
		require.EqualError(t, err, `load shard 00000001- info: no SI`)
		require.Nil(t, payload)
	})
}
