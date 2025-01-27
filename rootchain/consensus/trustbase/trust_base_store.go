package trustbase

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/keyvaluedb"
)

var trustBasePrefix = []byte("trust_base_") // append version and epoch number bytes

type (
	Store struct {
		storage keyvaluedb.KeyValueDB
	}
)

func NewStore(db keyvaluedb.KeyValueDB) (*Store, error) {
	if db == nil {
		return nil, errors.New("storage is nil")
	}
	return &Store{storage: db}, nil
}

func (x *Store) GetDB() keyvaluedb.KeyValueDB {
	return x.storage
}

// LoadTrustBase returns trust base for the given epoch number, with the cached verifiers.
func (x *Store) LoadTrustBase(epochNumber uint64) (types.RootTrustBase, error) {
	version := x.GetVersionNumber(epochNumber)
	trustBaseKey := getTrustBaseStoreKey(version, epochNumber)
	if version == 0 {
		var tb *types.RootTrustBaseV1
		ok, err := x.storage.Read(trustBaseKey, &tb)
		if err != nil {
			return nil, fmt.Errorf("failed to load trust base for version %d and epoch %d: %w", version, epochNumber, err)
		}
		if !ok {
			return nil, nil
		}
		return tb, nil
	} else {
		return nil, fmt.Errorf("trust base version %d not implemented", version)
	}
}

// StoreTrustBase saves to given trust base to disk indexed by the epoch number, in cbor encoding.
func (x *Store) StoreTrustBase(epochNumber uint64, tb types.RootTrustBase) error {
	version := x.GetVersionNumber(epochNumber)
	trustBaseKey := getTrustBaseStoreKey(version, epochNumber)
	if err := x.storage.Write(trustBaseKey, tb); err != nil {
		return fmt.Errorf("failed to store trust base: %w", err)
	}
	return nil
}

// GetVersionNumber returns trust base version number based on epoch number
func (x *Store) GetVersionNumber(epochNumber uint64) uint64 {
	return 0
}

func getTrustBaseStoreKey(version, epoch uint64) []byte {
	var trustBaseKey []byte
	trustBaseKey = append(trustBaseKey, trustBasePrefix...)
	trustBaseKey = append(trustBaseKey, util.Uint64ToBytes(version)...)
	trustBaseKey = append(trustBaseKey, util.Uint64ToBytes(epoch)...)
	return trustBaseKey
}
