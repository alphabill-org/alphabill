package certificates

import (
	gocrypto "crypto"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	abhash "github.com/alphabill-org/alphabill/internal/hash"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var zeroHash = make([]byte, 32)

type Option func(info *RootRoundInfo)

func WithParentRound(round uint64) Option {
	return func(info *RootRoundInfo) {
		info.ParentRoundNumber = round
	}
}

func WithTimestamp(time uint64) Option {
	return func(info *RootRoundInfo) {
		info.Timestamp = time
	}
}

func NewDummyRootRoundInfo(round uint64, options ...Option) *RootRoundInfo {
	voteInfo := &RootRoundInfo{RoundNumber: round, Epoch: 0,
		Timestamp: 1670314583523, ParentRoundNumber: round - 1, CurrentRootHash: []byte{0, 1, 3}}
	for _, o := range options {
		o(voteInfo)
	}
	return voteInfo
}

func NewDummyCommitInfo(algo gocrypto.Hash, voteInfo *RootRoundInfo, rootHash []byte) *CommitInfo {
	hash := voteInfo.Hash(algo)
	return &CommitInfo{RootRoundInfoHash: hash, RootHash: rootHash}
}

func TestUnicitySeal_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)

	tests := []struct {
		name     string
		seal     *UnicitySeal
		verifier map[string]crypto.Verifier
		wantErr  string
	}{
		{
			name:     "seal is nil",
			seal:     nil,
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrUnicitySealIsNil.Error(),
		},
		{
			name:     "no root nodes",
			seal:     &UnicitySeal{},
			verifier: nil,
			wantErr:  ErrRootValidatorInfoMissing.Error(),
		},
		{
			name: "Root round info is nil",
			seal: &UnicitySeal{
				RootRoundInfo: nil,
				CommitInfo:    &CommitInfo{},
				Signatures:    map[string][]byte{"": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrRootRoundInfoIsNil.Error(),
		},
		// todo: AB-871 PreviousHash should be removed, it is not compatible with DRC and not used for anything anyway
		/*
			{
				name: "PreviousHash is nil",
				seal: &UnicitySeal{
					RootChainRoundNumber: 1,
					PreviousHash:         nil,
					Hash:                 zeroHash,
					Signatures:           map[string][]byte{"": zeroHash},
				},
				verifier: map[string]crypto.Verifier{"test": verifier},
				wantErr:  ErrUnicitySealPreviousHashIsNil,
			},
		*/
		{
			name: "Commit info is nil",
			seal: &UnicitySeal{
				RootRoundInfo: NewDummyRootRoundInfo(1),
				CommitInfo:    nil,
				Signatures:    map[string][]byte{"": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrCommitInfoIsNil.Error(),
		},
		{
			name: "Commit info is invalid",
			seal: &UnicitySeal{
				RootRoundInfo: NewDummyRootRoundInfo(1),
				CommitInfo:    &CommitInfo{RootRoundInfoHash: nil, RootHash: zeroHash},
				Signatures:    map[string][]byte{"": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrInvalidRootInfoHash.Error(),
		},
		{
			name: "Root round info hash is invalid",
			seal: &UnicitySeal{
				RootRoundInfo: NewDummyRootRoundInfo(1),
				CommitInfo:    &CommitInfo{RootRoundInfoHash: []byte{0, 1, 2}, RootHash: zeroHash},
				Signatures:    nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrCommitInfoRoundInfoHash.Error(),
		},
		{
			name: "Signature is nil",
			seal: &UnicitySeal{
				RootRoundInfo: NewDummyRootRoundInfo(1),
				CommitInfo:    NewDummyCommitInfo(gocrypto.SHA256, NewDummyRootRoundInfo(1), []byte{0, 1, 2}),
				Signatures:    nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  "unicity seal is missing signature",
		},
		{
			name: "Round creation time is 0",
			seal: &UnicitySeal{
				RootRoundInfo: NewDummyRootRoundInfo(1, WithTimestamp(0)),
				CommitInfo:    NewDummyCommitInfo(gocrypto.SHA256, NewDummyRootRoundInfo(1, WithTimestamp(0)), []byte{0, 1, 2}),
				Signatures:    map[string][]byte{"test": zeroHash},
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrRoundCreationTimeNotSet.Error(),
		},
		{
			name: "block number is invalid is nil",
			seal: &UnicitySeal{
				RootRoundInfo: NewDummyRootRoundInfo(0, WithParentRound(0)),
				CommitInfo:    NewDummyCommitInfo(gocrypto.SHA256, NewDummyRootRoundInfo(1, WithParentRound(0)), []byte{0, 1, 2}),
				Signatures:    nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  ErrRootInfoInvalidRound.Error(),
		},
		{
			name: "Timestamp is missing",
			seal: &UnicitySeal{
				RootRoundInfo: NewDummyRootRoundInfo(1, WithTimestamp(0)),
				CommitInfo:    NewDummyCommitInfo(gocrypto.SHA256, NewDummyRootRoundInfo(1, WithTimestamp(0)), []byte{0, 1, 2}),
				Signatures:    nil,
			},
			verifier: map[string]crypto.Verifier{"test": verifier},
			wantErr:  "round creation time not set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, tt.seal.IsValid(tt.verifier), tt.wantErr)
		})
	}
}

func TestSign_InvalidCommitInfo(t *testing.T) {
	signer, _ := testsig.CreateSignerAndVerifier(t)
	roundInfo := NewDummyRootRoundInfo(1, WithTimestamp(util.MakeTimestamp()))
	seal := &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: nil, RootHash: zeroHash},
	}
	err := seal.Sign("test", signer)
	require.ErrorIs(t, err, ErrInvalidRootInfoHash)
	seal = &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: []byte{1, 2, 3}, RootHash: nil},
	}
	err = seal.Sign("test", signer)
	require.ErrorIs(t, err, ErrUnicitySealHashIsNil)
}

func TestIsValid_InvalidSignature(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	roundInfo := NewDummyRootRoundInfo(1)
	seal := &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: roundInfo.Hash(gocrypto.SHA256), RootHash: zeroHash},
		Signatures:    map[string][]byte{"test": zeroHash},
	}
	verifiers := map[string]crypto.Verifier{"test": verifier}

	err := seal.IsValid(verifiers)
	require.True(t, strings.Contains(err.Error(), "unicity seal signature verification failed"))
}

func TestSignAndVerify_Ok(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	roundInfo := NewDummyRootRoundInfo(1)
	seal := &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: roundInfo.Hash(gocrypto.SHA256), RootHash: zeroHash},
	}
	err := seal.Sign("test", signer)
	require.NoError(t, err)
	verifiers := map[string]crypto.Verifier{"test": verifier}
	err = seal.verify(verifiers)
	require.NoError(t, err)
}
func TestVerify_SignatureIsNil(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	roundInfo := NewDummyRootRoundInfo(1)
	seal := &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: roundInfo.Hash(gocrypto.SHA256), RootHash: zeroHash},
	}
	verifiers := map[string]crypto.Verifier{"test": verifier}
	err := seal.verify(verifiers)
	require.ErrorContains(t, err, "unicity seal is missing signature")
}

