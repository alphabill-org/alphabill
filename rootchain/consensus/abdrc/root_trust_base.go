package abdrc

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/libp2p/go-libp2p/core/peer"
)

type RootTrustBase struct {
	// holds a reference to config map
	nodeToPubkeyMap map[string]crypto.Verifier
	// Configured or calculated threshold
	quorumThreshold uint32
}

func NewRootTrustBase(keyMap map[string]crypto.Verifier, threshold uint32) (*RootTrustBase, error) {
	nofNodes := uint32(len(keyMap))
	minThreshold := nofNodes*2/3 + 1

	if threshold > nofNodes {
		// Quorum power is bigger than nof root validators registered
		return nil, fmt.Errorf("quorum threshold %d is too high - only %d root validator keys registered",
			threshold, nofNodes)
	}
	if threshold < minThreshold {
		// Threshold is configured too low, not safe
		return nil, fmt.Errorf("quorum threshold %d is too low, for %d validators min quorum is %d",
			threshold, nofNodes, minThreshold)
	}
	return &RootTrustBase{nodeToPubkeyMap: keyMap, quorumThreshold: threshold}, nil
}

func NewRootTrustBaseFromGenesis(genesisRoot *genesis.GenesisRootRecord) (*RootTrustBase, error) {
	keyMap, err := genesis.NewValidatorTrustBase(genesisRoot.RootValidators)
	if err != nil {
		return nil, fmt.Errorf("failed to extract root validator public info from genesis: %w", err)
	}
	return NewRootTrustBase(keyMap, genesisRoot.Consensus.QuorumThreshold)
}

// GetQuorumThreshold returns quorum power needed.
// Currently, all validators are equal and each vote counts as one.
func (r *RootTrustBase) GetQuorumThreshold() uint32 {
	return r.quorumThreshold
}

// GetMaxFaultyNodes a.k.a get max allowed faulty nodes
func (r *RootTrustBase) GetMaxFaultyNodes() uint32 {
	return uint32(len(r.nodeToPubkeyMap)) - r.quorumThreshold
}

func (r *RootTrustBase) VerifySignature(hash []byte, sig []byte, author peer.ID) error {
	ver, err := r.GetVerifier(author)
	if err != nil {
		return fmt.Errorf("failed to find public key for author %v", author)
	}
	err = ver.VerifyHash(sig, hash)
	if err != nil {
		return err
	}
	return nil
}

func (r *RootTrustBase) VerifyBytes(bytes []byte, sig []byte, author peer.ID) error {
	ver, err := r.GetVerifier(author)
	if err != nil {
		return err
	}
	err = ver.VerifyBytes(sig, bytes)
	if err != nil {
		return err
	}
	return nil
}

func (r *RootTrustBase) ValidateQuorum(authors []string) error {
	// 1. Check if authors are known
	for _, author := range authors {
		_, err := r.GetVerifier(peer.ID(author))
		if err != nil {
			return fmt.Errorf("invalid quorum: unknown author %v", author)
		}
	}
	// 2. Check that at least quorum number of authors present
	if uint32(len(authors)) < r.GetQuorumThreshold() {
		return fmt.Errorf("invalid quorum: requires %v only %v present",
			r.GetQuorumThreshold(), len(authors))
	}
	return nil
}

func (r *RootTrustBase) VerifyQuorumSignatures(hash []byte, signatures map[string][]byte) error {
	// Quick sanity check, make sure that there are not more signatures that we know public keys for
	err := r.checkNumberOfSignatures(signatures)
	if err != nil {
		return fmt.Errorf("quorum verify failed: %w", err)
	}
	// Check quorum, if not fail without checking signatures itself
	if uint32(len(signatures)) < r.GetQuorumThreshold() {
		return fmt.Errorf("quorum verify failed: no quorum %d of %d signed",
			len(signatures), r.GetQuorumThreshold())
	}
	// Verify all signatures
	for author, sig := range signatures {
		err := r.VerifySignature(hash, sig, peer.ID(author))
		if err != nil {
			return fmt.Errorf("quorum verify failed: %w", err)
		}
	}
	return nil
}

// CheckNumberOfSignatures makes sure there are not more signatures that registered public keys
func (r *RootTrustBase) checkNumberOfSignatures(signatures map[string][]byte) error {
	if len(r.nodeToPubkeyMap) < len(signatures) {
		return fmt.Errorf("more signatures %v than registered public key's %v",
			len(signatures), len(r.nodeToPubkeyMap))
	}
	return nil
}

func (r *RootTrustBase) GetVerifier(nodeID peer.ID) (crypto.Verifier, error) {
	ver, found := r.nodeToPubkeyMap[string(nodeID)]
	if !found {
		return nil, fmt.Errorf("no public key exist for node id %v", nodeID.String())
	}
	return ver, nil
}

func (r *RootTrustBase) GetVerifiers() map[string]crypto.Verifier {
	return r.nodeToPubkeyMap
}
