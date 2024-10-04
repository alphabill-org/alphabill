package certification

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestBlockCertificationRequest_IsValid_BlockCertificationRequestIsNil(t *testing.T) {
	var p1 *BlockCertificationRequest
	require.ErrorIs(t, p1.IsValid(nil), ErrBlockCertificationRequestIsNil)
}

func Test_BlockCertificationRequest_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	validBCR := func(t *testing.T) *BlockCertificationRequest {
		bcr := &BlockCertificationRequest{
			Partition:      1,
			NodeIdentifier: "1",
			Leader:         "1",
			InputRecord: &types.InputRecord{
				PreviousHash: []byte{},
				Hash:         []byte{},
				BlockHash:    []byte{},
				SummaryValue: []byte{},
				RoundNumber:  1,
			},
			RootRoundNumber: 1,
		}

		require.NoError(t, bcr.Sign(signer))
		return bcr
	}

	t.Run("valid", func(t *testing.T) {
		bcr := validBCR(t)
		require.NoError(t, bcr.IsValid(verifier))
	})

	t.Run("verifier is nil", func(t *testing.T) {
		bcr := validBCR(t)
		require.ErrorIs(t, bcr.IsValid(nil), errVerifierIsNil)
	})

	t.Run("invalid partition ID", func(t *testing.T) {
		bcr := validBCR(t)
		bcr.Partition = 0
		require.ErrorIs(t, bcr.IsValid(verifier), errInvalidSystemIdentifier)
	})

	t.Run("invalid node ID", func(t *testing.T) {
		bcr := validBCR(t)
		bcr.NodeIdentifier = ""
		require.ErrorIs(t, bcr.IsValid(verifier), errEmptyNodeIdentifier)
	})

	t.Run("invalid IR", func(t *testing.T) {
		bcr := validBCR(t)
		bcr.InputRecord = nil
		require.EqualError(t, bcr.IsValid(verifier), `input record error: input record is nil`)
	})

	t.Run("invalid signature", func(t *testing.T) {
		bcr := validBCR(t)
		bcr.Signature = make([]byte, 64)
		require.EqualError(t, bcr.IsValid(verifier), `signature verification failed`)
	})
}

func TestBlockCertificationRequest_GetPreviousHash(t *testing.T) {
	var req *BlockCertificationRequest = nil
	require.Nil(t, req.IRPreviousHash())
	req = &BlockCertificationRequest{
		Partition:      1,
		NodeIdentifier: "1",
		InputRecord:    nil,
	}
	require.Nil(t, req.IRPreviousHash())
	req.InputRecord = &types.InputRecord{PreviousHash: []byte{1, 2, 3}}
	require.Equal(t, []byte{1, 2, 3}, req.IRPreviousHash())
}

func TestBlockCertificationRequest_GetIRRound(t *testing.T) {
	var req *BlockCertificationRequest = nil
	require.EqualValues(t, 0, req.IRRound())
	req = &BlockCertificationRequest{
		Partition:      1,
		NodeIdentifier: "1",
		InputRecord:    nil,
	}
	require.EqualValues(t, 0, req.IRRound())
	req.InputRecord = &types.InputRecord{RoundNumber: 10}
	require.EqualValues(t, 10, req.IRRound())
}

func TestBlockCertificationRequest_RootRound(t *testing.T) {
	var req *BlockCertificationRequest = nil
	require.EqualValues(t, 0, req.RootRound())
	req = &BlockCertificationRequest{
		Partition:       1,
		NodeIdentifier:  "1",
		RootRoundNumber: 11,
	}
	require.EqualValues(t, 11, req.RootRound())
}
