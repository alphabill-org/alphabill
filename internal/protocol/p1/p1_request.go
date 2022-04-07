package p1

import (
	"bytes"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
)

var (
	ErrP1RequestIsNil                = errors.New("node P1 request is nil")
	ErrInvalidSystemIdentifierLength = errors.New("invalid system identifier length")
	ErrVerifierIsNil                 = errors.New("verifier is nil")
	ErrEmptyNodeIdentifier           = errors.New("node identifier is empty")
)

func (x *P1Request) IsValid(v crypto.Verifier) error {
	if x == nil {
		return ErrP1RequestIsNil
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
		return err
	}
	return nil
}

func (x *P1Request) Sign(signer crypto.Signer) error {
	signature, err := signer.SignBytes(x.Bytes())
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}

func (x *P1Request) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.SystemIdentifier)
	b.WriteString(x.NodeIdentifier)
	b.Write(util.Uint64ToBytes(x.RootRoundNumber))
	b.Write(x.InputRecord.PreviousHash)
	b.Write(x.InputRecord.Hash)
	b.Write(x.InputRecord.BlockHash)
	b.Write(x.InputRecord.SummaryValue)
	return b.Bytes()
}
