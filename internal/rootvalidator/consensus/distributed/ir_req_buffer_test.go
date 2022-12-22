package distributed

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
)

func Test_compareIR(t *testing.T) {
	type args struct {
		a *certificates.InputRecord
		b *certificates.InputRecord
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "equal",
			args: args{
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
			},
			want: true,
		},
		{
			name: "Previous hash not equal",
			args: args{
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
			},
			want: false,
		},
		{
			name: "Hash not equal",
			args: args{
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2, 3},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
			},
			want: false,
		},
		{
			name: "Block hash not equal",
			args: args{
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    nil,
					SummaryValue: []byte{4, 4, 4}},
			},
			want: false,
		},
		{
			name: "Summary value not equal",
			args: args{
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareIR(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("compareIR() = %v, want %v", got, tt.want)
			}
		})
	}
}
