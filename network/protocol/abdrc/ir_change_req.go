package abdrc

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	abdrc "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type IrChangeReqMsg struct {
	_           struct{}           `cbor:",toarray"`
	Author      string             `json:"author"`
	IrChangeReq *abdrc.IRChangeReq `json:"irChangeReq"`
	Signature   hex.Bytes          `json:"signature"`
}

func (x *IrChangeReqMsg) IsValid() error {
	if x.Author == "" {
		return fmt.Errorf("author is missing")
	}
	if x.IrChangeReq == nil {
		return fmt.Errorf("request is nil")
	}
	if err := x.IrChangeReq.IsValid(); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}
	if x.IrChangeReq.CertReason == abdrc.T2Timeout {
		return fmt.Errorf("invalid reason, timeout can only be proposed by leader")
	}
	return nil
}

func (x *IrChangeReqMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errSignerIsNil
	}
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("ir change request msg not valid: %w", err)
	}
	bs, err := x.bytes()
	if err != nil {
		return fmt.Errorf("failed to marshal ir change request msg: %w", err)
	}
	signature, err := signer.SignBytes(bs)
	if err != nil {
		return fmt.Errorf("failed to sign ir change request: %w", err)
	}
	x.Signature = signature
	return nil
}

func (x *IrChangeReqMsg) Verify(tb types.RootTrustBase) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("ir change request msg not valid: %w", err)
	}
	bs, err := x.bytes()
	if err != nil {
		return fmt.Errorf("failed to marshal ir change request msg: %w", err)
	}
	if _, err := tb.VerifySignature(bs, x.Signature, x.Author); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	return nil
}

func (x *IrChangeReqMsg) bytes() ([]byte, error) {
	xCopy := *x
	xCopy.Signature = nil
	return types.Cbor.Marshal(xCopy)
}
