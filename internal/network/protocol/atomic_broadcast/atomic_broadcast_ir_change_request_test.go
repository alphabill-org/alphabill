package atomic_broadcast

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"

	"github.com/alphabill-org/alphabill/internal/certificates"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	certification "github.com/alphabill-org/alphabill/internal/network/protocol/certification"
)

type DummyPartitionStore struct {
	signer   abcrypto.Signer
	verifier abcrypto.Verifier
}

func NewDummyPartitionStore(t *testing.T) *DummyPartitionStore {
	s, v := testsig.CreateSignerAndVerifier(t)
	return &DummyPartitionStore{signer: s, verifier: v}
}

func (x *DummyPartitionStore) SignBytes(t *testing.T, data []byte) []byte {
	sig, err := x.signer.SignBytes(data)
	require.NoError(t, err)
	return sig
}

func (x *DummyPartitionStore) GetTrustBase(id protocol.SystemIdentifier) (map[string]abcrypto.Verifier, error) {
	return map[string]abcrypto.Verifier{"0": x.verifier}, nil
}

func TestIRChangeReqMsg_Bytes(t *testing.T) {
	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	tests := []struct {
		name   string
		fields fields
		want   []byte
	}{
		{
			name: "Bytes result",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 1},
						NodeIdentifier:   "1",
						RootRoundNumber:  9,
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
						},
						Signature: []byte{0, 1},
					},
				},
			},
			want: []byte{
				0, 0, 0, 1, // 4 byte System identifier of IRChangeReqMsg
				0, 0, 0, 1, // 4 byte system identifier
				0, 0, 0, 1, // Start of the BlockCertificationRequest
				'1', // string is encoded without '/0'
				0, 0, 0, 0, 0, 0, 0, 9,
				0, 1, 2, 3, 4, 5, 6, 7,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			if got := x.Bytes(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Bytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIRChangeReqMsg_IsValid(t *testing.T) {

	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		partitions PartitionStore
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "IR change request timeout is not valid",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 1},
						NodeIdentifier:   "1",
						RootRoundNumber:  9,
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
						},
						Signature: []byte{0, 1},
					},
				},
			},
			args:    args{partitions: NewDummyPartitionStore(t)},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			if err := x.IsValid(tt.args.partitions); (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
