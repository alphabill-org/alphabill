package money

import (
	"crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/mt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/proof"
)

func ExtractBlockProof(b *block.Block, txIdx int, hashAlgorithm crypto.Hash) (*proof.BlockProof, error) {
	mtTxs := make([]mt.Data, len(b.Transactions))
	for i, txb := range b.Transactions {
		mtTxs[i] = &mt.ByteHasher{Val: txb.Bytes()}
	}

	merkleTree, err := mt.New(hashAlgorithm, mtTxs)
	if err != nil {
		return nil, err
	}

	path, err := merkleTree.GetMerklePath(txIdx)
	if err != nil {
		return nil, err
	}

	return &proof.BlockProof{
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		MerkleProof:        mt.ToProtobuf(path),
		UnicityCertificate: b.UnicityCertificate,
	}, nil
}
