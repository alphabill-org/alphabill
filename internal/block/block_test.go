package block

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

type alwaysOkValidator struct {
}

type alwaysNokValidator struct {
}

func (v *alwaysOkValidator) Validate(_ *certificates.UnicityCertificate) error {
	return nil
}

var genericUCError = errors.New("some uc error")

func (v *alwaysNokValidator) Validate(_ *certificates.UnicityCertificate) error {
	return genericUCError
}

func TestBlock_GetRoundNumber(t *testing.T) {
	var block *Block = nil
	// if Block is nil it returns 0. 0 block does not exist genesis is 1
	require.Equal(t, uint64(0), block.GetRoundNumber())
	block = &Block{
		UnicityCertificate: nil,
	}
	// If something is missing 0 is returned
	require.Equal(t, uint64(0), block.GetRoundNumber())
	block = &Block{
		UnicityCertificate: &certificates.UnicityCertificate{
			InputRecord: nil,
		},
	}
	require.Equal(t, uint64(0), block.GetRoundNumber())
	// otherwise round is read from Input record
	block = &Block{
		UnicityCertificate: &certificates.UnicityCertificate{
			InputRecord: &certificates.InputRecord{RoundNumber: 2},
		},
	}
	require.Equal(t, uint64(2), block.GetRoundNumber())
}

func TestBlock_IsValid(t *testing.T) {
	type fields struct {
		SystemIdentifier   []byte
		PreviousBlockHash  []byte
		NodeIdentifier     string
		Transactions       []*txsystem.Transaction
		UnicityCertificate *certificates.UnicityCertificate
	}
	type args struct {
		v CertificateValidator
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name: "system id nil",
			fields: fields{
				SystemIdentifier:   nil,
				PreviousBlockHash:  []byte{1, 2, 3},
				NodeIdentifier:     "test",
				Transactions:       []*txsystem.Transaction{},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
			args: args{
				v: &alwaysOkValidator{},
			},
			wantErr: ErrSystemIdIsNil.Error(),
		},
		{
			name: "prev block hash is nil",
			fields: fields{
				SystemIdentifier:   []byte{1, 2, 3},
				PreviousBlockHash:  nil,
				NodeIdentifier:     "test",
				Transactions:       []*txsystem.Transaction{},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
			args: args{
				v: &alwaysOkValidator{},
			},
			wantErr: ErrPrevBlockHashIsNil.Error(),
		},
		/* Todo: AB-845
		{
			name: "block proposer node id is missing",
			fields: fields{
				SystemIdentifier:   []byte{1, 2, 3},
				PreviousBlockHash:  []byte{1, 2, 3},
				Transactions:       []*txsystem.Transaction{},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
			args: args{
				v: &alwaysOkValidator{},
			},
			wantErr: ErrBlockProposerIdIsMissing.Error(),
		},
		*/
		{
			name: "transactions array is nil",
			fields: fields{
				SystemIdentifier:   []byte{1, 2, 3},
				PreviousBlockHash:  []byte{1, 2, 3},
				NodeIdentifier:     "test",
				Transactions:       nil,
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
			args: args{
				v: &alwaysOkValidator{},
			},
			wantErr: ErrTransactionsIsNil.Error(),
		},
		{
			name: "system id nil",
			fields: fields{
				SystemIdentifier:   []byte{1, 2, 3},
				PreviousBlockHash:  []byte{1, 2, 3},
				NodeIdentifier:     "test",
				Transactions:       []*txsystem.Transaction{},
				UnicityCertificate: nil,
			},
			args: args{
				v: &alwaysOkValidator{},
			},
			wantErr: ErrUCIsNil.Error(),
		},
		{
			name: "UC verification fails",
			fields: fields{
				SystemIdentifier:   []byte{1, 2, 3},
				PreviousBlockHash:  []byte{1, 2, 3},
				NodeIdentifier:     "test",
				Transactions:       []*txsystem.Transaction{},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
			args: args{
				v: &alwaysNokValidator{},
			},
			wantErr: "unicity certificate validation failed, some uc error",
		},
		{
			name: "ok",
			fields: fields{
				SystemIdentifier:   []byte{1, 2, 3},
				PreviousBlockHash:  []byte{1, 2, 3},
				NodeIdentifier:     "test",
				Transactions:       []*txsystem.Transaction{},
				UnicityCertificate: &certificates.UnicityCertificate{},
			},
			args: args{
				v: &alwaysOkValidator{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &Block{
				SystemIdentifier:   tt.fields.SystemIdentifier,
				PreviousBlockHash:  tt.fields.PreviousBlockHash,
				NodeIdentifier:     tt.fields.NodeIdentifier,
				Transactions:       tt.fields.Transactions,
				UnicityCertificate: tt.fields.UnicityCertificate,
			}
			err := x.IsValid(tt.args.v)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
