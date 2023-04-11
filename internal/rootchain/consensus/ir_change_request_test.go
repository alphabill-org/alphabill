package consensus

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	test "github.com/alphabill-org/alphabill/internal/testutils"
)

func TestCheckBlockCertificationRequest(t *testing.T) {
	type args struct {
		req *certification.BlockCertificationRequest
		luc *certificates.UnicityCertificate
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "req nil",
			args:    args{req: nil, luc: nil},
			wantErr: true,
		},
		{
			name: "luc nil",
			args: args{
				req: &certification.BlockCertificationRequest{
					InputRecord: &certificates.InputRecord{
						RoundNumber: 1,
					},
				},
				luc: nil,
			},
			wantErr: true,
		},
		{
			name: "invalid partition round",
			args: args{
				req: &certification.BlockCertificationRequest{
					InputRecord: &certificates.InputRecord{
						RoundNumber: 1,
					},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{
						RoundNumber: 1,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "hash mismatch",
			args: args{
				req: &certification.BlockCertificationRequest{
					InputRecord: &certificates.InputRecord{
						PreviousHash: test.RandomBytes(32),
						RoundNumber:  2,
					},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{
						RoundNumber: 1,
						Hash:        test.RandomBytes(32),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "ok",
			args: args{
				req: &certification.BlockCertificationRequest{
					InputRecord: &certificates.InputRecord{
						PreviousHash: make([]byte, 32),
						RoundNumber:  2,
					},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{
						RoundNumber: 1,
						Hash:        make([]byte, 32),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckBlockCertificationRequest(tt.args.req, tt.args.luc); (err != nil) != tt.wantErr {
				t.Errorf("CheckBlockCertificationRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
