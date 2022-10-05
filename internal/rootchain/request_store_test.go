package rootchain

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/stretchr/testify/require"
)

var req1 = &certification.BlockCertificationRequest{
	InputRecord: &certificates.InputRecord{Hash: []byte{1}},
}

var req2 = &certification.BlockCertificationRequest{
	InputRecord: &certificates.InputRecord{Hash: []byte{2}},
}

var req3 = &certification.BlockCertificationRequest{
	InputRecord: &certificates.InputRecord{Hash: []byte{3}},
}

func Test_requestStore_add(t *testing.T) {
	rs := NewRequestStore("")
	require.NoError(t, rs.Add("1", req1))
	require.ErrorContains(t, rs.Add("1", req1), "duplicated request")
	require.ErrorContains(t, rs.Add("1", req2), "equivocating request with different hash")
	require.NoError(t, rs.Add("2", req2))
	require.Equal(t, 2, len(rs.requests))
	require.Equal(t, 2, len(rs.hashCounts))
}

func Test_requestStore_isConsensusReceived(t *testing.T) {
	rs := NewRequestStore("")
	require.NoError(t, rs.Add("1", req1))
	require.NoError(t, rs.Add("2", req2))
	_, possible := rs.IsConsensusReceived(2)
	require.False(t, possible)
}

func TestRequestStore_isConsensusReceived_SingleNode(t *testing.T) {
	rs := NewRequestStore("")
	require.NoError(t, rs.Add("1", req1))
	ir, possible := rs.IsConsensusReceived(1)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	rs := NewRequestStore("")
	require.NoError(t, rs.Add("1", req1))
	require.NoError(t, rs.Add("2", req2))
	_, possible := rs.IsConsensusReceived(2)
	require.False(t, possible)
	rs.Reset()
	require.NoError(t, rs.Add("1", req1))
	require.NoError(t, rs.Add("2", req1))
	ir, possible := rs.IsConsensusReceived(2)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_FiveNodes(t *testing.T) {
	rs := NewRequestStore("")
	require.NoError(t, rs.Add("1", req1))
	ir, possible := rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.Add("2", req1))
	ir, possible = rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)
	require.NoError(t, rs.Add("3", req1))
	ir, possible = rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

	require.NoError(t, rs.Add("4", req1))
	ir, possible = rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

	require.NoError(t, rs.Add("5", req1))
	ir, possible = rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Equal(t, req1.InputRecord, ir)

}

func TestRequestStore_isConsensusNotPossible_FiveNodes(t *testing.T) {
	rs := NewRequestStore("")
	require.NoError(t, rs.Add("1", req1))
	ir, possible := rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.Add("2", req1))
	ir, possible = rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.Add("3", req2))
	ir, possible = rs.IsConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)

	require.NoError(t, rs.Add("4", req2))
	_, possible = rs.IsConsensusReceived(5)
	require.True(t, possible)

	// 50/50 split, but validator 5 sends a unique req
	require.NoError(t, rs.Add("5", req3))
	_, possible = rs.IsConsensusReceived(5)
	require.False(t, false)
}
