package partition

import (
	gocrypto "crypto"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
)

type Configuration struct {
	SystemIdentifier []byte

	// after a node sees a UC which appoints a leader, the leader waits for t1 time units before stopping accepting new
	// transaction orders and creating a block proposal.
	T1Timeout time.Duration

	Signer crypto.Signer

	HashAlgorithm gocrypto.Hash

	Genesis *Genesis // TODO AB-111
}
