package cluster_verifier

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

type ClusterVerifier interface {
	// VerifySignature verifies single signature
	VerifySignature(hash []byte, sig []byte, author peer.ID) error
	// VerifySignatures verify all signatures
	VerifySignatures(hash []byte, signatures map[string][]byte) error
	// CheckNumberOfSignatures check number of signatures vs keys stored. There cannot be more
	// signatures than public keys available.
	CheckNumberOfSignatures(signatures map[string][]byte) error
	// GetVerifier return verifier for node id
	GetVerifier(nodeId peer.ID) (crypto.Verifier, error)
}
