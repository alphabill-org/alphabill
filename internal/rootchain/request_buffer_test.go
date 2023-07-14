package rootchain

import (
	"bytes"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

var IR1 = &types.InputRecord{Hash: []byte{1}}
var IR2 = &types.InputRecord{Hash: []byte{2}}
var IR3 = &types.InputRecord{Hash: []byte{3}}
var SysID1 = []byte{0, 0, 0, 1}
var SysID2 = []byte{0, 0, 0, 2}

var req1 = &certification.BlockCertificationRequest{
	InputRecord: IR1,
}

// Test internals
func Test_requestStore_add(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.ErrorContains(t, err, "request in this round already stored, rejected")
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// try to store a different IR
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR2}, trustBase)
	require.ErrorContains(t, err, "request in this round already stored, rejected")
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// node 2 request
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.Len(t, proof, 2)
	require.Equal(t, 2, len(rs.nodeRequest))
	require.Equal(t, 1, len(rs.requests))
	_, res = rs.isConsensusReceived(trustBase)
	require.Equal(t, QuorumAchieved, res)
	for _, certReq := range rs.requests {
		require.Len(t, certReq, 2)
	}
}

func Test_requestStore_QuorumNotPossibleWith2Nodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumNotPossible, res)
	require.Len(t, proof, 2)
	proof, res = rs.isConsensusReceived(trustBase)
	require.Equal(t, QuorumNotPossible, res)
	require.NotNil(t, proof)
}

func TestRequestStore_isConsensusReceived_SingleNode(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil})
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 1)
	require.Equal(t, req1.InputRecord, proof[0].InputRecord)
}

func TestRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumNotPossible, res)
	require.NotNil(t, proof)
	rs.reset()
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 2)
	require.Equal(t, req1.InputRecord, proof[0].InputRecord)
}

func TestRequestStore_isConsensusReceived_FiveNodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil, "3": nil, "4": nil, "5": nil})
	// 1.
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 2.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 3.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 3)
	// 4. change one filed and make sure it is not part of consensus
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: &types.InputRecord{Hash: bytes.Clone(IR1.Hash), SumOfEarnedFees: 10}}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	// proof is still 3, do not include invalid requests in quorum proof (at the moment)
	require.Len(t, proof, 3)
	// 5.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	// proof is still 3, do not include invalid requests in quorum proof (at the moment)
	require.Len(t, proof, 4)
}

func TestRequestStore_isConsensusNotPossible_FiveNodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil, "3": nil, "4": nil, "5": nil})
	// 1.
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 2.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 3.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 4.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 50/50 split, but validator 5 sends a unique req
	// 5.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: IR3}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumNotPossible, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 5)
}

func TestCertRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	// 1.
	res, proof, err := cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 2.
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumNotPossible, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 2)
	require.Equal(t, QuorumNotPossible, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase))
	cs.Clear(protocol.SystemIdentifier(SysID1))
	// test all nodeRequest cleared
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 2)
	require.Equal(t, req1.InputRecord, proof[0].InputRecord)
	require.Equal(t, req1.InputRecord, proof[1].InputRecord)
}

func TestCertRequestStore_isConsensusReceived_MultipleSystemId(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "1", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase))
	// add more requests
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 2)
	require.Equal(t, IR1, proof[0].InputRecord)
	require.Equal(t, IR1, proof[1].InputRecord)
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "2", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Equal(t, QuorumAchieved, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase))
	require.Equal(t, QuorumAchieved, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase))
	require.Len(t, proof, 2)
	require.Equal(t, IR2, proof[0].InputRecord)
	require.Equal(t, IR2, proof[1].InputRecord)
	// Reset resets both stores
	cs.Reset()
	// test all nodeRequest cleared
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).requests)
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).nodeRequest)
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID2)).requests)
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID2)).nodeRequest)
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase))
}

func TestCertRequestStore_clearOne(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "1", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	res, proof, err = cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "2", InputRecord: IR2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	// clear sys id 1
	cs.Clear(protocol.SystemIdentifier(SysID1))
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).requests)
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).nodeRequest)
	require.Equal(t, QuorumAchieved, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase))
}

func TestCertRequestStore_EmptyStore(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).requests)
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).nodeRequest)
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).requests)
	require.Empty(t, cs.get(protocol.SystemIdentifier(SysID1)).nodeRequest)
	// Reset resets both stores
	require.NotPanics(t, func() { cs.Reset() })
	require.NotPanics(t, func() { cs.Clear(protocol.SystemIdentifier(SysID1)) })
	require.NotPanics(t, func() { cs.Clear(protocol.SystemIdentifier([]byte{1, 1, 1, 1, 1, 1})) })
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase))
}
