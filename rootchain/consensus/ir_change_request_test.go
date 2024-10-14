package consensus

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/stretchr/testify/require"
)

func TestCheckBlockCertificationRequest(t *testing.T) {
	t.Run("request is nil", func(t *testing.T) {
		require.EqualError(t, CheckBlockCertificationRequest(nil, nil), "block certification request is nil")
	})
	t.Run("luc is nil", func(t *testing.T) {
		req := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{Version: 1,
				RoundNumber: 1,
			},
		}
		require.EqualError(t, CheckBlockCertificationRequest(req, nil), "unicity certificate is nil")
	})
	t.Run("invalid partition round", func(t *testing.T) {
		req := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{Version: 1,
				RoundNumber: 1,
			},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1,
				RoundNumber: 1,
			},
		}
		require.EqualError(t, CheckBlockCertificationRequest(req, luc), "invalid partition round number 1, last certified round number 1")
	})
	t.Run("hash mismatch", func(t *testing.T) {
		req := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{Version: 1,
				PreviousHash: []byte{0, 0, 0, 0},
				RoundNumber:  2,
			},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1,
				RoundNumber: 1,
				Hash:        []byte{1, 1, 1, 1},
			},
		}
		require.EqualError(t, CheckBlockCertificationRequest(req, luc), "request extends unknown state: expected hash: [1 1 1 1], got: [0 0 0 0]")
	})
	t.Run("root round does not match", func(t *testing.T) {
		req := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{Version: 1,
				PreviousHash: []byte{0, 0, 0, 0},
				RoundNumber:  2,
			},
			RootRoundNumber: 1,
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1,
				RoundNumber: 1,
				Hash:        []byte{0, 0, 0, 0},
			},
			UnicitySeal: &types.UnicitySeal{Version: 1, RootChainRoundNumber: 0},
		}
		require.EqualError(t, CheckBlockCertificationRequest(req, luc), "request root round number 1 does not match luc root round 0")
	})
	t.Run("ok", func(t *testing.T) {
		req := &certification.BlockCertificationRequest{
			InputRecord: &types.InputRecord{Version: 1,
				PreviousHash: []byte{0, 0, 0, 0},
				RoundNumber:  2,
			},
		}
		luc := &types.UnicityCertificate{Version: 1,
			InputRecord: &types.InputRecord{Version: 1,
				RoundNumber: 1,
				Hash:        []byte{0, 0, 0, 0},
			},
		}
		require.NoError(t, CheckBlockCertificationRequest(req, luc))
	})
}
