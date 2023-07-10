package state

import (
	"crypto"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

type alwaysValid struct{}
type alwaysInvalid struct{}

func (a *alwaysValid) Validate(*types.UnicityCertificate) error {
	return nil
}

func (a alwaysInvalid) Validate(*types.UnicityCertificate) error {
	return errors.New("invalid uc")
}

func TestVerifyUnitStateProof(t *testing.T) {
	s, root, summary := prepareState(t)
	tests := []struct {
		name      string
		proof     *UnitStateProof
		algorithm crypto.Hash
		ucv       UnicityCertificateValidator
		errStr    string
	}{
		{
			name:      "unit state proof is nil",
			proof:     nil,
			algorithm: crypto.SHA256,
			ucv:       &alwaysValid{},
			errStr:    "unit state proof is nil",
		},
		{
			name:      "unit ID missing",
			proof:     &UnitStateProof{},
			algorithm: crypto.SHA256,
			ucv:       &alwaysValid{},
			errStr:    "unit ID is nil",
		},
		{
			name: "unit tree cert missing",
			proof: &UnitStateProof{
				unitID: []byte{0},
			},
			algorithm: crypto.SHA256,
			ucv:       &alwaysValid{},
			errStr:    "unit tree cert is nil",
		},
		{
			name: "state tree cert missing",
			proof: &UnitStateProof{
				unitID:       []byte{0},
				unitTreeCert: &UnitTreeCert{},
			},
			algorithm: crypto.SHA256,
			ucv:       &alwaysValid{},
			errStr:    "state tree cert is nil",
		},
		{
			name: "unicity certificate missing",
			proof: &UnitStateProof{
				unitID:        []byte{0},
				unitTreeCert:  &UnitTreeCert{},
				stateTreeCert: &StateTreeCert{},
			},
			algorithm: crypto.SHA256,
			ucv:       &alwaysValid{},
			errStr:    "unicity certificate is nil",
		},
		{
			name: "invalid unicity certificate",
			proof: &UnitStateProof{
				unitID:             []byte{0},
				unitTreeCert:       &UnitTreeCert{},
				stateTreeCert:      &StateTreeCert{},
				unicityCertificate: &types.UnicityCertificate{},
			},
			algorithm: crypto.SHA256,
			ucv:       &alwaysInvalid{},
			errStr:    "invalid unicity certificate",
		},
		{
			name:      "invalid summary value",
			proof:     getProof(t, s, root, 1),
			algorithm: crypto.SHA256,
			ucv:       &alwaysValid{},
			errStr:    "invalid summary value",
		},
		{
			name:      "invalid output hash",
			proof:     getProof(t, s, []byte{0, 0, 0, 0, 0}, summary),
			algorithm: crypto.SHA256,
			ucv:       &alwaysValid{},
			errStr:    "invalid state root hash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, VerifyUnitStateProof(tt.proof, tt.algorithm, tt.ucv), tt.errStr)
		})
	}
}

func getProof(t *testing.T, s *State, root []byte, summary uint64) *UnitStateProof {
	proof, err := s.CreateUnitStateProof([]byte{0, 0, 0, 5}, 0, &types.UnicityCertificate{InputRecord: &types.InputRecord{
		Hash:         root,
		SummaryValue: util.Uint64ToBytes(summary),
	}})
	require.NoError(t, err)
	return proof
}
