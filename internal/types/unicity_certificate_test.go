package types

import (
	gocrypto "crypto"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestUnicityCertificate_IsValid(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            util.MakeTimestamp(),
		PreviousHash:         zeroHash,
		Hash:                 zeroHash,
	}
	err := seal.Sign("test", signer)
	require.NoError(t, err)
	type fields struct {
		InputRecord            *InputRecord
		UnicityTreeCertificate *UnicityTreeCertificate
		UnicitySeal            *UnicitySeal
	}
	type args struct {
		verifiers             map[string]crypto.Verifier
		algorithm             gocrypto.Hash
		systemIdentifier      []byte
		systemDescriptionHash []byte
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		err    error
		errStr string
	}{
		{
			name: "invalid input record",
			fields: fields{
				InputRecord:            nil,
				UnicityTreeCertificate: uct,
				UnicitySeal:            seal,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			err: ErrInputRecordIsNil,
		},
		{
			name: "invalid uct",
			fields: fields{
				InputRecord:            ir,
				UnicityTreeCertificate: nil,
				UnicitySeal:            seal,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			err: ErrUnicityTreeCertificateIsNil,
		},
		{
			name: "invalid unicity seal",
			fields: fields{
				InputRecord:            ir,
				UnicityTreeCertificate: uct,
				UnicitySeal:            nil,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			err: ErrUnicitySealIsNil,
		},
		{
			name: "invalid root hash",
			fields: fields{
				InputRecord:            ir,
				UnicityTreeCertificate: uct,
				UnicitySeal:            seal,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: zeroHash,
			},
			errStr: "does not match with the root hash of the unicity tree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &UnicityCertificate{
				InputRecord:            tt.fields.InputRecord,
				UnicityTreeCertificate: tt.fields.UnicityTreeCertificate,
				UnicitySeal:            tt.fields.UnicitySeal,
			}

			err := uc.IsValid(tt.args.verifiers, tt.args.algorithm, tt.args.systemIdentifier, tt.args.systemDescriptionHash)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.errStr))
			}
		})
	}
}

func TestUnicityCertificate_UCIsNil(t *testing.T) {
	var uc *UnicityCertificate
	require.ErrorIs(t, uc.IsValid(nil, gocrypto.SHA256, nil, nil), ErrUnicityCertificateIsNil)
}

func TestUnicityCertificate_isRepeat(t *testing.T) {
	uc := &UnicityCertificate{
		InputRecord: &InputRecord{
			PreviousHash:    []byte{0, 0, 1},
			Hash:            []byte{0, 0, 2},
			BlockHash:       []byte{0, 0, 3},
			SummaryValue:    []byte{0, 0, 4},
			RoundNumber:     6,
			SumOfEarnedFees: 20,
		},
	}
	// everything is equal, this is the same UC and not repeat
	require.False(t, isRepeat(uc, uc))
	ruc := &UnicityCertificate{
		InputRecord: uc.InputRecord.NewRepeatUC(),
	}
	// now it is repeat of previous round
	require.True(t, isRepeat(uc, ruc))
	ruc.InputRecord.RoundNumber++
	// still is considered a repeat uc
	require.True(t, isRepeat(uc, ruc))
	// if anything else changes, it is no longer considered repeat
	require.False(t, isRepeat(uc, &UnicityCertificate{
		InputRecord: &InputRecord{
			Hash:            []byte{0, 0, 2},
			BlockHash:       []byte{0, 0, 3},
			SummaryValue:    []byte{0, 0, 4},
			RoundNumber:     6,
			SumOfEarnedFees: 20,
		},
	}))
	require.False(t, isRepeat(uc, &UnicityCertificate{
		InputRecord: &InputRecord{
			PreviousHash:    []byte{0, 0, 1},
			Hash:            []byte{0, 0, 2},
			BlockHash:       []byte{0, 0, 3},
			SummaryValue:    []byte{0, 0, 4},
			RoundNumber:     6,
			SumOfEarnedFees: 2,
		},
	}))
	// also not if order is opposite
	require.False(t, isRepeat(ruc, uc))
}
