package rootvalidator

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/stretchr/testify/require"
)

var IR1 = &certificates.InputRecord{Hash: []byte{1}}
var IR2 = &certificates.InputRecord{Hash: []byte{2}}
var IR3 = &certificates.InputRecord{Hash: []byte{3}}
var SysId1 = []byte{0, 0, 0, 1}
var SysId2 = []byte{0, 0, 0, 2}

var req1 = &certification.BlockCertificationRequest{
	InputRecord: IR1,
}

var req2 = &certification.BlockCertificationRequest{
	InputRecord: IR2,
}

var req3 = &certification.BlockCertificationRequest{
	InputRecord: IR3,
}

// Test internals
func Test_requestStore_add(t *testing.T) {
	rs := newRequestStore()
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.ErrorContains(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}), "duplicated request")
	require.ErrorContains(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR2}), "equivocating request with different hash")
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}))
	require.Equal(t, 2, len(rs.requests))
	require.Equal(t, 2, len(rs.hashCounts))
}

func Test_requestStore_isConsensusReceived(t *testing.T) {
	rs := newRequestStore()
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}))
	partitionInfo := partition_store.PartitionInfo{
		TrustBase: map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	_, possible := rs.isConsensusReceived(partitionInfo)
	require.False(t, possible)
}

func TestRequestStore_isConsensusReceived_SingleNode(t *testing.T) {
	rs := newRequestStore()
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	partitionInfo := partition_store.PartitionInfo{
		TrustBase: map[string]crypto.Verifier{"1": nil},
	}
	ir, possible := rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	rs := newRequestStore()
	partitionInfo := partition_store.PartitionInfo{
		TrustBase: map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR2}))
	_, possible := rs.isConsensusReceived(partitionInfo)
	require.False(t, possible)
	rs.reset()
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible := rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_FiveNodes(t *testing.T) {
	rs := newRequestStore()
	partitionInfo := partition_store.PartitionInfo{
		TrustBase: map[string]crypto.Verifier{"1": nil, "2": nil, "3": nil, "4": nil, "5": nil},
	}
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	ir, possible := rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Nil(t, ir)
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

}

func TestRequestStore_isConsensusNotPossible_FiveNodes(t *testing.T) {
	rs := newRequestStore()
	partitionInfo := partition_store.PartitionInfo{
		TrustBase: map[string]crypto.Verifier{"1": nil, "2": nil, "3": nil, "4": nil, "5": nil},
	}
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "1", InputRecord: IR1}))
	ir, possible := rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible = rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "3", InputRecord: IR2}))
	ir, possible = rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "4", InputRecord: IR2}))
	_, possible = rs.isConsensusReceived(partitionInfo)
	require.True(t, possible)

	// 50/50 split, but validator 5 sends a unique req
	require.NoError(t, rs.add(&certification.BlockCertificationRequest{NodeIdentifier: "5", InputRecord: IR3}))
	_, possible = rs.isConsensusReceived(partitionInfo)
	require.False(t, possible)
}

// Test interface
func Test_CertStore_add(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	partitionInfo := partition_store.PartitionInfo{
		SystemDescription: &genesis.SystemDescriptionRecord{SystemIdentifier: SysId1, T2Timeout: 2500},
		TrustBase:         map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "1", InputRecord: IR1}))
	require.ErrorContains(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "1", InputRecord: IR1}), "duplicated request")
	require.ErrorContains(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "1", InputRecord: IR2}), "equivocating request with different hash")
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "2", InputRecord: IR2}))
	_, possible := cs.IsConsensusReceived(partitionInfo)
	require.False(t, possible)
}

func TestCertRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	partitionInfo := partition_store.PartitionInfo{
		SystemDescription: &genesis.SystemDescriptionRecord{SystemIdentifier: SysId1, T2Timeout: 2500},
		TrustBase:         map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "2", InputRecord: IR2}))
	_, possible := cs.IsConsensusReceived(partitionInfo)
	require.False(t, possible)
	cs.Clear(p.SystemIdentifier(SysId1))
	// test all requests cleared
	require.Empty(t, cs.GetRequests(p.SystemIdentifier(SysId1)))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "2", InputRecord: IR1}))
	ir, possible := cs.IsConsensusReceived(partitionInfo)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestCertRequestStore_isConsensusReceived_MultipleSystemId(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	partInfo1 := partition_store.PartitionInfo{
		SystemDescription: &genesis.SystemDescriptionRecord{SystemIdentifier: SysId1, T2Timeout: 2500},
		TrustBase:         map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	partInfo2 := partition_store.PartitionInfo{
		SystemDescription: &genesis.SystemDescriptionRecord{SystemIdentifier: SysId2, T2Timeout: 2500},
		TrustBase:         map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId2, NodeIdentifier: "1", InputRecord: IR2}))
	_, possible := cs.IsConsensusReceived(partInfo1)
	require.True(t, possible)
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "2", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId2, NodeIdentifier: "2", InputRecord: IR2}))
	ir, possible := cs.IsConsensusReceived(partInfo1)
	require.True(t, possible)
	require.Equal(t, IR1, ir)
	require.Equal(t, len(cs.GetRequests(p.SystemIdentifier(SysId1))), 2)
	ir2, possible2 := cs.IsConsensusReceived(partInfo2)
	require.True(t, possible2)
	require.Equal(t, IR2, ir2)
	require.Equal(t, len(cs.GetRequests(p.SystemIdentifier(SysId2))), 2)
	// Reset resets both stores
	cs.Reset()
	// test all requests cleared
	require.Empty(t, cs.GetRequests(p.SystemIdentifier(SysId1)))
	require.Empty(t, cs.GetRequests(p.SystemIdentifier(SysId2)))
	ir3, possible3 := cs.IsConsensusReceived(partInfo1)
	require.True(t, possible3)
	require.Nil(t, ir3)
	ir4, possible4 := cs.IsConsensusReceived(partInfo2)
	require.True(t, possible4)
	require.Nil(t, ir4)
}

func TestCertRequestStore_clearOne(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	partInfo2 := partition_store.PartitionInfo{
		SystemDescription: &genesis.SystemDescriptionRecord{SystemIdentifier: SysId2, T2Timeout: 2500},
		TrustBase:         map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "1", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId2, NodeIdentifier: "1", InputRecord: IR2}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId1, NodeIdentifier: "2", InputRecord: IR1}))
	require.NoError(t, cs.Add(&certification.BlockCertificationRequest{SystemIdentifier: SysId2, NodeIdentifier: "2", InputRecord: IR2}))
	cs.Clear(p.SystemIdentifier(SysId1))
	require.Empty(t, cs.GetRequests(p.SystemIdentifier(SysId1)))
	ir2, possible2 := cs.IsConsensusReceived(partInfo2)
	require.True(t, possible2)
	require.Equal(t, IR2, ir2)
	require.Equal(t, len(cs.GetRequests(p.SystemIdentifier(SysId2))), 2)
}

func TestCertRequestStore_EmptyStore(t *testing.T) {
	cs := NewCertificationRequestBuffer()
	partInfo1 := partition_store.PartitionInfo{
		SystemDescription: &genesis.SystemDescriptionRecord{SystemIdentifier: SysId1, T2Timeout: 2500},
		TrustBase:         map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	partInfo2 := partition_store.PartitionInfo{
		SystemDescription: &genesis.SystemDescriptionRecord{SystemIdentifier: SysId2, T2Timeout: 2500},
		TrustBase:         map[string]crypto.Verifier{"1": nil, "2": nil},
	}
	require.Equal(t, len(cs.GetRequests(p.SystemIdentifier(SysId1))), 0)
	require.Equal(t, len(cs.GetRequests(p.SystemIdentifier(SysId2))), 0)
	// Reset resets both stores
	require.NotPanics(t, func() { cs.Reset() })
	require.NotPanics(t, func() { cs.Clear(p.SystemIdentifier(SysId1)) })
	require.NotPanics(t, func() { cs.Clear(p.SystemIdentifier([]byte{1, 1, 1, 1, 1, 1})) })
	ir3, possible3 := cs.IsConsensusReceived(partInfo1)
	require.True(t, possible3)
	require.Nil(t, ir3)
	ir4, possible4 := cs.IsConsensusReceived(partInfo2)
	require.True(t, possible4)
	require.Nil(t, ir4)
}