func TestVerify_SignatureUnknownSigner(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	roundInfo := NewDummyRootRoundInfo(1)
	seal := &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: roundInfo.Hash(gocrypto.SHA256), RootHash: zeroHash},
		Signatures:    map[string][]byte{"test": zeroHash},
	}
	verifiers := map[string]crypto.Verifier{"xxx": verifier}
	err := seal.verify(verifiers)
	require.ErrorIs(t, err, ErrUnknownSigner)
}

func TestSign_SignerIsNil(t *testing.T) {
	roundInfo := NewDummyRootRoundInfo(1)
	seal := &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: roundInfo.Hash(gocrypto.SHA256), RootHash: zeroHash},
	}
	err := seal.Sign("test", nil)
	require.ErrorIs(t, err, ErrSignerIsNil)
}

func TestVerify_VerifierIsNil(t *testing.T) {
	roundInfo := NewDummyRootRoundInfo(1)
	seal := &UnicitySeal{
		RootRoundInfo: roundInfo,
		CommitInfo:    &CommitInfo{RootRoundInfoHash: roundInfo.Hash(gocrypto.SHA256), RootHash: zeroHash},
		Signatures:    map[string][]byte{"": zeroHash},
	}
	err := seal.verify(nil)
	require.ErrorIs(t, err, ErrRootValidatorInfoMissing)
}

