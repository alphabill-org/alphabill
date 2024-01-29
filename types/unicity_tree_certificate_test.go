package types

import (
	gocrypto "crypto"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/tree/imt"
	"github.com/stretchr/testify/require"
)

const identifier SystemID = 0x01010101

func TestUnicityTreeCertificate_IsValid(t *testing.T) {
	t.Run("unicity tree certificate is nil", func(t *testing.T) {
		var uct *UnicityTreeCertificate = nil
		require.ErrorIs(t, uct.IsValid(nil, SystemID(2), test.RandomBytes(32), gocrypto.SHA256), ErrUnicityTreeCertificateIsNil)
	})

	t.Run("invalid system identifier", func(t *testing.T) {
		uct := &UnicityTreeCertificate{
			SystemIdentifier:      0x01010101,
			SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: test.RandomBytes(32)}},
			SystemDescriptionHash: zeroHash,
		}
		require.EqualError(t, uct.IsValid(nil, 0x01010100, test.RandomBytes(32), gocrypto.SHA256),
			"invalid system identifier: expected 01010100, got 01010101")
	})

	t.Run("invalid system description hash", func(t *testing.T) {
		uct := &UnicityTreeCertificate{
			SystemIdentifier:      identifier,
			SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: test.RandomBytes(32)}},
			SystemDescriptionHash: []byte{1, 1, 1, 1},
		}
		require.EqualError(t, uct.IsValid(nil, identifier, []byte{1, 1, 1, 2}, gocrypto.SHA256),
			"invalid system description hash: expected 01010102, got 01010101")
	})

	t.Run("ok", func(t *testing.T) {
		ir := &InputRecord{
			PreviousHash:    []byte{0, 0, 0, 0},
			Hash:            []byte{0, 0, 0, 2},
			BlockHash:       []byte{0, 0, 0, 3},
			SummaryValue:    []byte{0, 0, 0, 4},
			RoundNumber:     5,
			SumOfEarnedFees: 10,
		}
		sdrh := []byte{1, 2, 3, 4}
		leaf := UTData{
			SystemIdentifier:            identifier,
			InputRecord:                 ir,
			SystemDescriptionRecordHash: sdrh,
		}
		hasher := gocrypto.SHA256.New()
		leaf.AddToHasher(hasher)
		var uct = &UnicityTreeCertificate{
			SystemIdentifier:      identifier,
			SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: hasher.Sum(nil)}},
			SystemDescriptionHash: sdrh,
		}
		require.NoError(t, uct.IsValid(ir, identifier, sdrh, gocrypto.SHA256))
	})

}
