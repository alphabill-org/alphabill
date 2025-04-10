package testcertificates

import (
	gocrypto "crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func CreateUnicityCertificate(
	t *testing.T,
	signer crypto.Signer,
	ir *types.InputRecord,
	shardConf *types.PartitionDescriptionRecord,
	rootRound uint64,
	previousHash []byte,
	trHash []byte,
) *types.UnicityCertificate {
	t.Helper()
	shardConfHash := test.DoHash(t, shardConf)
	sTree, err := types.CreateShardTree(types.ShardingScheme{}, []types.ShardTreeInput{
		{Shard: types.ShardID{}, IR: ir, TRHash: trHash, ShardConfHash: shardConfHash},
	}, gocrypto.SHA256)
	if err != nil {
		t.Errorf("creating shard tree: %v", err)
		return nil
	}
	stCert, err := sTree.Certificate(types.ShardID{})
	if err != nil {
		t.Errorf("creating shard tree certificate: %v", err)
		return nil
	}
	data := []*types.UnicityTreeData{{
		Partition:     shardConf.PartitionID,
		ShardTreeRoot: sTree.RootHash(),
	}}
	ut, err := types.NewUnicityTree(gocrypto.SHA256, data)
	if err != nil {
		t.Error(err)
	}
	rootHash := ut.RootHash()
	unicitySeal := createUnicitySeal(rootHash, rootRound, previousHash)

	verifier, err := signer.Verifier()
	require.NoError(t, err)
	nodeID := nodeIDFromVerifier(t, verifier).String()

	err = unicitySeal.Sign(nodeID, signer)
	if err != nil {
		t.Error(err)
	}
	cert, err := ut.Certificate(shardConf.PartitionID)
	if err != nil {
		t.Error(err)
	}
	return &types.UnicityCertificate{
		Version:                1,
		InputRecord:            ir,
		TRHash:                 trHash,
		ShardConfHash:          shardConfHash,
		ShardTreeCertificate:   stCert,
		UnicityTreeCertificate: cert,
		UnicitySeal:            unicitySeal,
	}
}

func createUnicitySeal(rootHash []byte, roundNumber uint64, previousHash []byte) *types.UnicitySeal {
	return &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: roundNumber,
		Timestamp:            types.NewTimestamp(),
		PreviousHash:         previousHash,
		Hash:                 rootHash,
	}
}

func nodeIDFromVerifier(t *testing.T, v crypto.Verifier) peer.ID {
	pubKeyBytes, err := v.MarshalPublicKey()
	require.NoError(t, err)
	pubKey, err := p2pcrypto.UnmarshalSecp256k1PublicKey(pubKeyBytes)
	require.NoError(t, err)
	peerID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)
	return peerID
}

func UnicitySealBytes(t *testing.T, unicitySeal *types.UnicitySeal) []byte {
	t.Helper()
	h, err := unicitySeal.SigBytes()
	require.NoError(t, err)
	return h
}
