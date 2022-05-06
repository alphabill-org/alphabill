package partition

import (
	gocrypto "crypto"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

type Configuration struct {
	// after a node sees a UC which appoints a leader, the leader waits for t1 time units before stopping accepting new
	// transaction orders and creating a block proposal.
	T1Timeout     time.Duration
	TrustBase     crypto.Verifier
	Signer        crypto.Signer
	HashAlgorithm gocrypto.Hash
	Genesis       *genesis.PartitionGenesis
}

func (c *Configuration) GetSystemIdentifier() []byte {
	return c.Genesis.SystemDescriptionRecord.SystemIdentifier
}

func (c *Configuration) GetPublicKey(nodeIdentifier string) (crypto.Verifier, error) {
	keys := c.Genesis.Keys
	for _, key := range keys {
		if key.NodeIdentifier != nodeIdentifier {
			continue
		}
		return crypto.NewVerifierSecp256k1(key.PublicKey)

	}
	return nil, errors.Errorf("public key with node id %v not found", nodeIdentifier)
}
