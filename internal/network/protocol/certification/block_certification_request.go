package certification

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrBlockCertificationRequestIsNil = errors.New("block certification request is nil")
	ErrInvalidSystemIdentifierLength  = errors.New("invalid system identifier length")
	ErrVerifierIsNil                  = errors.New("verifier is nil")
	ErrEmptyNodeIdentifier            = errors.New("node identifier is empty")
)

func (x *BlockCertificationRequest) IsValid(v crypto.Verifier) error {
	if x == nil {
		return ErrBlockCertificationRequestIsNil
	}
	if v == nil {
		return ErrVerifierIsNil
	}
	if len(x.SystemIdentifier) != 4 {
		return ErrInvalidSystemIdentifierLength
	}
	if x.NodeIdentifier == "" {
		return ErrEmptyNodeIdentifier
	}
	if err := x.InputRecord.IsValid(); err != nil {
		return err
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
		return err
	}
	x.Signature = signature
	return nil
}

func (x *BlockCertificationRequest) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.SystemIdentifier)
	b.WriteString(x.NodeIdentifier)
	b.Write(x.InputRecord.PreviousHash)
	b.Write(x.InputRecord.Hash)
	b.Write(x.InputRecord.BlockHash)
	b.Write(x.InputRecord.SummaryValue)
	b.Write(util.Uint64ToBytes(x.InputRecord.RoundNumber))
	return b.Bytes()
}
