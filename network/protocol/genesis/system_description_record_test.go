package genesis

import (
	gocrypto "crypto"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

func TestSystemDescriptionRecord_CanBeHashed(t *testing.T) {
	sdr := &SystemDescriptionRecord{
		SystemIdentifier: 1,
		T2Timeout:        2,
	}
	actualHash := sdr.Hash(gocrypto.SHA256)

	hasher := gocrypto.SHA256.New()
	hasher.Reset()
	hasher.Write(sdr.SystemIdentifier.Bytes())
	hasher.Write(util.Uint64ToBytes(2))
	expectedHash := hasher.Sum(nil)

	require.EqualValues(t, expectedHash, actualHash)
}

func TestSystemDescriptionRecord_IsValid(t *testing.T) {
	type fields struct {
		SystemIdentifier types.SystemID
		T2Timeout        uint32
	}
	tests := []struct {
		name    string
		fields  fields
		wantStr string
	}{
		{
			name: "invalid system identifier",
			fields: fields{
				SystemIdentifier: 0,
				T2Timeout:        2,
			},
			wantStr: "invalid system identifier",
		},
		{
			name: "invalid t2 timeout",
			fields: fields{
				SystemIdentifier: 1,
				T2Timeout:        0,
			},
			wantStr: ErrT2TimeoutIsNil.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &SystemDescriptionRecord{
				SystemIdentifier: tt.fields.SystemIdentifier,
				T2Timeout:        tt.fields.T2Timeout,
			}
			err := x.IsValid()
			require.True(t, strings.Contains(err.Error(), tt.wantStr))
		})
	}
}