func TestRootRoundInfo_IsValid(t *testing.T) {
	type fields struct {
		RoundNumber       uint64
		Epoch             uint64
		Timestamp         uint64
		ParentRoundNumber uint64
		CurrentRootHash   []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
		errIs   error
	}{
		{
			name: "valid",
			fields: fields{
				RoundNumber:       23,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   []byte{0, 1, 2, 5},
			},
			wantErr: false,
			errIs:   nil,
		},
		{
			name: "Parent round must be strictly smaller than current round",
			fields: fields{
				RoundNumber:       22,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   []byte{0, 1, 2, 5},
			},
			wantErr: true,
			errIs:   ErrRootInfoInvalidRound,
		},
		{
			name: "current root hash is nil",
			fields: fields{
				RoundNumber:       23,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   nil,
			},
			wantErr: true,
			errIs:   ErrInvalidRootRoundInfoHash,
		},
		{
			name: "current root hash is empty",
			fields: fields{
				RoundNumber:       23,
				Epoch:             0,
				Timestamp:         11,
				ParentRoundNumber: 22,
				CurrentRootHash:   []byte{},
			},
			wantErr: true,
			errIs:   ErrInvalidRootRoundInfoHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &RootRoundInfo{
				RoundNumber:       tt.fields.RoundNumber,
				Epoch:             tt.fields.Epoch,
				Timestamp:         tt.fields.Timestamp,
				ParentRoundNumber: tt.fields.ParentRoundNumber,
				CurrentRootHash:   tt.fields.CurrentRootHash,
			}
			if err := x.IsValid(); (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommitInfo_IsValid(t *testing.T) {
	type fields struct {
		RootRoundInfoHash []byte
		RootHash          []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		{
			name:   "valid",
			fields: fields{RootRoundInfoHash: abhash.Sum256([]byte("test")), RootHash: []byte{1, 2, 3}},
		},
		{
			name:    "invalid, root hash is nil",
			fields:  fields{RootRoundInfoHash: abhash.Sum256([]byte("test")), RootHash: nil},
			wantErr: ErrUnicitySealHashIsNil,
		},
		{
			name:    "invalid, root hash len is 0",
			fields:  fields{RootRoundInfoHash: abhash.Sum256([]byte("test")), RootHash: []byte{}},
			wantErr: ErrUnicitySealHashIsNil,
		},
		{
			name:    "invalid, root round info hash is nil",
			fields:  fields{RootRoundInfoHash: nil, RootHash: []byte{1, 2, 3}},
			wantErr: ErrInvalidRootInfoHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &CommitInfo{
				RootRoundInfoHash: tt.fields.RootRoundInfoHash,
				RootHash:          tt.fields.RootHash,
			}
			err := x.IsValid()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestUnicitySeal_Bytes(t *testing.T) {
	seal := &UnicitySeal{
		Version: 1,
		RootRoundInfo: &RootRoundInfo{
			RoundNumber:       1,
			Epoch:             0,
			Timestamp:         0x123456789,
			ParentRoundNumber: 0,
			CurrentRootHash:   []byte{0, 1, 2},
		},
		CommitInfo: &CommitInfo{
			RootRoundInfoHash: []byte{1, 2, 3},
			RootHash:          []byte{2, 3, 4},
		},
		Signatures: map[string][]byte{"1": {3, 4, 5}},
	}
	var serializedSeal []byte
	serializedSeal = append(serializedSeal, util.Uint64ToBytes(seal.Version)...)
	// append root round info
	serializedSeal = append(serializedSeal, util.Uint64ToBytes(seal.RootRoundInfo.RoundNumber)...)
	serializedSeal = append(serializedSeal, util.Uint64ToBytes(seal.RootRoundInfo.Epoch)...)
	serializedSeal = append(serializedSeal, util.Uint64ToBytes(seal.RootRoundInfo.Timestamp)...)
	serializedSeal = append(serializedSeal, util.Uint64ToBytes(seal.RootRoundInfo.ParentRoundNumber)...)
	serializedSeal = append(serializedSeal, seal.RootRoundInfo.CurrentRootHash...)
	// append commit info
	serializedSeal = append(serializedSeal, seal.CommitInfo.RootRoundInfoHash...)
	serializedSeal = append(serializedSeal, seal.CommitInfo.RootHash...)

	require.Equal(t, seal.Bytes(), serializedSeal)
}

func TestCommitInfo_Hash(t *testing.T) {
	commitInfo := &CommitInfo{
		RootRoundInfoHash: []byte{0, 1, 2},
		RootHash:          []byte{2, 3, 4},
	}
	// make sure hash is calculated over commit info serialized like this
	var serialized []byte
	serialized = append(serialized, commitInfo.RootRoundInfoHash...)
	serialized = append(serialized, commitInfo.RootHash...)
	hasher := gocrypto.SHA256.New()
	hasher.Write(serialized)
	require.Equal(t, commitInfo.Hash(gocrypto.SHA256), hasher.Sum(nil))
}
