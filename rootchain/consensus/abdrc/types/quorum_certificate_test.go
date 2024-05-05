package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type Option func(*RoundInfo)

func NewDummyVoteInfo(round uint64, options ...Option) *RoundInfo {
	voteInfo := &RoundInfo{RoundNumber: round, Epoch: 0,
		Timestamp: 1670314583523, ParentRoundNumber: round - 1, CurrentRootHash: []byte{0, 1, 3}}
	for _, o := range options {
		o(voteInfo)
	}
	return voteInfo
}

func TestQuorumCert_IsValid(t *testing.T) {
	sb := newStructBuilder(t, 2)

	t.Run("no vote info", func(t *testing.T) {
		qc := sb.QC(t, 12)
		qc.VoteInfo = nil
		require.ErrorIs(t, qc.IsValid(), errVoteInfoIsNil)
	})

	t.Run("invalid vote info", func(t *testing.T) {
		// check that voteInfo.IsValid is called
		qc := sb.QC(t, 12)
		qc.VoteInfo.RoundNumber = 0
		require.ErrorIs(t, qc.IsValid(), errRoundNumberUnassigned)
	})

	t.Run("ledger commit info is nil", func(t *testing.T) {
		qc := sb.QC(t, 12)
		qc.LedgerCommitInfo = nil
		require.ErrorIs(t, qc.IsValid(), errLedgerCommitInfoIsNil)
	})

	t.Run("ledger commit info is invalid", func(t *testing.T) {
		qc := sb.QC(t, 12)
		qc.LedgerCommitInfo.PreviousHash = nil
		require.ErrorIs(t, qc.IsValid(), errInvalidRoundInfoHash)
	})
}

func TestQuorumCert_Verify(t *testing.T) {
	sb := newStructBuilder(t, 3)
	rootTrust := sb.trustBase
	require.NoError(t, sb.QC(t, 10).Verify(rootTrust), `sb.QC must return valid QuorumCert struct`)

	t.Run("IsValid is called", func(t *testing.T) {
		// trigger an error from IsValid to make sure it is called - so we do not need
		// to test for validity conditions here...
		qc := sb.QC(t, 10)
		qc.VoteInfo = nil
		require.EqualError(t, qc.Verify(rootTrust), `invalid quorum certificate: vote info is nil`)
	})

	t.Run("invalid vote info hash", func(t *testing.T) {
		qc := sb.QC(t, 10)

		// change vote info so hash should change
		qc.VoteInfo.Timestamp += 1
		require.EqualError(t, qc.Verify(rootTrust), `vote info hash verification failed`)

		// change stored hash
		qc.VoteInfo.Timestamp -= 1 // restore original value
		qc.LedgerCommitInfo.PreviousHash[0] += 1
		require.EqualError(t, qc.Verify(rootTrust), `vote info hash verification failed`)
	})

	t.Run("do we have enough signatures for quorum", func(t *testing.T) {
		qc := sb.QC(t, 10)
		// we have enough signatures for quorum
		require.NoError(t, qc.Verify(rootTrust))
		// require more signatures than QC has
		for k := range qc.Signatures {
			delete(qc.Signatures, k)
			break
		}
		require.EqualError(t, qc.Verify(rootTrust), `failed to verify quorum signatures: quorum not reached, signed_votes=2 quorum_threshold=3`)
	})

	t.Run("invalid signature", func(t *testing.T) {
		qc := sb.QC(t, 10)
		// replace one (valid) signature with invalid one
		var signerID string
		for k := range qc.Signatures {
			signerID = k
			qc.Signatures[signerID] = []byte{1, 2, 3}
			break
		}
		// the invalid signature must trigger error
		require.ErrorContains(t, qc.Verify(rootTrust), "failed to verify quorum signatures: quorum not reached, signed_votes=2 quorum_threshold=3")
	})
}

func TestQuorumCert_GetRound(t *testing.T) {
	var qc *QuorumCert = nil
	require.Equal(t, uint64(0), qc.GetRound())
	qc = &QuorumCert{}
	require.Equal(t, uint64(0), qc.GetRound())
	qc = &QuorumCert{VoteInfo: &RoundInfo{RoundNumber: 2}}
	require.Equal(t, uint64(2), qc.GetRound())
}

func TestQuorumCert_GetParentRound(t *testing.T) {
	var qc *QuorumCert = nil
	require.Equal(t, uint64(0), qc.GetParentRound())
	qc = &QuorumCert{}
	require.Equal(t, uint64(0), qc.GetParentRound())
	qc = &QuorumCert{VoteInfo: &RoundInfo{ParentRoundNumber: 2}}
	require.Equal(t, uint64(2), qc.GetParentRound())
}
