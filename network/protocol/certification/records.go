package certification

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

/*
There is one Technical Record for every shard of every partition, providing
synchronization for the next block production attempt.
*/
type TechnicalRecord struct {
	_        struct{} `cbor:",toarray"`
	Round    uint64
	Epoch    uint64
	Leader   string    // identifier of the round leader
	StatHash hex.Bytes // hash of statistical records
	FeeHash  hex.Bytes // hash of validator fee records
}

func (tr *TechnicalRecord) IsValid() error {
	if tr.Round == 0 {
		return errors.New("round is unassigned")
	}
	if tr.Leader == "" {
		return errors.New("leader is unassigned")
	}
	if len(tr.StatHash) == 0 {
		return errors.New("stat hash is unassigned")
	}
	if len(tr.FeeHash) == 0 {
		return errors.New("fee hash is unassigned")
	}
	return nil
}

func (tr *TechnicalRecord) Hash() ([]byte, error) {
	h := abhash.New(crypto.SHA256.New())
	h.Write(tr)
	return h.Sum()
}

func (tr *TechnicalRecord) HashMatches(trh []byte) error {
	h, err := tr.Hash()
	if err != nil {
		return fmt.Errorf("calculating hash: %w", err)
	}
	if !bytes.Equal(trh, h) {
		return errors.New("hash mismatch")
	}

	return nil
}

/*
There is one Statistical Record of current epoch, and one Statistical Record of previous
epoch of every shard of every public partition.
*/
type StatisticalRecord struct {
	_            struct{} `cbor:",toarray"`
	Blocks       uint64   // number of non-empty blocks in the epoch
	BlockFees    uint64   // total block fees of the epoch
	BlockSize    uint64   // the sum of all block sizes of the epoch
	StateSize    uint64   // the sum of all state sizes of the epoch
	MaxFee       uint64   // maximum block fee of the epoch
	MaxBlockSize uint64   // maximum block size of the epoch
	MaxStateSize uint64   // maximum state size of the epoch
}
