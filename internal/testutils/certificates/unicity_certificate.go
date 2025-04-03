package testcertificates

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

func CreateUnicityCertificate(
	t *testing.T,
	signer crypto.Signer,
	ir *types.InputRecord,
	pdr *types.PartitionDescriptionRecord,
	rootRound uint64,
	previousHash []byte,
	trHash []byte,
) *types.UnicityCertificate {
	t.Helper()
	sTree, err := types.CreateShardTree(types.ShardingScheme{}, []types.ShardTreeInput{{Shard: types.ShardID{}, IR: ir, TRHash: trHash}}, gocrypto.SHA256)
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
		Partition:     pdr.PartitionID,
		ShardTreeRoot: sTree.RootHash(),
		PDRHash:       test.DoHash(t, pdr),
	}}
	ut, err := types.NewUnicityTree(gocrypto.SHA256, data)
	if err != nil {
		t.Error(err)
	}
	rootHash := ut.RootHash()
	unicitySeal := createUnicitySeal(rootHash, rootRound, previousHash)
	err = unicitySeal.Sign("test", signer)
	if err != nil {
		t.Error(err)
	}
	cert, err := ut.Certificate(pdr.PartitionID)
	if err != nil {
		t.Error(err)
	}
	return &types.UnicityCertificate{
		Version:                1,
		InputRecord:            ir,
		TRHash:                 trHash,
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

func UnicitySealBytes(t *testing.T, unicitySeal *types.UnicitySeal) []byte {
	t.Helper()
	h, err := unicitySeal.SigBytes()
	require.NoError(t, err)
	return h
}
