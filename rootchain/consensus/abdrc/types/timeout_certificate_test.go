package types

import (
	"bytes"
	gocrypto "crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

func calcTimeoutSig(t *testing.T, s abcrypto.Signer, round, epoch, hQcRound uint64, author string) []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(round))
	b.Write(util.Uint64ToBytes(epoch))
	b.Write(util.Uint64ToBytes(hQcRound))
	b.Write([]byte(author))
	sig, err := s.SignBytes(b.Bytes())
	require.NoError(t, err)
	return sig
}

func TestTimeoutCert_Add(t *testing.T) {
	// create partial timeout certificate
	voteInfo := &RoundInfo{
		RoundNumber:       8,
		ParentRoundNumber: 7,
		Epoch:             0,
		Timestamp:         1670314583523,
		CurrentRootHash:   test.RandomBytes(32)}
	timeoutCert := &TimeoutCert{
		Timeout: &Timeout{
			Epoch: 0,
			Round: 10,
			HighQc: &QuorumCert{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &types.UnicitySeal{PreviousHash: voteInfo.Hash(gocrypto.SHA256)},
				Signatures:       map[string][]byte{"1": {1, 2, 1}},
			},
		},
		Signatures: make(map[string]*TimeoutVote),
	}
	t1 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: &types.UnicitySeal{PreviousHash: voteInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	err := timeoutCert.Add("1", t1, []byte{0, 1, 2})
	require.NoError(t, err)
	require.Equal(t, []string{"1"}, timeoutCert.GetAuthors())
	require.Equal(t, timeoutCert.Timeout.HighQc.VoteInfo.RoundNumber, voteInfo.RoundNumber)
	// Add a new timeout vote, but with lower round
	voteInfo = &RoundInfo{
		RoundNumber:       7,
		ParentRoundNumber: 6,
		Epoch:             0,
		Timestamp:         1670314583523,
		CurrentRootHash:   test.RandomBytes(32)}
	t2 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: &types.UnicitySeal{PreviousHash: voteInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	err = timeoutCert.Add("2", t2, []byte{1, 2, 2})
	require.NoError(t, err)
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Equal(t, uint64(8), timeoutCert.Timeout.HighQc.VoteInfo.RoundNumber)
	// Add a third vote, but with higher QC round so QC in the certificate gets updated
	voteInfo = &RoundInfo{
		RoundNumber:       9,
		ParentRoundNumber: 8,
		Epoch:             0,
		Timestamp:         1670314583523,
		CurrentRootHash:   test.RandomBytes(32)}
	t3 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: &types.UnicitySeal{PreviousHash: voteInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	err = timeoutCert.Add("3", t3, []byte{1, 2, 2})
	require.NoError(t, err)
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Contains(t, timeoutCert.GetAuthors(), "3")
	require.Equal(t, uint64(9), timeoutCert.Timeout.HighQc.VoteInfo.RoundNumber)
	// duplicate
	err = timeoutCert.Add("3", t3, []byte{1, 2, 3})
	require.ErrorContains(t, err, "already voted")

	// attempt to add vote from wrong round
	t4 := &Timeout{
		Epoch: 0,
		Round: timeoutCert.Timeout.Round + 1,
		HighQc: &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: &types.UnicitySeal{PreviousHash: voteInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	err = timeoutCert.Add("4", t4, []byte{1, 2, 3})
	require.EqualError(t, err, `TC is for round 10 not 11`)
	require.NotContains(t, timeoutCert.GetAuthors(), "4")
	require.EqualValues(t, 9, timeoutCert.Timeout.HighQc.VoteInfo.RoundNumber)
}

func TestTimeoutCert_IsValid(t *testing.T) {
	sb := newStructBuilder(t, 2)

	t.Run("timeout data is nil", func(t *testing.T) {
		tc := sb.TimeoutCert(t)
		tc.Timeout = nil
		require.EqualError(t, tc.IsValid(), `timeout data is unassigned`)
	})
}

func TestTimeoutCert_Verify(t *testing.T) {
	sb := newStructBuilder(t, 3)
	trustBase := sb.Verifiers()

	t.Run("IsValid is called", func(t *testing.T) {
		// trigger error from IsValid to make sure it is called
		tc := sb.TimeoutCert(t)
		tc.Timeout = nil
		err := tc.Verify(3, trustBase)
		require.EqualError(t, err, `invalid certificate: timeout data is unassigned`)
	})

	t.Run("timeout.Verify is called", func(t *testing.T) {
		// trigger Timeout verification error to make sure it is called
		tc := sb.TimeoutCert(t)
		tc.Timeout.HighQc.Signatures["foobar"] = []byte{1, 2, 3, 4}
		err := tc.Verify(3, trustBase)
		require.EqualError(t, err, `invalid timeout data: invalid high QC: signer "foobar" is not part of trustbase`)
	})

	t.Run("no quorum", func(t *testing.T) {
		tc := sb.TimeoutCert(t)
		for k := range tc.Signatures {
			delete(tc.Signatures, k)
			break
		}
		err := tc.Verify(uint32(len(tc.Signatures)+1), trustBase)
		require.EqualError(t, err, `quorum requires 3 signatures but certificate has 2`)
	})

	t.Run("invalid signature", func(t *testing.T) {
		tc := sb.TimeoutCert(t)
		for _, v := range tc.Signatures {
			// high QC round is part of signature so changing it should invalidate the signature
			v.HqcRound++ // works because v is a pointer pointing to the same item stored to map
			break
		}
		err := tc.Verify(3, trustBase)
		require.ErrorContains(t, err, `timeout certificate signature verification failed: verification failed`)
	})

	t.Run("unknown signer", func(t *testing.T) {
		tc := sb.TimeoutCert(t)
		tc.Signatures["foobar"] = &TimeoutVote{}
		err := tc.Verify(3, trustBase)
		require.EqualError(t, err, `signer "foobar" is not part of trustbase`)
	})

	t.Run("high QC round doesn't match max round", func(t *testing.T) {
		tc := sb.TimeoutCert(t)
		// replace one signature with a valid signature for higher round
		for k := range tc.Signatures {
			hqcR := tc.Timeout.HighQc.VoteInfo.RoundNumber + 1
			tc.Signatures[k] = &TimeoutVote{
				HqcRound:  hqcR,
				Signature: calcTimeoutSig(t, sb.signers[k], tc.Timeout.Round, 0, hqcR, k),
			}
			break
		}
		err := tc.Verify(3, trustBase)
		require.EqualError(t, err, `high QC round 10 does not match max signed QC round 11`)
	})
}

func TestTimeoutCert_GetRound(t *testing.T) {
	var tc *TimeoutCert = nil
	require.Equal(t, uint64(0), tc.GetRound())
	tc = &TimeoutCert{Timeout: nil}
	require.Equal(t, uint64(0), tc.GetRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{Round: 10},
	}
	require.Equal(t, uint64(10), tc.GetRound())
}

func TestTimeoutCert_GetHqcRound(t *testing.T) {
	var tc *TimeoutCert = nil
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{Timeout: nil}
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{
			Round:  10,
			HighQc: nil,
		},
	}
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{
			Round:  10,
			HighQc: &QuorumCert{},
		},
	}
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{
			Round:  10,
			HighQc: &QuorumCert{VoteInfo: &RoundInfo{RoundNumber: 9}},
		},
	}
	require.Equal(t, uint64(9), tc.GetHqcRound())
}

func Test_Timeout_IsValid(t *testing.T) {
	sb := newStructBuilder(t, 3)
	require.NoError(t, sb.Timeout(t, nil).IsValid(), `sb.Timeout must return valid Timeout struct`)

	t.Run("high QC unassigned", func(t *testing.T) {
		timeout := sb.Timeout(t, nil)
		timeout.HighQc = nil
		require.EqualError(t, timeout.IsValid(), "high QC is unassigned")
	})

	t.Run("high QC invalid", func(t *testing.T) {
		// basically check that HighQC.IsValid is called
		timeout := sb.Timeout(t, nil)
		timeout.HighQc.VoteInfo = nil
		require.EqualError(t, timeout.IsValid(), "invalid high QC: vote info is nil")
	})

	t.Run("invalid round", func(t *testing.T) {
		// Round must be greater than highQC.Round
		timeout := sb.Timeout(t, nil)

		timeout.Round = 0
		require.EqualError(t, timeout.IsValid(), "timeout round (0) must be greater than high QC round (10)")

		timeout.Round = timeout.HighQc.GetRound() - 1
		require.EqualError(t, timeout.IsValid(), "timeout round (9) must be greater than high QC round (10)")

		timeout.Round = timeout.HighQc.GetRound()
		require.EqualError(t, timeout.IsValid(), "timeout round (10) must be greater than high QC round (10)")
	})

	t.Run("last TC is unassigned", func(t *testing.T) {
		// if highQC is not for the previous round then lastTC must be
		timeout := sb.Timeout(t, nil)
		timeout.Round = timeout.HighQc.GetRound() + 2 // normally TO.Round==HighQC.Round+1
		require.EqualError(t, timeout.IsValid(), "last TC is missing")
	})

	t.Run("last TC is for wrong round", func(t *testing.T) {
		tc := sb.TimeoutCert(t)
		timeout := sb.Timeout(t, tc)
		timeout.Round = tc.GetRound() + 2
		require.EqualError(t, timeout.IsValid(), "last TC must be for round 12 but is for round 11")
	})
}

func Test_Timeout_Verify(t *testing.T) {
	sb := newStructBuilder(t, 3)
	rootTrust := sb.Verifiers()
	require.NoError(t, sb.Timeout(t, nil).Verify(3, rootTrust), `sb.Timeout must return valid Timeout struct`)

	t.Run("IsValid is called", func(t *testing.T) {
		timeout := sb.Timeout(t, nil)
		timeout.HighQc = nil
		require.EqualError(t, timeout.Verify(2, rootTrust), `invalid timeout data: high QC is unassigned`)
	})

	t.Run("invalid highQC", func(t *testing.T) {
		// check that highQC.Verify is called
		timeout := sb.Timeout(t, nil)
		// call with higher quorum than the signature count
		require.EqualError(t, timeout.Verify(uint32(len(rootTrust)+1), rootTrust), `invalid high QC: quorum requires 4 signatures but certificate has 3`)
	})

	t.Run("invalid lastTC", func(t *testing.T) {
		// check that lastTC.Verify is called
		tc := sb.TimeoutCert(t)
		timeout := sb.Timeout(t, tc)
		tc.Timeout.Epoch += 1 // epoch is part of signature so changing it should make it invalid
		require.ErrorContains(t, timeout.Verify(3, rootTrust), `invalid last TC: timeout certificate signature verification failed: verification failed`)
	})
}

func TestBytesFromTimeoutVote(t *testing.T) {
	timeout := &Timeout{
		Round: 10,
		Epoch: 0,
		HighQc: &QuorumCert{
			VoteInfo: &RoundInfo{
				RoundNumber:       9,
				Epoch:             0,
				Timestamp:         0x0010670314583523,
				ParentRoundNumber: 8,
				CurrentRootHash:   []byte{0, 1, 3}},
		},
	}
	// Require serialization is equal
	b := BytesFromTimeoutVote(timeout, "test", &TimeoutVote{HqcRound: 9, Signature: []byte{1, 2, 3}})
	require.NotNil(t, b)
	require.Len(t, b, 28)
}
