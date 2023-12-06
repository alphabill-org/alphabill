package types

import (
	gocrypto "crypto"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/testutils/sig"
	"github.com/alphabill-org/alphabill/util"
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
		UnicitySeal: &UnicitySeal{
			RootChainRoundNumber: 1,
		},
	}
	// everything is equal, this is the same UC and not repeat
	require.False(t, isRepeat(uc, uc))
	ruc := &UnicityCertificate{
		InputRecord: uc.InputRecord.NewRepeatIR(),
		UnicitySeal: &UnicitySeal{
			RootChainRoundNumber: uc.UnicitySeal.RootChainRoundNumber + 1,
		},
	}
	// now it is repeat of previous round
	require.True(t, isRepeat(uc, ruc))
	ruc.UnicitySeal.RootChainRoundNumber++
	// still is considered a repeat uc
	require.True(t, isRepeat(uc, ruc))
	// with incremented round number, not a repeat uc
	ruc.InputRecord.RoundNumber++
	require.False(t, isRepeat(uc, ruc))
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

func TestCheckNonEquivocatingCertificates(t *testing.T) {
	type args struct {
		prevUC *UnicityCertificate
		newUC  *UnicityCertificate
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "err - previous UC is nil",
			args: args{
				prevUC: nil,
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
			},
			wantErrStr: errLastUCIsNil.Error(),
		},
		{
			name: "err - new UC is nil",
			args: args{
				newUC: nil,
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
			},
			wantErrStr: errUCIsNil.Error(),
		},
		{
			name: "equal UC's",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
			},
		},
		{
			name: "equal round, different UC",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 5},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
			},
			wantErrStr: "equivocating UC, different input records for same partition round",
		},
		{
			name: "new is older partition round",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     5,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
			},
			wantErrStr: "new certificate is from older partition round",
		},
		{
			name: "new is older root round",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 9},
				},
			},
			wantErrStr: "new certificate is from older root round",
		},
		{
			name: "no state change, but block hash is not 0h",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     7,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
				},
			},
			wantErrStr: "invalid new certificate, non-empty block, but state hash does not change",
		},
		{
			name: "ok - new empty block UC",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 0},
						SummaryValue:    []byte{0, 0, 3},
						RoundNumber:     7,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
				},
			},
		},
		{
			name: "ok - repeat UC of empty block",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 0},
						SummaryValue:    []byte{0, 0, 3},
						RoundNumber:     6,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 0},
						SummaryValue:    []byte{0, 0, 3},
						RoundNumber:     9,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 18},
				},
			},
		},
		{
			name: "ok - too far apart to compare",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 2},
						SummaryValue:    []byte{0, 0, 3},
						RoundNumber:     6,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 0},
						SummaryValue:    []byte{0, 0, 3},
						RoundNumber:     9,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 18},
				},
			},
		},
		{
			name: "ok - normal progress",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 3},
						BlockHash:       []byte{0, 0, 4},
						SummaryValue:    []byte{0, 0, 6},
						RoundNumber:     7,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
				},
			},
		},
		{
			name: "ok - state changes, but block hash is 0h", // AB-1002
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 3},
						BlockHash:       []byte{0, 0, 0},
						SummaryValue:    []byte{0, 0, 6},
						RoundNumber:     7,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
				},
			},
		},
		{
			name: "ok - repeat UC of previous state (skipping some repeats between)",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 3},
						BlockHash:       []byte{0, 0, 2},
						SummaryValue:    []byte{0, 0, 6},
						RoundNumber:     8,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 16},
				},
			},
		},
		{
			name: "err - block hash repeats on normal progress",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 2},
						Hash:            []byte{0, 0, 3},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 6},
						RoundNumber:     7,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
				},
			},
			wantErrStr: "new certificate repeats previous block hash",
		},
		{
			name: "err - next round does not extend from previous state",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 3},
						BlockHash:       []byte{0, 0, 4},
						SummaryValue:    []byte{0, 0, 6},
						RoundNumber:     7,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
				},
			},
			wantErrStr: "new certificate does not extend previous state hash",
		},
		{
			name: "ok - too far apart to compare (can only check block hash)",
			args: args{
				prevUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 1},
						Hash:            []byte{0, 0, 2},
						BlockHash:       []byte{0, 0, 3},
						SummaryValue:    []byte{0, 0, 4},
						RoundNumber:     6,
						SumOfEarnedFees: 2,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
				},
				newUC: &UnicityCertificate{
					InputRecord: &InputRecord{
						PreviousHash:    []byte{0, 0, 5},
						Hash:            []byte{0, 0, 6},
						BlockHash:       []byte{0, 0, 4},
						SummaryValue:    []byte{0, 0, 2},
						RoundNumber:     9,
						SumOfEarnedFees: 1,
					},
					UnicitySeal: &UnicitySeal{RootChainRoundNumber: 15},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckNonEquivocatingCertificates(tt.args.prevUC, tt.args.newUC)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
