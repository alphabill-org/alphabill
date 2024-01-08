package certification

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

var (
	ErrBlockCertificationRequestIsNil = errors.New("block certification request is nil")
	errInvalidSystemIdentifier        = errors.New("invalid system identifier")
	errVerifierIsNil                  = errors.New("verifier is nil")
	errEmptyNodeIdentifier            = errors.New("node identifier is empty")
)

type BlockCertificationRequest struct {
	_                struct{}           `cbor:",toarray"`
	SystemIdentifier types.SystemID     `json:"system_identifier,omitempty"`
	NodeIdentifier   string             `json:"node_identifier,omitempty"`
	InputRecord      *types.InputRecord `json:"input_record,omitempty"`
	RootRoundNumber  uint64             `json:"root_round_number,omitempty"` // latest known RC's round number (AB-1155)
	Signature        []byte             `json:"signature,omitempty"`
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

func (x *BlockCertificationRequest) RootRound() uint64 {
	if x == nil {
		return 0
	}
	return x.RootRoundNumber
}

func (x *BlockCertificationRequest) IsValid(v crypto.Verifier) error {
	if x == nil {
		return ErrBlockCertificationRequestIsNil
	}
	if v == nil {
		return errVerifierIsNil
	}
	if x.SystemIdentifier == 0 {
		return errInvalidSystemIdentifier
	}
	if x.NodeIdentifier == "" {
		return errEmptyNodeIdentifier
	}
	if err := x.InputRecord.IsValid(); err != nil {
		return fmt.Errorf("input record error: %w", err)
	}
	if err := v.VerifyBytes(x.Signature, x.Bytes()); err != nil {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func (x *BlockCertificationRequest) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errors.New("signer is nil")
	}
	signature, err := signer.SignBytes(x.Bytes())
	if err != nil {
		return fmt.Errorf("sign error, %w", err)
	}
	x.Signature = signature
	return nil
}

func (x *BlockCertificationRequest) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.SystemIdentifier.Bytes())
	b.WriteString(x.NodeIdentifier)
	b.Write(x.InputRecord.PreviousHash)
	b.Write(x.InputRecord.Hash)
	b.Write(x.InputRecord.BlockHash)
	b.Write(x.InputRecord.SummaryValue)
	b.Write(util.Uint64ToBytes(x.InputRecord.RoundNumber))
	b.Write(util.Uint64ToBytes(x.RootRoundNumber))
	return b.Bytes()
}
