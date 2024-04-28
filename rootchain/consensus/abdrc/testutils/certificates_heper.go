package testutils

import (
	"bytes"
	gocrypto "crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	"github.com/stretchr/testify/require"
)

func CalcTimeoutSig(t *testing.T, s abcrypto.Signer, round, epoch, hQcRound uint64, author string) []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(round))
	b.Write(util.Uint64ToBytes(epoch))
	b.Write(util.Uint64ToBytes(hQcRound))
	b.Write([]byte(author))
	sig, err := s.SignBytes(b.Bytes())
	require.NoError(t, err)
	return sig
}

func NewDummyCommitInfo(algo gocrypto.Hash, voteInfo *abtypes.RoundInfo) *types.UnicitySeal {
	hash := voteInfo.Hash(algo)
	return &types.UnicitySeal{PreviousHash: hash, Hash: nil}
}

type RoundInfoOption func(info *abtypes.RoundInfo)

func WithParentRound(round uint64) RoundInfoOption {
	return func(info *abtypes.RoundInfo) {
		info.ParentRoundNumber = round
	}
}

func WithTimestamp(time uint64) RoundInfoOption {
	return func(info *abtypes.RoundInfo) {
		info.Timestamp = time
	}
}

func NewDummyRootRoundInfo(round uint64, options ...RoundInfoOption) *abtypes.RoundInfo {
	voteInfo := &abtypes.RoundInfo{RoundNumber: round, Epoch: 0,
		Timestamp: 1670314583523, ParentRoundNumber: round - 1, CurrentRootHash: []byte{0, 1, 3}}
	for _, o := range options {
		o(voteInfo)
	}
	return voteInfo
}
