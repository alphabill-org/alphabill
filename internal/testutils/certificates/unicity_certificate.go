package testcertificates

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/rootchain/unicitytree"
)

func CreateUnicityCertificate(
	t *testing.T,
	signer crypto.Signer,
	ir *types.InputRecord,
	systemDescription *types.SystemDescriptionRecord,
	roundNumber uint64,
	previousRoundRootHash []byte,

) *types.UnicityCertificate {
	t.Helper()
	data := []*types.UnicityTreeData{{
		SystemIdentifier:            systemDescription.SystemIdentifier,
		InputRecord:                 ir,
		SystemDescriptionRecordHash: systemDescription.Hash(gocrypto.SHA256),
	}}
	ut, err := unicitytree.New(gocrypto.SHA256, data)
	if err != nil {
		t.Error(err)
	}
	rootHash := ut.GetRootHash()
	unicitySeal := createUnicitySeal(rootHash, roundNumber, previousRoundRootHash)
	err = unicitySeal.Sign("test", signer)
	if err != nil {
		t.Error(err)
	}
	cert, err := ut.GetCertificate(systemDescription.SystemIdentifier)
	if err != nil {
		t.Error(err)
	}
	return &types.UnicityCertificate{
		InputRecord: ir,
		UnicityTreeCertificate: &types.UnicityTreeCertificate{
			SystemIdentifier:      cert.SystemIdentifier,
			SiblingHashes:         cert.SiblingHashes,
			SystemDescriptionHash: systemDescription.Hash(gocrypto.SHA256),
		},
		UnicitySeal: unicitySeal,
	}
}

func createUnicitySeal(rootHash []byte, roundNumber uint64, previousRoundRootHash []byte) *types.UnicitySeal {
	return &types.UnicitySeal{
		RootChainRoundNumber: roundNumber,
		Timestamp:            types.NewTimestamp(),
		PreviousHash:         previousRoundRootHash,
		Hash:                 rootHash,
	}
}
