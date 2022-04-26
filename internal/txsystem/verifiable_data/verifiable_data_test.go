package verifiable_data

import (
	"crypto"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const defaultUnicityTrustBase = "0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"

type (
	genericTx struct {
		systemID   []byte
		unitId     *uint256.Int
		timeout    uint64
		ownerProof []byte
		sigBytes   []byte
	}

	reg struct {
		genericTx
	}

	unknownTx struct {
		genericTx
		foo string
	}
)

func TestRegisterData(t *testing.T) {
	vd, err := NewVerifiableDataTxSystem([]string{defaultUnicityTrustBase})
	require.NoError(t, err)
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	err = vd.Process(&reg{
		genericTx: genericTx{
			systemID: []byte{1},
			unitId:   uint256.NewInt(0).SetBytes(id),
			timeout:  2,
		},
	})
	require.NoError(t, err)
}

func TestRegisterData_invalidSystemId(t *testing.T) {
	vd, err := NewVerifiableDataTxSystem([]string{defaultUnicityTrustBase})
	require.NoError(t, err)
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	err = vd.Process(&reg{
		genericTx: genericTx{
			systemID: []byte{0},
			unitId:   uint256.NewInt(0).SetBytes(id),
			timeout:  2,
		},
	})
	require.Error(t, err)
}

func TestRegisterData_invalidOwnerProof(t *testing.T) {
	vd, err := NewVerifiableDataTxSystem([]string{defaultUnicityTrustBase})
	require.NoError(t, err)
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	err = vd.Process(&reg{
		genericTx: genericTx{
			systemID:   []byte{0},
			unitId:     uint256.NewInt(0).SetBytes(id),
			timeout:    2,
			ownerProof: script.PredicateAlwaysTrue(),
		},
	})
	require.Error(t, err)
}

func TestRegisterData_UnknownTx(t *testing.T) {
	vd, err := NewVerifiableDataTxSystem([]string{defaultUnicityTrustBase})
	require.NoError(t, err)
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	err = vd.Process(&unknownTx{
		genericTx: genericTx{
			systemID: []byte{1},
			unitId:   uint256.NewInt(0).SetBytes(id),
			timeout:  2,
		},
		foo: "bar",
	})
	// in fact any tx (until systemID matches) fits as a 'reg' tx,
	// at least until we have a fixed set of attributes
	require.NoError(t, err)
}

func TestRegisterData_withDuplicate(t *testing.T) {
	vd, err := NewVerifiableDataTxSystem([]string{defaultUnicityTrustBase})
	require.NoError(t, err)
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	reg := &reg{
		genericTx: genericTx{
			systemID: []byte{1},
			unitId:   uint256.NewInt(0).SetBytes(id),
			timeout:  2,
		},
	}
	err = vd.Process(reg)
	require.NoError(t, err)
	// send duplicate
	err = vd.Process(reg)
	require.Error(t, err, "could not add item")
}

func (t *genericTx) SystemID() []byte     { return t.systemID }
func (t *genericTx) UnitID() *uint256.Int { return t.unitId }
func (t *genericTx) Timeout() uint64      { return t.timeout }
func (t *genericTx) OwnerProof() []byte   { return t.ownerProof }
func (t *genericTx) SigBytes() []byte     { return t.sigBytes }

func (r *reg) Hash(_ crypto.Hash) []byte { return []byte("reg hash") }

func (r *unknownTx) Hash(_ crypto.Hash) []byte { return nil }
