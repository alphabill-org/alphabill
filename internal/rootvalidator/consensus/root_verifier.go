package consensus

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/libp2p/go-libp2p/core/peer"
)

type RootNodeVerifier struct {
	// holds a reference to config map
	nodeToPubkeyMap map[string]crypto.Verifier
	// Configured or calculated threshold
	quorumThreshold uint32
}

func NewRootClusterVerifier(keyMap map[string]crypto.Verifier, threshold uint32) (*RootNodeVerifier, error) {
	nofNodes := uint32(len(keyMap))
	minFaulty := (nofNodes - 1) / 3
	if minFaulty == 0 {
		// Too few nodes, no failures can be tolerated
		return nil, errors.New("Too few root validator nodes for distributed root validator chain")
	}
	if nofNodes > threshold {
		// Too few nodes, no failures can be tolerated
		return nil, errors.New("Quorum threshold is configured bigger than number of root validator nodes")
	}
	if nofNodes-threshold < minFaulty {
		// Threshold is configured too high, no failures are tolerated
		return nil, errors.New("Quorum threshold is configured too high, no failures are tolerated")
	}
	return &RootNodeVerifier{nodeToPubkeyMap: keyMap, quorumThreshold: threshold}, nil
}

// GetQuorumThreshold returns quorum power needed.
// Currently, all validators are equal and each vote counts as
func (r *RootNodeVerifier) GetQuorumThreshold() uint32 {
	return r.quorumThreshold
}

func (r *RootNodeVerifier) VerifySignature(hash []byte, sig []byte, author peer.ID) error {
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

func (r *RootNodeVerifier) VerifyBytes(bytes []byte, sig []byte, author peer.ID) error {
	ver, err := r.GetVerifier(author)
	if err != nil {
		return errors.Errorf("Failed to find public key for author %v", author)
	}
	err = ver.VerifyBytes(sig, bytes)
	if err != nil {
		return err
	}
	return nil
}

func (r *RootNodeVerifier) ValidateQuorum(authors []string) error {
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

func (r *RootNodeVerifier) VerifyQuorumSignatures(hash []byte, signatures map[string][]byte) error {
	// Quick sanity check, make sure that there are not more signatures that we know public keys for
	err := r.checkNumberOfSignatures(signatures)
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
func (r *RootNodeVerifier) checkNumberOfSignatures(signatures map[string][]byte) error {
	if len(r.nodeToPubkeyMap) < len(signatures) {
		return errors.Errorf("More signatures %v than registered public key's %v",
			len(signatures), r.nodeToPubkeyMap)
	}
	return nil
}

func (r *RootNodeVerifier) GetVerifier(nodeId peer.ID) (crypto.Verifier, error) {
	ver, exists := r.nodeToPubkeyMap[string(nodeId)]
	if exists == false {
		return nil, errors.Errorf("No public key exist for node id %v", nodeId)
	}
	return ver, nil
}

func (r *RootNodeVerifier) GetVerifiers() map[string]crypto.Verifier {
	return r.nodeToPubkeyMap
}
