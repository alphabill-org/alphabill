package certification

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

var (
	ErrBlockCertificationRequestIsNil = errors.New("block certification request is nil")
	errInvalidPartitionID             = errors.New("invalid partition identifier")
	errVerifierIsNil                  = errors.New("verifier is nil")
	errEmptyNodeID                    = errors.New("node identifier is empty")
)

type BlockCertificationRequest struct {
	_              struct{}           `cbor:",toarray"`
	Partition      types.PartitionID  `json:"partitionId"`
	Shard          types.ShardID      `json:"shardIdentifier"`
	NodeID         string             `json:"nodeId"`
	InputRecord    *types.InputRecord `json:"inputRecord"`
	BlockSize      uint64             `json:"blockSize"`
	StateSize      uint64             `json:"stateSize"`
	Signature      hex.Bytes          `json:"signature"`
}

func (x *BlockCertificationRequest) IRRound() uint64 {
	if x == nil || x.InputRecord == nil {
		return 0
	}
	return x.InputRecord.RoundNumber
}

func (x *BlockCertificationRequest) IRPreviousHash() []byte {
	if x == nil || x.InputRecord == nil {
		return nil
	}
	return x.InputRecord.PreviousHash
}

func (x *BlockCertificationRequest) IsValid(v crypto.Verifier) error {
	if x == nil {
		return ErrBlockCertificationRequestIsNil
	}
	if v == nil {
		return errVerifierIsNil
	}
	if x.Partition == 0 {
		return errInvalidPartitionID
	}
	if x.NodeID == "" {
		return errEmptyNodeID
	}
	if err := x.InputRecord.IsValid(); err != nil {
		return fmt.Errorf("invalid input record: %w", err)
	}
	bs, err := x.Bytes()
	if err != nil {
		return fmt.Errorf("failed to marshal block certification request, %w", err)
	}
	if err := v.VerifyBytes(x.Signature, bs); err != nil {
		return fmt.Errorf("signature verification: %w", err)
	}
	return nil
}

func (x *BlockCertificationRequest) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errors.New("signer is nil")
	}
	bs, err := x.Bytes()
	if err != nil {
		return fmt.Errorf("failed to marshal block certification request, %w", err)
	}
	signature, err := signer.SignBytes(bs)
	if err != nil {
		return fmt.Errorf("sign error, %w", err)
	}
	x.Signature = signature
	return nil
}

func (x BlockCertificationRequest) Bytes() ([]byte, error) {
	x.Signature = nil
	return types.Cbor.Marshal(x)
}
