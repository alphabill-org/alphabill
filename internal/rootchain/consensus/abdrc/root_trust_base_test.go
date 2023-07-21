package abdrc

import (
	gocrypto "crypto"
	"crypto/sha256"
	"strconv"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func generateDummyValidatorMap(nofValidators uint64) map[string]crypto.Verifier {
	validators := make(map[string]crypto.Verifier)

	for i := uint64(0); i < nofValidators; {
		signer, _ := crypto.NewInMemorySecp256K1Signer()
		ver, _ := signer.Verifier()
		validators[strconv.FormatUint(i, 10)] = ver
		i++
	}
	return validators
}

func generateSignersAndVerifiers(nofValidators uint64) (map[string]crypto.Signer, map[string]crypto.Verifier) {
	signers := make(map[string]crypto.Signer)
	validators := make(map[string]crypto.Verifier)

	for i := uint64(0); i < nofValidators; {
		signer, _ := crypto.NewInMemorySecp256K1Signer()
		ver, _ := signer.Verifier()
		signers[strconv.FormatUint(i, 10)] = signer
		validators[strconv.FormatUint(i, 10)] = ver
		i++
	}
	return signers, validators
}

func createSignatures(t *testing.T, hash []byte, signers map[string]crypto.Signer) map[string][]byte {
	t.Helper()
	signatures := make(map[string][]byte)
	for id, signer := range signers {
		sig, err := signer.SignHash(hash)
		require.NoError(t, err)
		signatures[id] = sig
	}
	return signatures
}

func TestNewRootClusterVerifier(t *testing.T) {
	type args struct {
		keyMap    map[string]crypto.Verifier
		threshold uint32
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name:       "5 nodes threshold 4 ok",
			args:       args{keyMap: generateDummyValidatorMap(5), threshold: 4},
			wantErrStr: "",
		},
		{
			name:       "5 nodes threshold 5 ok",
			args:       args{keyMap: generateDummyValidatorMap(5), threshold: 5},
			wantErrStr: "",
		},
		{
			name:       "5 nodes threshold 3 - too small",
			args:       args{keyMap: generateDummyValidatorMap(5), threshold: 3},
			wantErrStr: "quorum threshold 3 is too low, for 5 validators min quorum is 4",
		},
		{
			name:       "Threshold too high",
			args:       args{keyMap: generateDummyValidatorMap(5), threshold: 6},
			wantErrStr: "quorum threshold 6 is too high - only 5 root validator keys registered",
		},
		{
			name:       "10/6 - too low",
			args:       args{keyMap: generateDummyValidatorMap(10), threshold: 6},
			wantErrStr: "quorum threshold 6 is too low, for 10 validators min quorum is 7",
		},
		{
			name:       "10/7 - ok",
			args:       args{keyMap: generateDummyValidatorMap(10), threshold: 7},
			wantErrStr: "",
		},
		{
			name:       "4/3 -ok",
			args:       args{keyMap: generateDummyValidatorMap(4), threshold: 3},
			wantErrStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRootTrustBase(tt.args.keyMap, tt.args.threshold)

			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Nil(t, got)
				return
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
			}
		})
	}
}

func TestRootNodeTrustBase_GetQuorumThreshold(t *testing.T) {
	keyMap := generateDummyValidatorMap(4)
	ver, err := NewRootTrustBase(keyMap, 3)
	require.NoError(t, err)
	require.Equal(t, uint32(3), ver.GetQuorumThreshold())
}

func TestRootNodeTrustBase_GetVerifier(t *testing.T) {
	// generates map with node id's from "0" to "3"
	keyMap := generateDummyValidatorMap(4)
	ver, err := NewRootTrustBase(keyMap, 3)
	require.NoError(t, err)
	nodeVer, err := ver.GetVerifier("1")
	require.NoError(t, err)
	key, err := nodeVer.MarshalPublicKey()
	require.NoError(t, err)
	require.NotEmpty(t, key)
	// Get missing node ver
	nodeVer, err = ver.GetVerifier("4")
	require.ErrorContains(t, err, "no public key exist for node id")
	require.Nil(t, nodeVer)
}

func TestRootNodeTrustBase_GetVerifiers(t *testing.T) {
	// generates map with node id's from "0" to "3"
	keyMap := generateDummyValidatorMap(4)
	ver, err := NewRootTrustBase(keyMap, 3)
	require.NoError(t, err)
	nodeMap := ver.GetVerifiers()
	require.Equal(t, 4, len(nodeMap))
}

