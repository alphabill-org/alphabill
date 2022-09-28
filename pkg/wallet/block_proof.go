package wallet

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/mt"
)

// TODO AB-385
func ExtractBlockProof(b *block.Block, txIdx int, hashAlgorithm crypto.Hash) (*block.BlockProof, error) {
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

	return &block.BlockProof{
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		MerkleProof:        block.ToProtobuf(path),
		UnicityCertificate: b.UnicityCertificate,
	}, nil
}
