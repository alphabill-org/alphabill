package types

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

/*
structBuilder is helper to build *valid* data structures for tests.
Generally fields are filled with random data but the data struct should
succeed IsValid and Verify checks.
*/
type structBuilder struct {
	verifiers map[string]abcrypto.Verifier
	signers   map[string]abcrypto.Signer
	trustBase *types.RootTrustBaseV0
}

func newStructBuilder(t *testing.T, peerCnt int) *structBuilder {
	t.Helper()

	sb := &structBuilder{
		verifiers: map[string]abcrypto.Verifier{},
		signers:   map[string]abcrypto.Signer{},
		trustBase: &types.RootTrustBaseV0{},
	}

	var nodes []*types.NodeInfo
	for i := 0; i < peerCnt; i++ {
		signer, err := abcrypto.NewInMemorySecp256K1Signer()
		require.NoError(t, err)

		verifier, err := signer.Verifier()
		require.NoError(t, err)

		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		pub, err := p2pcrypto.UnmarshalSecp256k1PublicKey(pubKey)
		require.NoError(t, err)
		id, err := peer.IDFromPublicKey(pub)
		require.NoError(t, err)

		nodeID := id.String()
		sb.signers[nodeID] = signer
		sb.verifiers[nodeID] = verifier
		nodes = append(nodes, types.NewNodeInfo(nodeID, 1, sb.verifiers[nodeID]))
	}

	tb, err := types.NewTrustBaseGenesis(nodes, []byte{1})
	if err != nil {
		require.NoError(t, err)
	}
	sb.trustBase = tb

	return sb
}

/*
Verifiers returns map of Verifier-s which is usable as trust base for data structs created by the builder.
NB! returned map should be treated as read only!
*/
func (sb structBuilder) Verifiers() map[string]abcrypto.Verifier {
	return sb.verifiers
}

/*
RandomPeerID returns random peer ID from trust base.
*/
func (sb structBuilder) RandomPeerID(t *testing.T) string {
	for k := range sb.verifiers {
		return k
	}

	t.Fatal("it appears that the verifiers map is empty")
	return ""
}

/*
QC returns valid QC (with random data) for round "round"
*/
func (sb structBuilder) QC(t *testing.T, round uint64) *QuorumCert {
	voteInfo := &RoundInfo{RoundNumber: round, ParentRoundNumber: round - 1, Epoch: 0, Timestamp: 1670314583523, CurrentRootHash: test.RandomBytes(32)}
	commitInfo := types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
		seal.PreviousHash = voteInfo.Hash(crypto.SHA256)
	})
	qc := &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{},
	}

	cib := commitInfo.Bytes()
	for k, v := range sb.signers {
		sig, err := v.SignBytes(cib)
		require.NoError(t, err)
		qc.Signatures[k] = sig
	}
	return qc
}

/*
"lastTC" may be nil (ie previous round was not a timeout round)
*/
func (sb structBuilder) Timeout(t *testing.T, lastTC *TimeoutCert) *Timeout {
	var round uint64 = 11
	qcRound := round - 1
	if lastTC != nil {
		round = lastTC.GetRound() + 1
		qcRound = lastTC.GetRound() - 1
	}

	return &Timeout{
		Epoch:  0,
		Round:  round,
		HighQc: sb.QC(t, qcRound),
		LastTC: lastTC,
	}
}

func (sb structBuilder) TimeoutCert(t *testing.T) *TimeoutCert {
	tc := &TimeoutCert{
		Timeout:    sb.Timeout(t, nil),
		Signatures: map[string]*TimeoutVote{},
	}

	for k, v := range sb.signers {
		sig := calcTimeoutSig(t, v, tc.Timeout.Round, 0, tc.Timeout.GetHqcRound(), k)
		tc.Signatures[k] = &TimeoutVote{HqcRound: tc.Timeout.GetHqcRound(), Signature: sig}
	}
	return tc
}

func (sb structBuilder) BlockData(t *testing.T) *BlockData {
	block := &BlockData{
		Author:    sb.RandomPeerID(t),
		Round:     21,
		Epoch:     0,
		Timestamp: 0x0102030405060708,
		Payload:   &Payload{}, // empty payload is valid
	}
	block.Qc = sb.QC(t, block.Round-1)

	return block
}

func Test_structBuilder(t *testing.T) {
	sb := newStructBuilder(t, 3)
	tb := sb.trustBase
	require.NotNil(t, tb)
	require.Equal(t, len(sb.signers), len(sb.verifiers))
	for k := range sb.verifiers {
		require.NotNil(t, sb.signers[k], "missing signer %q", k)
	}

	// make sure we get valid objects from builder
	qc := sb.QC(t, 42)
	require.NoError(t, qc.IsValid())
	require.NoError(t, qc.Verify(tb))

	tc := sb.TimeoutCert(t)
	require.NoError(t, tc.Verify(tb))

	to := sb.Timeout(t, tc)
	require.NoError(t, to.IsValid())
	require.NoError(t, to.Verify(tb))
	to = sb.Timeout(t, nil)
	require.NoError(t, to.IsValid())
	require.NoError(t, to.Verify(tb))

	bd := sb.BlockData(t)
	require.NoError(t, bd.IsValid())
	require.NoError(t, bd.Verify(tb))
}