func TestRootNodeTrustBase_ValidateQuorum(t *testing.T) {
	type args struct {
		keyMap    map[string]crypto.Verifier
		threshold uint32
		authors   []string
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "All ok",
			args: args{keyMap: generateDummyValidatorMap(4), threshold: 3,
				authors: []string{"0", "1", "3"}},
			wantErrStr: "",
		},
		{
			name: "Unknown node id",
			args: args{keyMap: generateDummyValidatorMap(4), threshold: 3,
				authors: []string{"0", "1", "2", "5"}},
			wantErrStr: "invalid quorum: unknown author",
		},
		{
			name: "Less than quorum of authors",
			args: args{keyMap: generateDummyValidatorMap(4), threshold: 3,
				authors: []string{"0", "1"}},
			wantErrStr: "invalid quorum: requires",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier, err := NewRootTrustBase(tt.args.keyMap, tt.args.threshold)
			require.NoError(t, err)
			err = verifier.ValidateQuorum(tt.args.authors)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			}
		})
	}
}

func TestRootNodeTrustBase_VerifyBytes(t *testing.T) {
	signers, validators := generateSignersAndVerifiers(4)
	commitInfo := &types.UnicitySeal{RootInternalInfo: []byte{0, 1, 2}, Hash: []byte{2, 3, 4}}
	bytes := commitInfo.Bytes()
	signer := signers["0"]
	sig, err := signer.SignBytes(bytes)
	require.NoError(t, err)
	verifier, err := NewRootTrustBase(validators, 3)
	require.NoError(t, err)
	require.NoError(t, verifier.VerifyBytes(bytes, sig, "0"))
	require.ErrorContains(t, verifier.VerifyBytes(bytes, sig, "N"), "no public key exist for node id")
	// modify bytes, so signature becomes invalid
	bytes = append(bytes, 1)
	require.ErrorContains(t, verifier.VerifyBytes(bytes, sig, "0"), "signature verify failed")
}

func TestRootNodeTrustBase_VerifyQuorumSignatures(t *testing.T) {
	signers, validators := generateSignersAndVerifiers(4)
	verifier, err := NewRootTrustBase(validators, 3)
	require.NoError(t, err)
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 1, 2, 3})
	commitInfo := NewDummyLedgerCommitInfo(voteInfo)
	hash := sha256.Sum256(commitInfo.Bytes())
	signatures := createSignatures(t, hash[:], signers)
	qc := &abtypes.QuorumCert{VoteInfo: voteInfo, LedgerCommitInfo: commitInfo, Signatures: signatures}
	require.NoError(t, verifier.VerifyQuorumSignatures(hash[:], qc.Signatures))
	// add an unknown signature
	signatures["5"] = []byte{0, 1, 2, 3, 4}
	require.ErrorContains(t, verifier.VerifyQuorumSignatures(hash[:], qc.Signatures), "quorum verify failed: more signatures")
	// remove two so that there are less than quorum of signatures
	delete(signatures, "5")
	delete(signatures, "3")
	delete(signatures, "2")
	require.ErrorContains(t, verifier.VerifyQuorumSignatures(hash[:], qc.Signatures), "quorum verify failed: no quorum")
	// add invalid signer
	signatures["5"] = []byte{0, 1, 2, 3, 4}
	require.ErrorContains(t, verifier.VerifyQuorumSignatures(hash[:], qc.Signatures), "quorum verify failed: failed to find public key for author")
	delete(signatures, "5")
	// add invalid signature
	signatures["3"] = []byte{0, 1, 2, 3, 4}
	require.ErrorContains(t, verifier.VerifyQuorumSignatures(hash[:], qc.Signatures), "quorum verify failed: signature length")
}

func TestRootNodeTrustBase_VerifySignature(t *testing.T) {
	signers, validators := generateSignersAndVerifiers(4)
	// create certificate for previous round
	roundInfo := &abtypes.RoundInfo{RoundNumber: 1, Timestamp: 11111, CurrentRootHash: []byte{0, 1, 3}}

	blockData := &abtypes.BlockData{
		Author:    "0",
		Round:     2,
		Epoch:     0,
		Timestamp: 11111,
		Payload:   &abtypes.Payload{},
		Qc: &abtypes.QuorumCert{
			VoteInfo:         roundInfo,
			LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: roundInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"0": {1, 2, 3}},
		},
	}
	signer := signers["0"]
	hash := blockData.Hash(gocrypto.SHA256)
	require.NotNil(t, hash)
	sig, err := signer.SignHash(hash)
	require.NoError(t, err)
	verifier, err := NewRootTrustBase(validators, 3)
	require.NoError(t, err)
	require.NoError(t, verifier.VerifySignature(hash, sig, peer.ID(blockData.Author)))
}
