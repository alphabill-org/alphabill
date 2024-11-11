package storage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

func TestBlockKey(t *testing.T) {
	const round uint64 = 1
	require.Equal(t, []byte("block_\000\000\000\000\000\000\000\001"), blockKey(round))
}

func TestWriteReadLastVote(t *testing.T) {
	t.Run("error - store proposal", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		proposal := abdrc.ProposalMsg{}
		require.ErrorContains(t, WriteVote(db, proposal), "unknown vote type")
	})
	t.Run("read blank store", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		msg, err := ReadVote(db)
		require.NoError(t, err)
		require.Nil(t, msg)
	})
	t.Run("ok - store vote", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		vote := &abdrc.VoteMsg{Author: "test"}
		require.NoError(t, WriteVote(db, vote))
		// read back
		msg, err := ReadVote(db)
		require.NoError(t, err)
		require.IsType(t, &abdrc.VoteMsg{}, msg)
		require.Equal(t, "test", msg.(*abdrc.VoteMsg).Author)
	})
	t.Run("ok - store timeout vote", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		vote := &abdrc.TimeoutMsg{Timeout: &abtypes.Timeout{Round: 1}, Author: "test"}
		require.NoError(t, WriteVote(db, vote))
		// read back
		msg, err := ReadVote(db)
		require.NoError(t, err)
		require.IsType(t, &abdrc.TimeoutMsg{}, msg)
		require.Equal(t, "test", msg.(*abdrc.TimeoutMsg).Author)
		require.EqualValues(t, 1, msg.(*abdrc.TimeoutMsg).Timeout.Round)
	})
}
