package certification

import (
	"testing"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"

	"github.com/alphabill-org/alphabill/internal/certificates"

	"github.com/stretchr/testify/require"
)

func TestBlockCertificationRequest_IsValid_BlockCertificationRequestIsNil(t *testing.T) {
	var p1 *BlockCertificationRequest
	require.ErrorIs(t, p1.IsValid(nil), ErrBlockCertificationRequestIsNil)
}

func TestBlockCertificationRequest_IsValid_VerifierIsNil(t *testing.T) {
	p1 := &BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord:      &certificates.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(nil), ErrVerifierIsNil)
}

func TestBlockCertificationRequest_IsValid_InvalidSystemIdentifier(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: []byte{0},
		NodeIdentifier:   "11",
		RootRoundNumber:  0,
		InputRecord:      &certificates.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(verifier), ErrInvalidSystemIdentifierLength)
}

func TestBlockCertificationRequest_IsValid_EmptyNodeIdentifier(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "",
		RootRoundNumber:  0,
		InputRecord:      &certificates.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(verifier), ErrEmptyNodeIdentifier)
}

func TestBlockCertificationRequest_IsValid_InvalidInputRecord(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord:      nil,
	}
	require.ErrorIs(t, p1.IsValid(verifier), certificates.ErrInputRecordIsNil)
}

func TestBlockCertificationRequest_IsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{},
			Hash:         []byte{},
			BlockHash:    []byte{},
			SummaryValue: []byte{},
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
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{},
			Hash:         []byte{},
			BlockHash:    []byte{},
			SummaryValue: []byte{},
		},
	}
	err := p1.Sign(signer)
	require.NoError(t, err)
	err = p1.IsValid(verifier)
	require.NoError(t, err)
}
