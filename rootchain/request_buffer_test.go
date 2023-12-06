package rootchain

import (
	"bytes"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

var ir1 = &types.InputRecord{Hash: []byte{1}}
var ir2 = &types.InputRecord{Hash: []byte{2}}
var ir3 = &types.InputRecord{Hash: []byte{3}}
var sysID1 = types.SystemID32(1)
var sysID2 = types.SystemID32(2)

var req1 = &certification.BlockCertificationRequest{
	InputRecord: ir1,
}

// Test internals
func Test_requestStore_add(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.ErrorContains(t, err, "request in this round already stored, rejected")
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// try to store a different IR
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir2}, trustBase)
	require.ErrorContains(t, err, "request in this round already stored, rejected")
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// node 2 request
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: ir1}, trustBase)
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
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: ir2}, trustBase)
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
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 1)
	require.Equal(t, req1.InputRecord, proof[0].InputRecord)
}

func TestRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumNotPossible, res)
	require.NotNil(t, proof)
	rs.reset()
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: ir1}, trustBase)
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
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 2.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 3.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 3)
	// 4. change one filed and make sure it is not part of consensus
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: &types.InputRecord{Hash: bytes.Clone(ir1.Hash), SumOfEarnedFees: 10}}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	// proof is still 3, do not include invalid requests in quorum proof (at the moment)
	require.Len(t, proof, 3)
	// 5.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: ir1}, trustBase)
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
	res, proof, err := rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 2.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 3.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 4.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 50/50 split, but validator 5 sends a unique req
	// 5.
	res, proof, err = rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: ir3}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumNotPossible, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 5)
}

func TestCertRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	// 1.
	res, proof, err := cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	// 2.
	res, proof, err = cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "2", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumNotPossible, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 2)
	require.Equal(t, QuorumNotPossible, cs.IsConsensusReceived(sysID1, trustBase))
	cs.Clear(sysID1)
	// test all nodeRequest cleared
	res, proof, err = cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "2", InputRecord: ir1}, trustBase)
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
	res, proof, err := cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(sysID2, &certification.BlockCertificationRequest{SystemIdentifier: sysID2.ToSystemID(), NodeIdentifier: "1", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(sysID1, trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(sysID2, trustBase))
	// add more requests
	res, proof, err = cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "2", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Len(t, proof, 2)
	require.Equal(t, ir1, proof[0].InputRecord)
	require.Equal(t, ir1, proof[1].InputRecord)
	res, proof, err = cs.Add(sysID2, &certification.BlockCertificationRequest{SystemIdentifier: sysID2.ToSystemID(), NodeIdentifier: "2", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	require.Equal(t, QuorumAchieved, cs.IsConsensusReceived(sysID1, trustBase))
	require.Equal(t, QuorumAchieved, cs.IsConsensusReceived(sysID2, trustBase))
	require.Len(t, proof, 2)
	require.Equal(t, ir2, proof[0].InputRecord)
	require.Equal(t, ir2, proof[1].InputRecord)
	// Reset resets both stores
	cs.Reset()
	// test all nodeRequest cleared
	require.Empty(t, cs.get(sysID1).requests)
	require.Empty(t, cs.get(sysID1).nodeRequest)
	require.Empty(t, cs.get(sysID1).requests)
	require.Empty(t, cs.get(sysID1).nodeRequest)
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(sysID1, trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(sysID1, trustBase))
}

func TestCertRequestStore_clearOne(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	res, proof, err := cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "1", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(sysID2, &certification.BlockCertificationRequest{SystemIdentifier: sysID2.ToSystemID(), NodeIdentifier: "1", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumInProgress, res)
	require.Nil(t, proof)
	res, proof, err = cs.Add(sysID1, &certification.BlockCertificationRequest{SystemIdentifier: sysID1.ToSystemID(), NodeIdentifier: "2", InputRecord: ir1}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	res, proof, err = cs.Add(sysID2, &certification.BlockCertificationRequest{SystemIdentifier: sysID2.ToSystemID(), NodeIdentifier: "2", InputRecord: ir2}, trustBase)
	require.NoError(t, err)
	require.Equal(t, QuorumAchieved, res)
	require.NotNil(t, proof)
	// clear sys id 1
	cs.Clear(sysID1)
	require.Empty(t, cs.get(sysID1).requests)
	require.Empty(t, cs.get(sysID1).nodeRequest)
	require.Equal(t, QuorumAchieved, cs.IsConsensusReceived(sysID2, trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(sysID1, trustBase))
}

func TestCertRequestStore_EmptyStore(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.Empty(t, cs.get(sysID1).requests)
	require.Empty(t, cs.get(sysID1).nodeRequest)
	require.Empty(t, cs.get(sysID1).requests)
	require.Empty(t, cs.get(sysID1).nodeRequest)
	// Reset resets both stores
	require.NotPanics(t, func() { cs.Reset() })
	require.NotPanics(t, func() { cs.Clear(sysID1) })
	require.NotPanics(t, func() { cs.Clear(types.SystemID32(0x1010101)) })
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(sysID1, trustBase))
	require.Equal(t, QuorumInProgress, cs.IsConsensusReceived(sysID1, trustBase))
}
