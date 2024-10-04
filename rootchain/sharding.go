package rootchain

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"maps"
	"slices"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

type shardInfo struct {
	Round    uint64
	Epoch    uint64
	RootHash []byte // last certified root hash

	// statistical record of the previous epoch. As we only need
	// it for hashing we keep it in serialized representation
	PrevEpochStat []byte

	// statistical record of the current epoch
	Stat certification.StatisticalRecord

	// per validator total, invariant fees of the previous epoch
	// but as with statistical record of the previous epoch we need it
	// for hashing so we keep it in serialized representation
	PrevEpochFees []byte

	Fees   map[string]uint64         // per validator summary fees of the current epoch
	Leader string                    // identifier of the Round leader
	UC     *types.UnicityCertificate // last created unicity certificate
}

/*
StatHash returns hash of the StatisticalRecord, suitable
for use in Technical Record.
*/
func (si *shardInfo) StatHash(algo crypto.Hash) []byte {
	h := algo.New()
	h.Write(si.PrevEpochStat)
	h.Write(si.Stat.Bytes())
	return h.Sum(nil)
}

func (si *shardInfo) FeeHash(algo crypto.Hash) []byte {
	h := algo.New()
	h.Write(si.PrevEpochFees)
	h.Write(si.feeBytes())
	return h.Sum(nil)
}

func (si *shardInfo) Update(req *certification.BlockCertificationRequest) {
	si.Round = req.IRRound()
	si.RootHash = req.InputRecord.Hash
	// we need to get current leader form CR until we implement validator
	// leader selection on the RC side! (AB-1719)
	si.Fees[req.Leader] += req.InputRecord.SumOfEarnedFees

	if !bytes.Equal(req.InputRecord.Hash, req.InputRecord.PreviousHash) {
		si.Stat.Blocks += 1
	}
	si.Stat.BlockFees += req.InputRecord.SumOfEarnedFees
	si.Stat.BlockSize += req.BlockSize
	si.Stat.StateSize += req.StateSize
	si.Stat.MaxFee = max(si.Stat.MaxFee, req.InputRecord.SumOfEarnedFees)
	si.Stat.MaxBlockSize = max(si.Stat.MaxBlockSize, req.BlockSize)
	si.Stat.MaxStateSize = max(si.Stat.MaxStateSize, req.StateSize)
}

// feeBytes returns serialized "node->fees" data for hashing
func (si *shardInfo) feeBytes() []byte {
	nodes := slices.Collect(maps.Keys(si.Fees))
	slices.Sort(nodes)
	buf := bytes.NewBuffer(nil)
	fee := make([]byte, 8)
	for _, nodeID := range nodes {
		buf.Write([]byte(nodeID))
		binary.BigEndian.PutUint64(fee, si.Fees[nodeID])
		buf.Write(fee)
	}
	return buf.Bytes()
}
