package types

import (
	"fmt"
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
	rootTrust := sb.Verifiers()
	require.NoError(t, sb.QC(t, 10).Verify(3, rootTrust), `sb.QC must return valid QuorumCert struct`)

	t.Run("IsValid is called", func(t *testing.T) {
		// trigger an error from IsValid to make sure it is called - so we do not need
		// to test for validity conditions here...
		qc := sb.QC(t, 10)
		qc.VoteInfo = nil
		require.EqualError(t, qc.Verify(3, rootTrust), `invalid quorum certificate: vote info is nil`)
	})

	t.Run("invalid vote info hash", func(t *testing.T) {
		qc := sb.QC(t, 10)

		// change vote info so hash should change
		qc.VoteInfo.Timestamp += 1
		require.EqualError(t, qc.Verify(3, rootTrust), `vote info hash verification failed`)

		// change stored hash
		qc.VoteInfo.Timestamp -= 1 // restore original value
		qc.LedgerCommitInfo.PreviousHash = []byte{1, 2, 3, 4}
		require.EqualError(t, qc.Verify(3, rootTrust), `vote info hash verification failed`)
	})

	t.Run("do we have enough signatures for quorum", func(t *testing.T) {
		qc := sb.QC(t, 10)
		// we have enough signatures for quorum
		require.NoError(t, qc.Verify(2, rootTrust))
		require.NoError(t, qc.Verify(3, rootTrust))
		// require more signatures than QC has
		require.EqualError(t, qc.Verify(4, rootTrust), `quorum requires 4 signatures but certificate has 3`)
	})

	t.Run("unknown signer", func(t *testing.T) {
		qc := sb.QC(t, 10)
		qc.Signatures["foobar"] = []byte{1, 2, 3}
		require.EqualError(t, qc.Verify(2, rootTrust), `signer "foobar" is not part of trustbase`)
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
		// require one less signature than qc has but the invalid signature must still trigger error!
		require.ErrorContains(t, qc.Verify(uint32(len(qc.Signatures)-1), rootTrust), fmt.Sprintf("signer %q signature is not valid:", signerID))
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
