package types

import (
	gocrypto "crypto"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/tree/imt"
	"github.com/stretchr/testify/require"
)

func TestUnicityCertificate_IsValid(t *testing.T) {
	sdrh := zeroHash
	hasher := gocrypto.SHA256.New()
	leaf := UnicityTreeData{
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
	t.Run("err - previous UC is nil", func(t *testing.T) {
		var prevUC *UnicityCertificate = nil
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		require.ErrorIs(t, CheckNonEquivocatingCertificates(prevUC, newUC), errLastUCIsNil)
	})
	t.Run("err - new UC is nil", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		var newUC *UnicityCertificate = nil
		require.ErrorIs(t, CheckNonEquivocatingCertificates(prevUC, newUC), errUCIsNil)
	})
	t.Run("equal UC's", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		require.NoError(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
	t.Run("equal round, different UC", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 5},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		require.EqualError(t, CheckNonEquivocatingCertificates(prevUC, newUC), "equivocating UC, different input records for same partition round 6")
	})
	t.Run("new is older partition round", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     5,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		require.EqualError(t, CheckNonEquivocatingCertificates(prevUC, newUC), "new certificate is from older partition round 5 than previous certificate 6")
	})
	t.Run("new is older root round", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 9},
		}
		require.EqualError(t, CheckNonEquivocatingCertificates(prevUC, newUC), "new certificate is from older root round 9 than previous certificate 10")
	})
	t.Run("round gap, new is 0H block repeating the same state as last seen", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     8,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
		}
		require.EqualError(t, CheckNonEquivocatingCertificates(prevUC, newUC), "new UC extends state hash, new state hash does not change, but block is not empty")
	})
	t.Run("ok - new empty block UC", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 0},
				SummaryValue:    []byte{0, 0, 3},
				RoundNumber:     7,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
		}
		require.NoError(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
	t.Run("ok - repeat UC of empty block", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 0},
				SummaryValue:    []byte{0, 0, 3},
				RoundNumber:     6,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 0},
				SummaryValue:    []byte{0, 0, 3},
				RoundNumber:     9,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 18},
		}
		require.NoError(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
	t.Run("ok - too far apart to compare", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 2},
				SummaryValue:    []byte{0, 0, 3},
				RoundNumber:     6,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{3, 3, 3},
				Hash:            []byte{4, 4, 4},
				BlockHash:       []byte{0, 0, 0},
				SummaryValue:    []byte{0, 0, 3},
				RoundNumber:     9,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 18},
		}
		require.NoError(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
	t.Run("ok - normal progress", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 3},
				BlockHash:       []byte{0, 0, 4},
				SummaryValue:    []byte{0, 0, 6},
				RoundNumber:     7,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
		}
		require.NoError(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
	t.Run("error - state changes, but block hash is 0h", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 3},
				BlockHash:       []byte{0, 0, 0},
				SummaryValue:    []byte{0, 0, 6},
				RoundNumber:     7,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
		}
		require.Error(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
	t.Run("ok - repeat UC of previous state (skipping some repeats between)", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 3},
				BlockHash:       []byte{0, 0, 2},
				SummaryValue:    []byte{0, 0, 6},
				RoundNumber:     8,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 16},
		}
		require.NoError(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
	t.Run("err - block hash repeats on normal progress", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 2},
				Hash:            []byte{0, 0, 3},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 6},
				RoundNumber:     7,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
		}
		require.EqualError(t, CheckNonEquivocatingCertificates(prevUC, newUC), "new certificate repeats previous block hash")
	})
	t.Run("err - next round does not extend from previous state", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 3},
				BlockHash:       []byte{0, 0, 4},
				SummaryValue:    []byte{0, 0, 6},
				RoundNumber:     7,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
		}
		require.EqualError(t, CheckNonEquivocatingCertificates(prevUC, newUC), "new certificate does not extend previous state hash")
	})
	t.Run("err - extending from same state, but ", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 3},
				BlockHash:       []byte{0, 0, 4},
				SummaryValue:    []byte{0, 0, 6},
				RoundNumber:     7,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 11},
		}
		require.EqualError(t, CheckNonEquivocatingCertificates(prevUC, newUC), "new certificate does not extend previous state hash")
	})
	t.Run("ok - too far apart to compare (can only check block hash)", func(t *testing.T) {
		prevUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 1},
				Hash:            []byte{0, 0, 2},
				BlockHash:       []byte{0, 0, 3},
				SummaryValue:    []byte{0, 0, 4},
				RoundNumber:     6,
				SumOfEarnedFees: 2,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 10},
		}
		newUC := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash:    []byte{0, 0, 5},
				Hash:            []byte{0, 0, 6},
				BlockHash:       []byte{0, 0, 4},
				SummaryValue:    []byte{0, 0, 2},
				RoundNumber:     9,
				SumOfEarnedFees: 1,
			},
			UnicitySeal: &UnicitySeal{RootChainRoundNumber: 15},
		}
		require.NoError(t, CheckNonEquivocatingCertificates(prevUC, newUC))
	})
}

