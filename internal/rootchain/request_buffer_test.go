package rootchain

import (
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
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.ErrorContains(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}), "duplicated request")
	require.ErrorContains(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR2}), "equivocating request with different hash")
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}))
	require.Equal(t, 2, len(rs.nodeRequest))
	require.Equal(t, 2, len(rs.requests))
	for _, certReq := range rs.requests {
		require.Len(t, certReq, 1)
	}
}

func Test_requestStore_isConsensusReceived(t *testing.T) {
	rs := newRequestStore()
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}))
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	_, possible := rs.isConsensusReceived(trustBase)
	require.False(t, possible)
}

func TestRequestStore_isConsensusReceived_SingleNode(t *testing.T) {
	rs := newRequestStore()
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil})
	ir, possible := rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}))
	_, possible := rs.isConsensusReceived(trustBase)
	require.False(t, possible)
	rs.reset()
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible := rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_FiveNodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil, "3": nil, "4": nil, "5": nil})
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	ir, possible := rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Nil(t, ir)
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

}

func TestRequestStore_isConsensusNotPossible_FiveNodes(t *testing.T) {
	rs := newRequestStore()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil, "3": nil, "4": nil, "5": nil})
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	ir, possible := rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: IR2}))
	ir, possible = rs.isConsensusReceived(trustBase)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: IR2}))
	_, possible = rs.isConsensusReceived(trustBase)
	require.True(t, possible)

	// 50/50 split, but validator 5 sends a unique req
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: IR3}))
	_, possible = rs.isConsensusReceived(trustBase)
	require.False(t, possible)
}

// Test interface
func Test_CertStore_add(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}))
	require.ErrorContains(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}), "duplicated request")
	require.ErrorContains(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR2}), "equivocating request with different hash")
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR2}))
	_, possible := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase)
	require.False(t, possible)
}

func TestCertRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR2}))
	_, possible := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase)
	require.False(t, possible)
	cs.Clear(protocol.SystemIdentifier(SysID1))
	// test all nodeRequest cleared
	require.Empty(t, cs.GetRequests(protocol.SystemIdentifier(SysID1)))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestCertRequestStore_isConsensusReceived_MultipleSystemId(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "1", InputRecord: IR2}))
	_, possible := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase)
	require.True(t, possible)
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "2", InputRecord: IR2}))
	ir, possible := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase)
	require.True(t, possible)
	require.Equal(t, IR1, ir)
	require.Equal(t, len(cs.GetRequests(protocol.SystemIdentifier(SysID1))), 2)
	ir2, possible2 := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase)
	require.True(t, possible2)
	require.Equal(t, IR2, ir2)
	require.Equal(t, len(cs.GetRequests(protocol.SystemIdentifier(SysID2))), 2)
	// Reset resets both stores
	cs.Reset()
	// test all nodeRequest cleared
	require.Empty(t, cs.GetRequests(protocol.SystemIdentifier(SysID1)))
	require.Empty(t, cs.GetRequests(protocol.SystemIdentifier(SysID2)))
	ir3, possible3 := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase)
	require.True(t, possible3)
	require.Nil(t, ir3)
	ir4, possible4 := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase)
	require.True(t, possible4)
	require.Nil(t, ir4)
}

func TestCertRequestStore_clearOne(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "1", InputRecord: IR2}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID1, NodeIdentifier: "2", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysID2, NodeIdentifier: "2", InputRecord: IR2}))
	cs.Clear(protocol.SystemIdentifier(SysID1))
	require.Empty(t, cs.GetRequests(protocol.SystemIdentifier(SysID1)))
	ir2, possible2 := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase)
	require.True(t, possible2)
	require.Equal(t, IR2, ir2)
	require.Equal(t, len(cs.GetRequests(protocol.SystemIdentifier(SysID2))), 2)
}

func TestCertRequestStore_EmptyStore(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	trustBase := partitions.NewPartitionTrustBase(map[string]crypto.Verifier{"1": nil, "2": nil})
	require.Equal(t, len(cs.GetRequests(protocol.SystemIdentifier(SysID1))), 0)
	require.Equal(t, len(cs.GetRequests(protocol.SystemIdentifier(SysID2))), 0)
	// Reset resets both stores
	require.NotPanics(t, func() { cs.Reset() })
	require.NotPanics(t, func() { cs.Clear(protocol.SystemIdentifier(SysID1)) })
	require.NotPanics(t, func() { cs.Clear(protocol.SystemIdentifier([]byte{1, 1, 1, 1, 1, 1})) })
	ir3, possible3 := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID1), trustBase)
	require.True(t, possible3)
	require.Nil(t, ir3)
	ir4, possible4 := cs.IsConsensusReceived(protocol.SystemIdentifier(SysID2), trustBase)
	require.True(t, possible4)
	require.Nil(t, ir4)
}
