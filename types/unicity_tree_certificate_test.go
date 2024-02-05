package types

import (
	gocrypto "crypto"
	"fmt"
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
	t.Run("invalid path", func(t *testing.T) {
		uct := &UnicityTreeCertificate{
			SystemIdentifier:      identifier,
			SiblingHashes:         []*imt.PathItem{},
			SystemDescriptionHash: []byte{1, 1, 1, 1},
		}
		require.EqualError(t, uct.IsValid(nil, identifier, []byte{1, 1, 1, 1}, gocrypto.SHA256),
			"error sibling hash chain is empty")
	})
	t.Run("invalid leaf key", func(t *testing.T) {
		uct := &UnicityTreeCertificate{
			SystemIdentifier:      identifier,
			SiblingHashes:         []*imt.PathItem{{Key: []byte{0, 0, 0, 0}, Hash: test.RandomBytes(32)}},
			SystemDescriptionHash: []byte{1, 1, 1, 1},
		}
		require.EqualError(t, uct.IsValid(nil, identifier, []byte{1, 1, 1, 1}, gocrypto.SHA256),
			"error invalid leaf key: expected 01010101 got 00000000")
	})
	t.Run("invalid data hash", func(t *testing.T) {
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
		// modify input record
		ir.RoundNumber = 6
		require.EqualError(t, uct.IsValid(ir, identifier, sdrh, gocrypto.SHA256),
			"error invalid data hash: expected 43DA31FD087023D810C98779642CB6B3445E2EB5B435D16B5372B4322FC3AD0A got E6723532032472549C80FA9E148C60038F179C2B086611C24C48D3E80516A002")
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
		require.Equal(t, identifier.Bytes(), leaf.Key())
		var uct = &UnicityTreeCertificate{
			SystemIdentifier:      identifier,
			SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: hasher.Sum(nil)}},
			SystemDescriptionHash: sdrh,
		}
		require.NoError(t, uct.IsValid(ir, identifier, sdrh, gocrypto.SHA256))
	})
}

func TestUnicityTreeCertificate_Serialize(t *testing.T) {
	ut := &UnicityTreeCertificate{
		SystemIdentifier:      identifier,
		SiblingHashes:         []*imt.PathItem{{Key: identifier.Bytes(), Hash: []byte{1, 2, 3}}},
		SystemDescriptionHash: []byte{1, 2, 3, 4},
	}
	expectedBytes := []byte{
		1, 1, 1, 1, //identifier
		1, 1, 1, 1, 1, 2, 3, // siblings key+hash
		1, 2, 3, 4, // system description hash
	}
	require.Equal(t, expectedBytes, ut.Bytes())
	// test add to hasher too
	hasher := gocrypto.SHA256.New()
	hasher.Write(ut.Bytes())
	hash := hasher.Sum(nil)
	// not very useful, but since we get a value then compare
	require.EqualValues(t, "FF0C9E17E999EA6202818F8C723275068468F18DA2524B522F83D48BC2B494DD", fmt.Sprintf("%X", hash))
	hasher.Reset()
	ut.AddToHasher(hasher)
	require.EqualValues(t, "FF0C9E17E999EA6202818F8C723275068468F18DA2524B522F83D48BC2B494DD", fmt.Sprintf("%X", hasher.Sum(nil)))
}
