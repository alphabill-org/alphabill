package testcertificates

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
)

func CreateUnicityCertificate(
	t *testing.T,
	signer crypto.Signer,
	ir *types.InputRecord,
	pdr *types.PartitionDescriptionRecord,
	roundNumber uint64,
	previousRoundRootHash []byte,
	trHash []byte,
) *types.UnicityCertificate {
	t.Helper()
	sTree, err := types.CreateShardTree(pdr.Shards, []types.ShardTreeInput{{Shard: types.ShardID{}, IR: ir, TRHash: trHash}}, gocrypto.SHA256)
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
		Partition:     pdr.PartitionIdentifier,
		ShardTreeRoot: sTree.RootHash(),
		PDRHash:       pdr.Hash(gocrypto.SHA256),
	}}
	ut, err := types.NewUnicityTree(gocrypto.SHA256, data)
	if err != nil {
		t.Error(err)
	}
	rootHash := ut.RootHash()
	unicitySeal := createUnicitySeal(rootHash, roundNumber, previousRoundRootHash)
	err = unicitySeal.Sign("test", signer)
	if err != nil {
		t.Error(err)
	}
	cert, err := ut.Certificate(pdr.PartitionIdentifier)
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

func createUnicitySeal(rootHash []byte, roundNumber uint64, previousRoundRootHash []byte) *types.UnicitySeal {
	return &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: roundNumber,
		Timestamp:            types.NewTimestamp(),
		PreviousHash:         previousRoundRootHash,
		Hash:                 rootHash,
	}
}
