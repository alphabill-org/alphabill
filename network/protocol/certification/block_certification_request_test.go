package certification

import (
	"testing"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestBlockCertificationRequest_IsValid_BlockCertificationRequestIsNil(t *testing.T) {
	var p1 *BlockCertificationRequest
	require.ErrorIs(t, p1.IsValid(nil), ErrBlockCertificationRequestIsNil)
}

func TestBlockCertificationRequest_IsValid_VerifierIsNil(t *testing.T) {
	p1 := &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "1",
		InputRecord:      &types.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(nil), errVerifierIsNil)
}

func TestBlockCertificationRequest_IsValid_InvalidSystemIdentifier(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: 0,
		NodeIdentifier:   "11",
		InputRecord:      &types.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(verifier), errInvalidSystemIdentifier)
}

func TestBlockCertificationRequest_IsValid_EmptyNodeIdentifier(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "",
		InputRecord:      &types.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(verifier), errEmptyNodeIdentifier)
}

func TestBlockCertificationRequest_IsValid_InvalidInputRecord(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "1",
		InputRecord:      nil,
	}
	require.ErrorContains(t, p1.IsValid(verifier), types.ErrInputRecordIsNil.Error())
}

func TestBlockCertificationRequest_IsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "1",
		InputRecord: &types.InputRecord{
			PreviousHash: []byte{},
			Hash:         []byte{},
			BlockHash:    []byte{},
			SummaryValue: []byte{},
			RoundNumber:  1,
		},
		Signature: make([]byte, 64),
	}
	err := p1.IsValid(verifier)
	require.Error(t, err)
	require.ErrorContains(t, err, "signature verification failed")
}

func TestBlockCertificationRequest_ValidRequest(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "1",
		InputRecord: &types.InputRecord{
			PreviousHash: []byte{},
			Hash:         []byte{},
			BlockHash:    []byte{},
			SummaryValue: []byte{},
			RoundNumber:  1,
		},
		RootRoundNumber: 1,
	}
	err := p1.Sign(signer)
	require.NoError(t, err)
	err = p1.IsValid(verifier)
	require.NoError(t, err)
}

func TestBlockCertificationRequest_GetPreviousHash(t *testing.T) {
	var req *BlockCertificationRequest = nil
	require.Nil(t, req.IRPreviousHash())
	req = &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "1",
		InputRecord:      nil,
	}
	require.Nil(t, req.IRPreviousHash())
	req.InputRecord = &types.InputRecord{PreviousHash: []byte{1, 2, 3}}
	require.Equal(t, []byte{1, 2, 3}, req.IRPreviousHash())
}

func TestBlockCertificationRequest_GetIRRound(t *testing.T) {
	var req *BlockCertificationRequest = nil
	require.EqualValues(t, 0, req.IRRound())
	req = &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "1",
		InputRecord:      nil,
	}
	require.EqualValues(t, 0, req.IRRound())
	req.InputRecord = &types.InputRecord{RoundNumber: 10}
	require.EqualValues(t, 10, req.IRRound())
}

func TestBlockCertificationRequest_RootRound(t *testing.T) {
	var req *BlockCertificationRequest = nil
	require.EqualValues(t, 0, req.RootRound())
	req = &BlockCertificationRequest{
		SystemIdentifier: 1,
		NodeIdentifier:   "1",
		RootRoundNumber:  11,
	}
	require.EqualValues(t, 11, req.RootRound())
}
