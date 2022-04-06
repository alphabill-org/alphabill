package rootchain

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
)

var req1 = &p1.RequestEvent{
	Req: &p1.P1Request{
		InputRecord: &certificates.InputRecord{Hash: []byte{1}},
	},
}

var req2 = &p1.RequestEvent{
	Req: &p1.P1Request{
		InputRecord: &certificates.InputRecord{Hash: []byte{2}},
	},
}

func Test_requestStore_add(t *testing.T) {
	rs := newRequestStore()
	rs.add("1", req1)
	rs.add("1", req1)
	rs.add("2", req2)
	require.Equal(t, 2, len(rs.requests))
	require.Equal(t, 2, len(rs.hashCounts))
}

func Test_requestStore_isConsensusReceived(t *testing.T) {
	rs := newRequestStore()
	rs.add("1", req1)
	rs.add("2", req2)
	_, possible := rs.isConsensusReceived(2)
	require.False(t, possible)
}

func TestRequestStore_isConsensusReceived_SingleNode(t *testing.T) {
	rs := newRequestStore()
	rs.add("1", req1)
	ir, possible := rs.isConsensusReceived(1)
	require.True(t, possible)
	require.Equal(t, req1.Req.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_TwoNodes(t *testing.T) {
	rs := newRequestStore()
	rs.add("1", req1)
	rs.add("2", req2)
	_, possible := rs.isConsensusReceived(2)
	require.False(t, possible)

	rs.add("2", req1)
	ir, possible := rs.isConsensusReceived(2)
	require.True(t, possible)
	require.Equal(t, req1.Req.InputRecord, ir)
}

func TestRequestStore_isConsensusReceived_FiveNodes(t *testing.T) {
	rs := newRequestStore()
	rs.add("1", req1)
	ir, possible := rs.isConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)

	rs.add("2", req1)
	ir, possible = rs.isConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)

	rs.add("3", req1)
	ir, possible = rs.isConsensusReceived(5)
	require.True(t, possible)
	require.Equal(t, req1.Req.InputRecord, ir)

	rs.add("3", req2)
	ir, possible = rs.isConsensusReceived(5)
	require.True(t, possible)
	require.Nil(t, ir)

	rs.add("4", req2)
	_, possible = rs.isConsensusReceived(5)
	require.False(t, false)
}
