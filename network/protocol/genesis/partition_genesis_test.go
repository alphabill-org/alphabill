package genesis

import (
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/stretchr/testify/require"
)

func TestPartitionGenesis_IsValid(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	trustBase := testtb.NewTrustBase(t, verifier)
	keyInfo := &PublicKeyInfo{
		NodeID:  "1",
		SignKey: pubKey,
		AuthKey: pubKey,
	}

	rootKeyInfo := &PublicKeyInfo{
		NodeID:  "1",
		SignKey: pubKey,
		AuthKey: pubKey,
	}
	validPDR := &types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: 1,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   1 * time.Second,
	}

	type fields struct {
		PDR            *types.PartitionDescriptionRecord
		Certificate    *types.UnicityCertificate
		RootValidators []*PublicKeyInfo
		Keys           []*PublicKeyInfo
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
				Keys: []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: ErrTrustBaseIsNil.Error(),
		},
		{
			name: "system description record is nil",
			args: args{verifier: trustBase},
			fields: fields{
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: types.ErrSystemDescriptionIsNil.Error(),
		},
		{
			name: "keys are missing",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           nil,
			},
			wantErrStr: ErrKeysAreMissing.Error(),
		},
		{
			name: "node signing key info is nil",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{nil},
			},
			wantErrStr: "partition keys validation failed, public key info is empty",
		},

		{
			name: "key info identifier is empty",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys: []*PublicKeyInfo{
					{NodeID: "", SignKey: pubKey, AuthKey: test.RandomBytes(33)},
				},
			},
			wantErrStr: "partition keys validation failed, public key info node identifier is empty",
		},
		{
			name: "signing pub key is invalid",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{{NodeID: "111", SignKey: []byte{0, 0}}},
			},
			wantErrStr: "partition keys validation failed, invalid signing key: pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "authentication key is invalid",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{{NodeID: "111", SignKey: pubKey, AuthKey: []byte{0, 0}}},
			},
			wantErrStr: "partition keys validation failed, invalid authentication key: pubkey must be 33 bytes long, but is 2",
		},
		{
			name: "invalid root signing public key",
			args: args{verifier: trustBase},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{{NodeID: "1", SignKey: []byte{0}, AuthKey: pubKey}},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: "root node list validation failed, invalid signing key: pubkey must be 33 bytes long, but is 1",
		},
		{
			name: "certificate is nil",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				Certificate:    nil,
				RootValidators: []*PublicKeyInfo{rootKeyInfo},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: ErrPartitionUnicityCertificateIsNil.Error(),
		},
		{
			name: "authentication key is nil",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{{NodeID: "1", SignKey: pubKey, AuthKey: nil}},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: "root node list validation failed, public key info authentication key is invalid",
		},
		{
			name: "authentication key is invalid",
			args: args{verifier: trustBase, hashAlgorithm: gocrypto.SHA256},
			fields: fields{
				PDR:            validPDR,
				RootValidators: []*PublicKeyInfo{{NodeID: "1", SignKey: pubKey, AuthKey: []byte{0, 0, 0, 0}}},
				Keys:           []*PublicKeyInfo{keyInfo},
			},
			wantErrStr: "root node list validation failed, invalid authentication key: pubkey must be 33 bytes long, but is 4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionGenesis{
				PartitionDescription: tt.fields.PDR,
				Certificate:          tt.fields.Certificate,
				RootValidators:       tt.fields.RootValidators,
				Keys:                 tt.fields.Keys,
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
