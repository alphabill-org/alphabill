package abdrc

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	abdrc "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/stretchr/testify/require"
)

func TestIrChangeReqMsg_SignAndVerifyOK(t *testing.T) {
	msg := &IrChangeReqMsg{
		Author: "test",
		IrChangeReq: &abdrc.IRChangeReq{
			Partition:  1,
			CertReason: abdrc.Quorum,
		},
	}
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	require.NoError(t, msg.Sign(signer))
	require.NotEmpty(t, msg.Signature)
	rootTrust := testtb.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"test": verifier})
	require.NoError(t, msg.Verify(rootTrust))
}

func TestIrChangeReqMsg_IsValid(t *testing.T) {
	type fields struct {
		Author      string
		IrChangeReq *abdrc.IRChangeReq
		Signature   []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr string
	}{
		{
			name: "no author",
			fields: fields{
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: abdrc.Quorum,
				},
			},
			wantErr: "author is missing",
		},
		{
			name: "request is nil",
			fields: fields{
				Author:      "test",
				IrChangeReq: nil,
			},
			wantErr: "request is nil",
		},
		{
			name: "unknown reason",
			fields: fields{
				Author: "test",
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: 10,
				},
			},
			wantErr: "request validation failed: unknown reason (10)",
		},
		{
			name: "invalid reason - timeout",
			fields: fields{
				Author: "test",
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: abdrc.T2Timeout,
				},
			},
			wantErr: "invalid reason, timeout can only be proposed by leader",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IrChangeReqMsg{
				Author:      tt.fields.Author,
				IrChangeReq: tt.fields.IrChangeReq,
				Signature:   tt.fields.Signature,
			}
			err := x.IsValid()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIrChangeReqMsg_Sign_Errors(t *testing.T) {
	s, _ := testsig.CreateSignerAndVerifier(t)

	type fields struct {
		Author      string
		IrChangeReq *abdrc.IRChangeReq
	}
	type args struct {
		signer crypto.Signer
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name: "signer is nil",
			fields: fields{
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: abdrc.Quorum,
				},
			},
			args:    args{signer: nil},
			wantErr: "signer is nil",
		},
		{
			name: "invalid request",
			fields: fields{
				Author: "test",
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: abdrc.T2Timeout,
				},
			},
			args:    args{signer: s},
			wantErr: "ir change request msg not valid: invalid reason, timeout can only be proposed by leader",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IrChangeReqMsg{
				Author:      tt.fields.Author,
				IrChangeReq: tt.fields.IrChangeReq,
			}
			require.ErrorContains(t, x.Sign(tt.args.signer), tt.wantErr)
		})
	}
}

func TestIrChangeReqMsg_Verify(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	tb := testtb.NewTrustBaseFromVerifiers(t, map[string]crypto.Verifier{"test": verifier})

	type fields struct {
		Author      string
		IrChangeReq *abdrc.IRChangeReq
		Signature   []byte
	}
	type args struct {
		rootTrust types.RootTrustBase
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name: "invalid request",
			fields: fields{
				Author: "test",
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: abdrc.T2Timeout,
				},
			},
			args:    args{rootTrust: tb},
			wantErr: "ir change request msg not valid: invalid reason, timeout can only be proposed by leader",
		},
		{
			name: "author not found",
			fields: fields{
				Author: "bar",
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: abdrc.Quorum,
				},
			},
			args:    args{rootTrust: tb},
			wantErr: `author 'bar' is not part of the trust base`,
		},
		{
			name: "verify error",
			fields: fields{
				Author: "test",
				IrChangeReq: &abdrc.IRChangeReq{
					Partition:  1,
					CertReason: abdrc.Quorum,
				},
				Signature: []byte{0, 1, 2, 3, 4, 5},
			},
			args:    args{rootTrust: tb},
			wantErr: "signature verification failed: verify bytes failed: signature length is 6 b (expected 64 b)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IrChangeReqMsg{
				Author:      tt.fields.Author,
				IrChangeReq: tt.fields.IrChangeReq,
				Signature:   tt.fields.Signature,
			}
			err := x.Verify(tt.args.rootTrust)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIrChangeReqMsg_bytes(t *testing.T) {
	msg := &IrChangeReqMsg{
		Author: "test",
		IrChangeReq: &abdrc.IRChangeReq{
			Partition:  1,
			CertReason: abdrc.T2Timeout,
		},
		Signature: []byte{1, 2, 4},
	}
	msgCopy := *msg
	msgCopy.Signature = nil
	expected, err := cbor.Marshal(msgCopy)
	require.NoError(t, err)
	msgBytes, err := msg.bytes()
	require.NoError(t, err)
	require.Equal(t, expected, msgBytes)
}
