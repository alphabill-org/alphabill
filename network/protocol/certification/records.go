package certification

import (
	"crypto"
	"encoding/binary"
)

/*
There is one Technical Record for every shard of every partition, providing
synchronization for the next block production attempt.
*/
type TechnicalRecord struct {
	_        struct{} `cbor:",toarray"`
	Round    uint64
	Epoch    uint64
	Leader   string // identifier of the round leader
	StatHash []byte // hash of statistical records
	FeeHash  []byte // hash of validator fee records
}

func (tr *TechnicalRecord) Hash(algo crypto.Hash) []byte {
	h := algo.New()

	h.Write(binary.BigEndian.AppendUint64(nil, tr.Round))
	h.Write(binary.BigEndian.AppendUint64(nil, tr.Epoch))
	h.Write([]byte(tr.Leader))
	h.Write(tr.StatHash)
	h.Write(tr.FeeHash)

	return h.Sum(nil)
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

func (sr *StatisticalRecord) Bytes() []byte {
	buf := make([]byte, 0, 7*8)
	buf = binary.BigEndian.AppendUint64(buf, sr.Blocks)
	buf = binary.BigEndian.AppendUint64(buf, sr.BlockFees)
	buf = binary.BigEndian.AppendUint64(buf, sr.BlockSize)
	buf = binary.BigEndian.AppendUint64(buf, sr.StateSize)
	buf = binary.BigEndian.AppendUint64(buf, sr.MaxFee)
	buf = binary.BigEndian.AppendUint64(buf, sr.MaxBlockSize)
	buf = binary.BigEndian.AppendUint64(buf, sr.MaxStateSize)
	return buf
}
