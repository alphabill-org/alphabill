package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const identifier SystemID = 0x01010101

var uct = &UnicityTreeCertificate{
	SystemIdentifier: identifier,
	SiblingHashes: [][]byte{
		zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash,
		zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash,
		zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash,
		zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash, zeroHash},
	SystemDescriptionHash: zeroHash,
}

func TestUnicityTreeCertificate_IsValid(t *testing.T) {
	type args struct {
		systemIdentifier      SystemID
		systemDescriptionHash []byte
	}
	tests := []struct {
		name   string
		uct    *UnicityTreeCertificate
		args   args
		err    error
		errStr string
	}{
		{
			name: "unicity tree certificate is nil",
			uct:  nil,
			args: args{},
			err:  ErrUnicityTreeCertificateIsNil,
		},
		{
			name: "invalid system identifier",
			uct: &UnicityTreeCertificate{
				SystemIdentifier:      0x01010101,
				SiblingHashes:         [][]byte{},
				SystemDescriptionHash: zeroHash,
			},
			args: args{
				systemIdentifier: 0x01010100,
			},
			errStr: "invalid system identifier",
		},
		{
			name: "invalid system description hash",
			uct: &UnicityTreeCertificate{
				SystemIdentifier:      0x01010101,
				SiblingHashes:         [][]byte{},
				SystemDescriptionHash: nil,
			},
			args: args{
				systemIdentifier:      0x01010101,
				systemDescriptionHash: []byte{2, 1, 1, 1},
			},
			errStr: "invalid system description hash",
		},
		{
			name: "invalid count of sibling hashes",
			uct: &UnicityTreeCertificate{
				SystemIdentifier:      0x01010101,
				SiblingHashes:         [][]byte{},
				SystemDescriptionHash: []byte{2, 1, 1, 1},
			},
			args: args{
				systemIdentifier:      0x01010101,
				systemDescriptionHash: []byte{2, 1, 1, 1},
			},
			errStr: "invalid count of sibling hashes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.uct.IsValid(tt.args.systemIdentifier, tt.args.systemDescriptionHash)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.errStr))
			}
		})
	}
}

func TestUnicityTreeCertificate_IsValidOk(t *testing.T) {
	require.NoError(t, uct.IsValid(identifier, zeroHash))
}