func TestUCHash(t *testing.T) {
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
	// serialize manually
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
	expectedHash := sha256.Sum256(expectedBytes)
	require.EqualValues(t, expectedHash[:], uc.Hash(gocrypto.SHA256))
}

func TestUnicityCertificate_GetPreviousStateHash(t *testing.T) {
	t.Run("UC is nil", func(t *testing.T) {
		var x *UnicityCertificate = nil
		require.Nil(t, x.GetPreviousStateHash())
	})
	t.Run("IR is nil", func(t *testing.T) {
		x := &UnicityCertificate{}
		require.Nil(t, x.GetPreviousStateHash())
	})
	t.Run("hash value", func(t *testing.T) {
		x := &UnicityCertificate{
			InputRecord: &InputRecord{
				PreviousHash: []byte{1, 2, 3},
			},
		}
		require.Equal(t, []byte{1, 2, 3}, x.GetPreviousStateHash())
	})
}

func TestUnicityCertificate_GetFeeSum(t *testing.T) {
	t.Run("UC is nil", func(t *testing.T) {
		var x *UnicityCertificate = nil
		require.EqualValues(t, 0, x.GetFeeSum())
	})
	t.Run("IR is nil", func(t *testing.T) {
		x := &UnicityCertificate{}
		require.EqualValues(t, 0, x.GetFeeSum())
	})
	t.Run("hash value", func(t *testing.T) {
		x := &UnicityCertificate{
			InputRecord: &InputRecord{
				SumOfEarnedFees: 10,
			},
		}
		require.EqualValues(t, 10, x.GetFeeSum())
	})
}

func TestUnicityCertificate_GetSummaryValue(t *testing.T) {
	t.Run("UC is nil", func(t *testing.T) {
		var x *UnicityCertificate = nil
		require.Nil(t, x.GetSummaryValue())
	})
	t.Run("IR is nil", func(t *testing.T) {
		x := &UnicityCertificate{}
		require.Nil(t, x.GetSummaryValue())
	})
	t.Run("hash value", func(t *testing.T) {
		x := &UnicityCertificate{
			InputRecord: &InputRecord{
				SummaryValue: []byte{1, 2, 3},
			},
		}
		require.EqualValues(t, []byte{1, 2, 3}, x.GetSummaryValue())
	})
}

func TestUnicityCertificate_GetRootRoundNumber(t *testing.T) {
	t.Run("UC is nil", func(t *testing.T) {
		var x *UnicityCertificate = nil
		require.EqualValues(t, 0, x.GetRootRoundNumber())
	})
	t.Run("UC seal is nil", func(t *testing.T) {
		x := &UnicityCertificate{}
		require.EqualValues(t, 0, x.GetRootRoundNumber())
	})
	t.Run("returns round", func(t *testing.T) {
		x := &UnicityCertificate{
			UnicitySeal: &UnicitySeal{
				RootChainRoundNumber: 1,
			}}
		require.EqualValues(t, 1, x.GetRootRoundNumber())
	})
}
