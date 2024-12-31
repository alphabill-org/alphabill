package genesis

import (
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/stretchr/testify/require"
)

func TestPartitionGenesis_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	sigKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	trustBase := testtb.NewTrustBase(t, verifier)
	keyInfo := types.NewNodeInfoFromVerifier("1", 1, verifier)
	rootKeyInfo := types.NewNodeInfoFromVerifier("1", 1, verifier)
	validPDR := &types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: 1,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   1 * time.Second,
	}

	type fields struct {
		PDR                 *types.PartitionDescriptionRecord
		Certificate         *types.UnicityCertificate
		RootValidators      []*types.NodeInfo
		PartitionValidators []*types.NodeInfo
	}
	type args struct {
		verifier      types.RootTrustBase
		hashAlgorithm gocrypto.Hash
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "verifier is nil",
			args: args{verifier: nil},
			fields: fields{
				PartitionValidators: []*types.NodeInfo{keyInfo},
			},
			wantErrStr: ErrTrustBaseIsNil.Error(),
		},
		{
			name: "system description record is nil",
			args: args{verifier: trustBase},
			fields: fields{
				RootValidators:      []*types.NodeInfo{rootKeyInfo},
				PartitionValidators: []*types.NodeInfo{keyInfo},
			},
			wantErrStr: types.ErrSystemDescriptionIsNil.Error(),
		},
		{
			name: "keys are missing",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:                 validPDR,
				RootValidators:      []*types.NodeInfo{rootKeyInfo},
				PartitionValidators: nil,
			},
			wantErrStr: ErrPartitionValidatorsMissing.Error(),
		},
		{
			name: "node info is nil",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:                 validPDR,
				RootValidators:      []*types.NodeInfo{rootKeyInfo},
				PartitionValidators: []*types.NodeInfo{nil},
			},
			wantErrStr: "invalid partition validators, node info is empty",
		},

		{
			name: "node identifier is empty",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:                 validPDR,
				RootValidators:      []*types.NodeInfo{rootKeyInfo},
				PartitionValidators: []*types.NodeInfo{{NodeID: "", SigKey: sigKey}},
			},
			wantErrStr: "invalid partition validators, node identifier is empty",
		},
		{
			name: "signing key is invalid",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*types.NodeInfo{rootKeyInfo},
				PartitionValidators: []*types.NodeInfo{&types.NodeInfo{NodeID: "111", SigKey: []byte{1, 2}}},
			},
			wantErrStr: "invalid partition validators, signing key is invalid: pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "invalid root signing key",
			args: args{verifier: trustBase},
			fields: fields{
				PDR:                 validPDR,
				RootValidators:      []*types.NodeInfo{{NodeID: "1", SigKey: []byte{0}}},
				PartitionValidators: []*types.NodeInfo{keyInfo},
			},
			wantErrStr: "invalid root validators, signing key is invalid: pubkey must be 33 bytes long, but is 1",
		},
		{
			name: "certificate is nil",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:                 validPDR,
				Certificate:         nil,
				RootValidators:      []*types.NodeInfo{rootKeyInfo},
				PartitionValidators: []*types.NodeInfo{keyInfo},
			},
			wantErrStr: ErrPartitionUnicityCertificateIsNil.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionGenesis{
				PartitionDescription: tt.fields.PDR,
				Certificate:          tt.fields.Certificate,
				RootValidators:       tt.fields.RootValidators,
				PartitionValidators:  tt.fields.PartitionValidators,
			}
			err = x.IsValid(tt.args.verifier, tt.args.hashAlgorithm)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPartitionGenesis_IsValid_Nil(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	var pr *PartitionGenesis
	verifiers := testtb.NewTrustBase(t, verifier)
	require.ErrorIs(t, pr.IsValid(verifiers, gocrypto.SHA256), ErrPartitionGenesisIsNil)
}
