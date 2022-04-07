package p1

import (
	"strings"
	"testing"

	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"github.com/stretchr/testify/require"
)

func TestP1Request_IsValid_P1IsNil(t *testing.T) {
	var p1 *P1Request
	require.ErrorIs(t, p1.IsValid(nil), ErrP1RequestIsNil)
}

func TestP1Request_IsValid_VerifierIsNil(t *testing.T) {
	p1 := &P1Request{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord:      &certificates.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(nil), ErrVerifierIsNil)
}

func TestP1Request_IsValid_InvalidSystemIdentifier(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &P1Request{
		SystemIdentifier: []byte{0},
		NodeIdentifier:   "11",
		RootRoundNumber:  0,
		InputRecord:      &certificates.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(verifier), ErrInvalidSystemIdentifierLength)
}

func TestP1Request_IsValid_EmptyNodeIdentifier(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &P1Request{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "",
		RootRoundNumber:  0,
		InputRecord:      &certificates.InputRecord{},
	}
	require.ErrorIs(t, p1.IsValid(verifier), ErrEmptyNodeIdentifier)
}

func TestP1Request_IsValid_InvalidInputRecord(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &P1Request{
		SystemIdentifier: []byte{0, 0, 0, 0},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord:      nil,
	}
	require.ErrorIs(t, p1.IsValid(verifier), certificates.ErrInputRecordIsNil)
}

func TestP1Request_IsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &P1Request{
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
	require.True(t, strings.Contains(err.Error(), "signature verify failed"))
}

func TestP1Request_ValidRequest(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	p1 := &P1Request{
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
