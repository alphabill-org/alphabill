package types

import (
	gocrypto "crypto"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
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

func TestNewQuorumCertificate(t *testing.T) {
	voteInfo := &RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 3}}
	commitInfo := &types.UnicitySeal{RootInternalInfo: []byte{0, 1, 2}}
	qc := NewQuorumCertificateFromVote(voteInfo, commitInfo, nil)
	require.NotNil(t, qc)
}

func TestQuorumCert_IsValid(t *testing.T) {
	type fields struct {
		VoteInfo         *RoundInfo
		LedgerCommitInfo *types.UnicitySeal
		Signatures       map[string][]byte
	}
	voteInfo := &RoundInfo{RoundNumber: 10, Epoch: 0, Timestamp: 1670314583523, ParentRoundNumber: 9, CurrentRootHash: test.RandomBytes(32)}
	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name: "QC not  valid - no vote info",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
				Signatures:       nil,
			},
			wantErrStr: errVoteInfoIsNil.Error(),
		},
		// Vote info valid unit tests are covered in VoteInfo tests
		{
			name: "QC not  valid - vote info no valid",
			fields: fields{
				VoteInfo:         &RoundInfo{RoundNumber: 2, Epoch: 0, Timestamp: 0},
				LedgerCommitInfo: nil,
				Signatures:       nil,
			},
			wantErrStr: "invalid vote info: parent round number is not assigned",
		},
		{
			name: "QC not  valid - ledger info is nil",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: nil,
				Signatures:       nil,
			},
			wantErrStr: errLedgerCommitInfoIsNil.Error(),
		},
		{
			name: "QC not  valid - ledger info is nil",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: nil, Hash: nil},
				Signatures:       nil,
			},
			wantErrStr: errInvalidRoundInfoHash.Error(),
		},
		{
			name: "QC not  valid - missing signatures",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
				Signatures:       nil,
			},
			wantErrStr: errQcIsMissingSignatures.Error(),
		},
		// Is valid, but would not verify
		{
			name: "QC valid - would not pass verify",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
				Signatures: map[string][]byte{
					"test1": {0, 1, 2},
					"test2": {1, 2, 3},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &QuorumCert{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				Signatures:       tt.fields.Signatures,
			}
			err := x.IsValid()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestQuorumCert_Verify(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}

	validQC := func() *QuorumCert {
		voteInfo := &RoundInfo{RoundNumber: 10, ParentRoundNumber: 9, Epoch: 0, Timestamp: 1670314583523, CurrentRootHash: test.RandomBytes(32)}
		commitInfo := &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)}
		cib := commitInfo.Bytes()
		sig1, err := s1.SignBytes(cib)
		require.NoError(t, err)
		sig2, err := s2.SignBytes(cib)
		require.NoError(t, err)
		sig3, err := s3.SignBytes(cib)
		require.NoError(t, err)
		qc := &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: commitInfo,
			Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
		}
		return qc
	}
	require.NoError(t, validQC().Verify(3, rootTrust), `validQC must return valid QC`)

	t.Run("IsValid is called", func(t *testing.T) {
		// trigger an error from IsValid to make sure it is called - so we do not need
		// to test for validity conditions here...
		qc := validQC()
		qc.VoteInfo = nil
		require.EqualError(t, qc.Verify(3, rootTrust), `invalid quorum certificate: vote info is nil`)
	})

	t.Run("invalid vote info hash", func(t *testing.T) {
		qc := validQC()

		// change vote info so hash should change
		qc.VoteInfo.Timestamp += 1
		require.EqualError(t, qc.Verify(3, rootTrust), `vote info hash verification failed`)

		// change stored hash
		qc.VoteInfo.Timestamp -= 1 // restore original value
		qc.LedgerCommitInfo.RootInternalInfo[0] = 0
		require.EqualError(t, qc.Verify(3, rootTrust), `vote info hash verification failed`)
	})

	t.Run("do we have enough signatures for quorum", func(t *testing.T) {
		qc := validQC()
		// we have enough signatures for quorum
		require.NoError(t, qc.Verify(2, rootTrust))
		require.NoError(t, qc.Verify(3, rootTrust))
		// require more signatures than QC has
		require.EqualError(t, qc.Verify(4, rootTrust), `quorum requires 4 signatures but certificate has 3`)
	})

	t.Run("unknown signer", func(t *testing.T) {
		qc := validQC()
		qc.Signatures["foobar"] = []byte{1, 2, 3}
		require.EqualError(t, qc.Verify(2, rootTrust), `signer "foobar" is not part of trustbase`)
	})

	t.Run("invalid signature", func(t *testing.T) {
		qc := validQC()
		qc.Signatures["2"] = []byte{1, 2, 3}
		require.ErrorContains(t, qc.Verify(2, rootTrust), `signer "2" signature is not valid:`)
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
