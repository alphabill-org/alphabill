package abdrc

import (
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	abdrc "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/testutils/sig"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestIrChangeReqMsg_SignAndVerifyOK(t *testing.T) {
	msg := &IrChangeReqMsg{
		Author: "test",
		IrChangeReq: &abdrc.IRChangeReq{
			SystemIdentifier: types.SystemID32(1),
			CertReason:       abdrc.Quorum,
		},
	}
	signer, ver := testsig.CreateSignerAndVerifier(t)
	require.NoError(t, msg.Sign(signer))
	require.NotEmpty(t, msg.Signature)
	rootTrust := map[string]crypto.Verifier{"test": ver}
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
					SystemIdentifier: types.SystemID32(1),
					CertReason:       abdrc.Quorum,
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
					SystemIdentifier: types.SystemID32(1),
					CertReason:       10,
				},
			},
			wantErr: "request validation failed: unknown reason 10",
		},
		{
			name: "invalid reason - timeout",
			fields: fields{
				Author: "test",
				IrChangeReq: &abdrc.IRChangeReq{
					SystemIdentifier: types.SystemID32(1),
					CertReason:       abdrc.T2Timeout,
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
					SystemIdentifier: types.SystemID32(1),
					CertReason:       abdrc.Quorum,
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
					SystemIdentifier: types.SystemID32(1),
					CertReason:       abdrc.T2Timeout,
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
	_, ver := testsig.CreateSignerAndVerifier(t)
	tb := map[string]crypto.Verifier{"test": ver}

	type fields struct {
		Author      string
		IrChangeReq *abdrc.IRChangeReq
		Signature   []byte
	}
	type args struct {
		rootTrust map[string]crypto.Verifier
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
					SystemIdentifier: types.SystemID32(1),
					CertReason:       abdrc.T2Timeout,
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
					SystemIdentifier: types.SystemID32(1),
					CertReason:       abdrc.Quorum,
				},
			},
			args:    args{rootTrust: tb},
			wantErr: `author "bar" is not in the trustbase`,
		},
		{
			name: "verify error",
			fields: fields{
				Author: "test",
				IrChangeReq: &abdrc.IRChangeReq{
					SystemIdentifier: types.SystemID32(1),
					CertReason:       abdrc.Quorum,
				},
				Signature: []byte{0, 1, 2, 3, 4, 5},
			},
			args:    args{rootTrust: tb},
			wantErr: "signature verification failed: signature length is 6",
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
			SystemIdentifier: types.SystemID32(1),
			CertReason:       abdrc.T2Timeout,
		},
		Signature: []byte{1, 2, 4},
	}
	var expected = []byte{'t', 'e', 's', 't', 0, 0, 0, 1, 0, 0, 0, 2}
	require.Equal(t, expected, msg.bytes())
}
