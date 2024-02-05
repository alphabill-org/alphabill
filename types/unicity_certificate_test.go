package types

import (
	"bytes"
	gocrypto "crypto"
	"encoding/gob"
	"math"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/tree/imt"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestUnicityCertificate_IsValid(t *testing.T) {
	sdrh := zeroHash
	hasher := gocrypto.SHA256.New()
	leaf := UTData{
		SystemIdentifier:            identifier,
		InputRecord:                 ir,
		SystemDescriptionRecordHash: sdrh,
	}
	leaf.AddToHasher(hasher)
	dataHash := hasher.Sum(nil)
	var uct = &UnicityTreeCertificate{
		SystemIdentifier:      identifier,
		SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: dataHash}},
		SystemDescriptionHash: zeroHash,
	}
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	seal := &UnicitySeal{
		RootChainRoundNumber: 1,
		Timestamp:            NewTimestamp(),
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
		systemIdentifier      SystemID
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
				UnicityTreeCertificate: nil,
				UnicitySeal:            seal,
			},
			args: args{
				verifiers:             map[string]crypto.Verifier{"test": verifier},
				algorithm:             gocrypto.SHA256,
				systemIdentifier:      identifier,
				systemDescriptionHash: sdrh,
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
				systemDescriptionHash: sdrh,
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
				systemDescriptionHash: sdrh,
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
				systemDescriptionHash: sdrh,
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
	require.EqualValues(t, 0, uc.GetRoundNumber())
	require.EqualValues(t, 0, uc.GetRootRoundNumber())
	require.Nil(t, uc.GetStateHash())
	require.ErrorIs(t, uc.IsValid(nil, gocrypto.SHA256, 0, nil), ErrUnicityCertificateIsNil)
}

func TestUnicityCertificate_IRNil(t *testing.T) {
	uc := &UnicityCertificate{
		UnicityTreeCertificate: &UnicityTreeCertificate{},
		UnicitySeal:            &UnicitySeal{},
	}
	require.EqualValues(t, 0, uc.GetRoundNumber())
	require.EqualValues(t, 0, uc.GetRootRoundNumber())
	require.Nil(t, uc.GetStateHash())
	require.EqualError(t, uc.IsValid(nil, gocrypto.SHA256, 0, nil), "unicity seal validation failed, root node info is missing")
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
	require.EqualValues(t, []byte{0, 0, 2}, uc.GetStateHash())
	// everything is equal, this is the same UC and not repeat
	require.False(t, isRepeat(uc, uc))
	ruc := &UnicityCertificate{
		InputRecord: uc.InputRecord.NewRepeatIR(),
		UnicitySeal: &UnicitySeal{
			RootChainRoundNumber: uc.UnicitySeal.RootChainRoundNumber + 1,
		},
	}
	require.True(t, ruc.IsRepeat(uc))
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

func TestSerialize(t *testing.T) {
	uc := &UnicityCertificate{
		InputRecord: &InputRecord{
			PreviousHash:    []byte{0, 0, 1},
			Hash:            []byte{0, 0, 2},
			BlockHash:       []byte{0, 0, 3},
			SummaryValue:    []byte{0, 0, 4},
			RoundNumber:     6,
			SumOfEarnedFees: 20,
		},
		UnicityTreeCertificate: &UnicityTreeCertificate{
			SystemIdentifier:      identifier,
			SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: []byte{1, 2, 3}}},
			SystemDescriptionHash: []byte{1, 2, 3, 4},
		},
		UnicitySeal: &UnicitySeal{
			RootChainRoundNumber: 1,
			Timestamp:            9,
			PreviousHash:         []byte{1, 2, 3},
			Hash:                 []byte{2, 3, 4},
			Signatures:           map[string][]byte{"1": {1, 1, 1}},
		},
	}
	expectedBytes := []byte{
		0, 0, 1, // IR: previous hash
		0, 0, 2, // IR: hash
		0, 0, 3, // IR: block hash
		0, 0, 4, // IR: summary hash
		0, 0, 0, 0, 0, 0, 0, 6, // IR: round
		0, 0, 0, 0, 0, 0, 0, 20, // IR: sum of fees
		1, 1, 1, 1, // UT: identifier
		1, 1, 1, 1, 1, 2, 3, // UT: siblings key+hash
		1, 2, 3, 4, // UT: system description hash
		0, 0, 0, 0, 0, 0, 0, 1, // UC: root round
		0, 0, 0, 0, 0, 0, 0, 9, // UC: timestamp
		1, 2, 3, // UC: previous hash
		2, 3, 4, // UC: hash
		'1', 1, 1, 1, // UC: signature
	}
	require.EqualValues(t, expectedBytes, uc.Bytes())
}

func BenchmarkBytes(b *testing.B) {
	uc := &UnicityCertificate{
		InputRecord: &InputRecord{
			PreviousHash:    []byte{0, 0, 1},
			Hash:            []byte{0, 0, 2},
			BlockHash:       []byte{0, 0, 3},
			SummaryValue:    []byte{0, 0, 4},
			RoundNumber:     6,
			SumOfEarnedFees: 20,
		},
		UnicityTreeCertificate: &UnicityTreeCertificate{
			SystemIdentifier:      identifier,
			SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: make([]byte, 32)}},
			SystemDescriptionHash: make([]byte, 32),
		},
		UnicitySeal: &UnicitySeal{
			RootChainRoundNumber: 1,
			Timestamp:            math.MaxInt64,
			PreviousHash:         make([]byte, 32),
			Hash:                 make([]byte, 32),
			Signatures:           map[string][]byte{"test": make([]byte, 65)},
		},
	}
	b.ResetTimer()
	b.Run("serialize to bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hasher := gocrypto.SHA256.New()
			hasher.Write(uc.Bytes())
			_ = hasher.Sum(nil)
		}
	})
	b.Run("serialize to cbor", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hasher := gocrypto.SHA256.New()
			ucBytes, _ := cbor.Marshal(uc)
			hasher.Write(ucBytes)
			_ = hasher.Sum(nil)
		}
	})
	b.Run("serialize to gob", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var buf bytes.Buffer
			enc := gob.NewEncoder(&buf)
			hasher := gocrypto.SHA256.New()
			if err := enc.Encode(uc); err != nil {
				b.Fatal("encoding failed")
			}
			hasher.Write(buf.Bytes())
			_ = hasher.Sum(nil)
		}
	})
}
