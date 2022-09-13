package rootchain

import (
	"github.com/alphabill-org/alphabill/internal/cluster_verifier"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/libp2p/go-libp2p-core/peer"
)

type RootClusterVerifier struct {
	// holds a reference to config map
	nodeToPubkeyMap map[peer.ID][]byte
	// Configured or calculated threshold
	quorumThreshold uint32
}

func NewRootClusterVerifier(validators []*genesis.PublicKeyInfo, consensus *genesis.ConsensusParams) (cluster_verifier.ClusterVerifier, error) {
	keyMap := make(map[peer.ID][]byte)
	for _, nodeInfo := range validators {
		keyMap[peer.ID(nodeInfo.NodeIdentifier)] = nodeInfo.SigningPublicKey
	}
	quorum := genesis.GetMinQuorumThreshold(consensus.TotalRootValidators)
	if consensus.QuorumThreshold != nil {
		quorum = *consensus.QuorumThreshold
	}
	return &RootClusterVerifier{nodeToPubkeyMap: keyMap, quorumThreshold: quorum}, nil
}

// GetQuorumThreshold returns quorum power needed.
// Currently, all validators are equal and each vote counts as
func (r *RootClusterVerifier) GetQuorumThreshold() uint32 {
	return r.quorumThreshold
}

func (r *RootClusterVerifier) VerifySignature(hash []byte, sig []byte, author peer.ID) error {
	ver, err := r.GetVerifier(author)
	if err != nil {
		return errors.Errorf("Failed to find public key for author %v", author)
	}
	err = ver.VerifyHash(sig, hash)
	if err != nil {
		return err
	}
	return nil
}

func (r *RootClusterVerifier) ValidateQuorum(authors []string) error {
	for _, author := range authors {
		_, err := r.GetVerifier(peer.ID(author))
		if err != nil {
			return errors.Errorf("Timeout certificate, unknown author %v", author)
		}
	}
	// check less than quorum of authors
	if uint32(len(authors)) < r.GetQuorumThreshold() {
		return errors.Errorf("Timeout certificate, less than quorum %v of votes %v",
			r.GetQuorumThreshold(), len(authors))
	}
	return nil
}

func (r *RootClusterVerifier) VerifySignatures(hash []byte, signatures map[string][]byte) error {
	// Todo: This could be handled in parallel, if the size of the signatures grows big
	// Quick sanity check, make sure that there are not more signatures that we know public keys for
	err := r.CheckNumberOfSignatures(signatures)
	if err != nil {
		return errors.Wrap(err, "Quorum verify failed")
	}
	// Check quorum, if not fail without checking signatures itself
	if uint32(len(signatures)) < r.GetQuorumThreshold() {
		return errors.New("Too few signatures for quorum")
	}
	for author, sig := range signatures {
		err := r.VerifySignature(hash, sig, peer.ID(author))
		if err != nil {
			return errors.Wrap(err, "Quorum verify failed")
		}
	}
	return nil
}

// CheckNumberOfSignatures makes sure there are not more signatures that registered public keys
func (r *RootClusterVerifier) CheckNumberOfSignatures(signatures map[string][]byte) error {
	if len(r.nodeToPubkeyMap) < len(signatures) {
		return errors.Errorf("More signatures %v than registered public key's %v",
			len(signatures), r.nodeToPubkeyMap)
	}
	return nil
}

func (r *RootClusterVerifier) GetVerifier(nodeId peer.ID) (crypto.Verifier, error) {
	pubKey, exists := r.nodeToPubkeyMap[nodeId]
	if exists == false {
		return nil, errors.Errorf("No public key exist for node id %v", nodeId)
	}
	ver, err := crypto.NewVerifierSecp256k1(pubKey)
	return ver, err
}
