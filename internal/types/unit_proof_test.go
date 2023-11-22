package types

import (
	"crypto"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type alwaysValid struct{}
type alwaysInvalid struct{}

func (a *alwaysValid) Validate(*UnicityCertificate) error {
	return nil
}

func (a alwaysInvalid) Validate(*UnicityCertificate) error {
	return errors.New("invalid uc")
}

func TestVerifyUnitStateProof(t *testing.T) {
	t.Run("unit state proof is nil", func(t *testing.T) {
		require.ErrorContains(t, VerifyUnitStateProof(nil, crypto.SHA256, &alwaysValid{}), "unit state proof is nil")
	})
	t.Run("unit ID missing", func(t *testing.T) {
		require.ErrorContains(t, VerifyUnitStateProof(&UnitStateProof{}, crypto.SHA256, &alwaysValid{}), "unit ID is nil")
	})
	t.Run("unit tree cert missing", func(t *testing.T) {
		proof := &UnitStateProof{
			UnitID: []byte{0},
		}
		require.ErrorContains(t, VerifyUnitStateProof(proof, crypto.SHA256, &alwaysValid{}), "unit tree cert is nil")
	})
	t.Run("state tree cert missing", func(t *testing.T) {
		proof := &UnitStateProof{
			UnitID:       []byte{0},
			UnitTreeCert: &UnitTreeCert{},
		}
		require.ErrorContains(t, VerifyUnitStateProof(proof, crypto.SHA256, &alwaysValid{}), "state tree cert is nil")
	})
	t.Run("unicity certificate missing", func(t *testing.T) {
		proof := &UnitStateProof{
			UnitID:        []byte{0},
			UnitTreeCert:  &UnitTreeCert{},
			StateTreeCert: &StateTreeCert{},
		}
		require.ErrorContains(t, VerifyUnitStateProof(proof, crypto.SHA256, &alwaysValid{}), "unicity certificate is nil")
	})
	t.Run("invalid unicity certificate", func(t *testing.T) {
		proof := &UnitStateProof{
			UnitID:             []byte{0},
			UnitTreeCert:       &UnitTreeCert{},
			StateTreeCert:      &StateTreeCert{},
			UnicityCertificate: &UnicityCertificate{},
		}
		require.ErrorContains(t, VerifyUnitStateProof(proof, crypto.SHA256, &alwaysInvalid{}), "invalid unicity certificate")
	})
	/*
		t.Run("invalid summary value", func(t *testing.T) {
			s, root, _ := setupState(t)
			proof := getCreateProof(t, s, root, []byte{1})
			require.ErrorContains(t, VerifyUnitStateProof(proof, crypto.SHA256, &alwaysValid{}), "invalid summary value")
		})
		t.Run("invalid root hash", func(t *testing.T) {
			s, _, summary := setupState(t)
			proof := getCreateProof(t, s, []byte{0, 0, 0, 0, 0}, summary)
			require.ErrorContains(t, VerifyUnitStateProof(proof, crypto.SHA256, &alwaysValid{}), "invalid state root hash")
		})
		t.Run("ok", func(t *testing.T) {
			s, root, summary := setupState(t)
			proof := getCreateProof(t, s, root, summary)
			require.NoError(t, VerifyUnitStateProof(proof, crypto.SHA256, &alwaysValid{}))
		})
	*/
}
