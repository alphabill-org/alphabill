package abdrc

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/util"
	abdrc "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
)

type IrChangeReqMsg struct {
	_           struct{}           `cbor:",toarray"`
	Author      string             `json:"author,omitempty"`
	IrChangeReq *abdrc.IRChangeReq `json:"ir_change_req,omitempty"`
	Signature   []byte             `json:"signature,omitempty"`
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
	signature, err := signer.SignBytes(x.bytes())
	if err != nil {
		return fmt.Errorf("failed to sign ir change request: %w", err)
	}
	x.Signature = signature
	return nil
}

func (x *IrChangeReqMsg) Verify(rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("ir change request msg not valid: %w", err)
	}
	v, f := rootTrust[x.Author]
	if !f {
		return fmt.Errorf("author %q is not in the trustbase", x.Author)
	}
	if err := v.VerifyBytes(x.Signature, x.bytes()); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	return nil
}

func (x *IrChangeReqMsg) bytes() []byte {
	var b bytes.Buffer
	b.Write([]byte(x.Author))
	b.Write(x.IrChangeReq.SystemIdentifier.Bytes())
	b.Write(util.Uint32ToBytes(uint32(x.IrChangeReq.CertReason)))
	return b.Bytes()
}
