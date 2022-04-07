package partition

import (
	"crypto"
	"time"
)

type Configuration struct {
	SystemIdentifier []byte

	// after a node sees a UC which appoints a leader, the leader waits for t1 time units before stopping accepting new
	// transaction orders and creating a block proposal.
	T1Timeout time.Duration

	Genesis       *Genesis // TODO AB-111
	HashAlgorithm crypto.Hash
}
