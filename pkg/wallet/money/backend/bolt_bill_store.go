package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/util"
	bolt "go.etcd.io/bbolt"
)

const BoltBillStoreFileName = "bills.db"

var (
	unitsBucket        = []byte("unitsBucket")        // unitID => unit_bytes
	predicatesBucket   = []byte("predicatesBucket")   // predicate => bucket[unitID]nil
	metaBucket         = []byte("metaBucket")         // block_number_key => block_number_val
	expiredBillsBucket = []byte("expiredBillsBucket") // block_number => list of expired bill ids
	feeUnitsBucket     = []byte("feeUnitsBucket")     // unitID => unit_bytes (for free credit units)
)

var (
	blockNumberKey = []byte("blockNumberKey")
)

var (
	ErrOwnerPredicateIsNil = errors.New("unit owner predicate is nil")
)

type (
	BoltBillStore struct {
		db *bolt.DB
	}

	BoltBillStoreTx struct {
		db *BoltBillStore
		tx *bolt.Tx
	}

	expiredBill struct {
		UnitID []byte `json:"unitId"`
	}
)

// NewBoltBillStore creates new on-disk persistent storage for bills and proofs using bolt db.
// If the file does not exist then it will be created, however, parent directories must exist beforehand.
func NewBoltBillStore(dbFile string) (*BoltBillStore, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second}) // -rw-------
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt DB: %w", err)
	}
	s := &BoltBillStore{db: db}
	err = s.createBuckets()
	if err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}
	err = s.initMetaData()
	if err != nil {
		return nil, fmt.Errorf("failed to init db metadata: %w", err)
	}
	return s, nil
}

func (s *BoltBillStore) WithTransaction(fn func(txc BillStoreTx) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return fn(&BoltBillStoreTx{db: s, tx: tx})
	})
}

func (s *BoltBillStore) Do() BillStoreTx {
	return &BoltBillStoreTx{db: s, tx: nil}
}

func (s *BoltBillStoreTx) GetBill(unitID []byte) (*Bill, error) {
	var unit *Bill
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		bill, err := s.getUnit(tx, unitID)
		if err != nil {
			return err
		}
		if bill == nil {
			return nil
		}
		unit = bill
		return nil
	}, false)
	if err != nil {
		return nil, err
	}
	return unit, nil
}

func (s *BoltBillStoreTx) GetBills(ownerPredicate []byte) ([]*Bill, error) {
	var units []*Bill
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		unitIDBucket := tx.Bucket(predicatesBucket).Bucket(ownerPredicate)
		if unitIDBucket == nil {
			return nil
		}
		return unitIDBucket.ForEach(func(unitID, _ []byte) error {
			unit, err := s.getUnit(tx, unitID)
			if err != nil {
				return err
			}
			if unit == nil {
				return fmt.Errorf("unit in secondary index not found in primary unit bucket unitID=%x", unitID)
			}
			units = append(units, unit)
			return nil
		})
	}, false)
	if err != nil {
		return nil, err
	}
	return units, nil
}

func (s *BoltBillStoreTx) SetBill(bill *Bill) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		billsBucket := tx.Bucket(unitsBucket)
		if bill.OwnerPredicate == nil {
			return ErrOwnerPredicateIsNil
		}

		// remove from previous owner index
		prevUnit, err := s.getUnit(tx, bill.Id)
		if err != nil {
			return err
		}
		if prevUnit != nil {
			prevUnitIDBucket := tx.Bucket(predicatesBucket).Bucket(prevUnit.OwnerPredicate)
			if prevUnitIDBucket != nil {
				err = prevUnitIDBucket.Delete(prevUnit.Id)
				if err != nil {
					return err
				}
			}
		}

		// add to new owner index
		unitIDBucket, err := tx.Bucket(predicatesBucket).CreateBucketIfNotExists(bill.OwnerPredicate)
		if err != nil {
			return err
		}
		err = unitIDBucket.Put(bill.Id, nil)
		if err != nil {
			return err
		}

		// add to main store
		billBytes, err := json.Marshal(bill)
		if err != nil {
			return err
		}
		err = billsBucket.Put(bill.Id, billBytes)
		if err != nil {
			return err
		}
		return nil
	}, true)
}

func (s *BoltBillStoreTx) RemoveBill(unitID []byte) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		return s.removeUnit(tx, unitID)
	}, true)
}

func (s *BoltBillStoreTx) SetBillExpirationTime(blockNumber uint64, unitID []byte) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		expiredBills, err := s.getExpiredBills(tx, blockNumber)
		if err != nil {
			return err
		}
		expiredBills = append(expiredBills, &expiredBill{UnitID: unitID})
		return s.setExpiredBills(tx, blockNumber, expiredBills)
	}, true)
}

func (s *BoltBillStoreTx) DeleteExpiredBills(blockNumber uint64) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		expiredBills, err := s.getExpiredBills(tx, blockNumber)
		if err != nil {
			return err
		}
		// delete bills if not already deleted/swapped
		for _, bill := range expiredBills {
			err := s.removeUnit(tx, bill.UnitID)
			if err != nil {
				return err
			}
		}
		// delete metadata
		return tx.Bucket(expiredBillsBucket).Delete(util.Uint64ToBytes(blockNumber))
	}, true)
}

func (s *BoltBillStoreTx) GetBlockNumber() (uint64, error) {
	blockNumber := uint64(0)
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		blockNumberBytes := tx.Bucket(metaBucket).Get(blockNumberKey)
		blockNumber = util.BytesToUint64(blockNumberBytes)
		return nil
	}, false)
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}

func (s *BoltBillStoreTx) SetBlockNumber(blockNumber uint64) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		blockNumberBytes := util.Uint64ToBytes(blockNumber)
		err := tx.Bucket(metaBucket).Put(blockNumberKey, blockNumberBytes)
		if err != nil {
			return err
		}
		return nil
	}, true)
}

func (s *BoltBillStoreTx) GetFeeCreditBill(unitID []byte) (*Bill, error) {
	var b *Bill
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		fcbBytes := tx.Bucket(feeUnitsBucket).Get(unitID)
		if fcbBytes == nil {
			return nil
		}
		return json.Unmarshal(fcbBytes, &b)
	}, false)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *BoltBillStoreTx) SetFeeCreditBill(fcb *Bill) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		fcbBytes, err := json.Marshal(fcb)
		if err != nil {
			return err
		}
		return tx.Bucket(feeUnitsBucket).Put(fcb.Id, fcbBytes)
	}, true)
}

func (s *BoltBillStoreTx) removeUnit(tx *bolt.Tx, unitID []byte) error {
	unit, err := s.getUnit(tx, unitID)
	if err != nil {
		return err
	}
	if unit == nil {
		return nil
	}

	// delete from "predicate index"
	unitIDBucket := tx.Bucket(predicatesBucket).Bucket(unit.OwnerPredicate)
	if unitIDBucket != nil {
		err = unitIDBucket.Delete(unitID)
		if err != nil {
			return err
		}
	}
	// delete from main store
	return tx.Bucket(unitsBucket).Delete(unitID)
}

func (s *BoltBillStoreTx) getUnit(tx *bolt.Tx, unitID []byte) (*Bill, error) {
	unitBytes := tx.Bucket(unitsBucket).Get(unitID)
	if len(unitBytes) == 0 {
		return nil, nil
	}
	var unit *Bill
	err := json.Unmarshal(unitBytes, &unit)
	if err != nil {
		return nil, err
	}
	return unit, nil
}

func (s *BoltBillStoreTx) getExpiredBills(tx *bolt.Tx, blockNumber uint64) ([]*expiredBill, error) {
	var expiredBills []*expiredBill
	b := tx.Bucket(expiredBillsBucket).Get(util.Uint64ToBytes(blockNumber))
	if b == nil {
		return nil, nil
	}
	err := json.Unmarshal(b, &expiredBills)
	if err != nil {
		return expiredBills, err
	}
	return expiredBills, nil
}

func (s *BoltBillStoreTx) setExpiredBills(tx *bolt.Tx, blockNumber uint64, bills []*expiredBill) error {
	b, err := json.Marshal(bills)
	if err != nil {
		return err
	}
	return tx.Bucket(expiredBillsBucket).Put(util.Uint64ToBytes(blockNumber), b)
}

func (s *BoltBillStoreTx) withTx(dbTx *bolt.Tx, myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if dbTx != nil {
		return myFunc(dbTx)
	} else if writeTx {
		return s.db.db.Update(myFunc)
	} else {
		return s.db.db.View(myFunc)
	}
}

func (s *BoltBillStore) createBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(unitsBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(feeUnitsBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(predicatesBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(expiredBillsBucket)
		if err != nil {
			return err
		}
		return nil
	})
}

func (s *BoltBillStore) initMetaData() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		val := tx.Bucket(metaBucket).Get(blockNumberKey)
		if val == nil {
			return tx.Bucket(metaBucket).Put(blockNumberKey, util.Uint64ToBytes(0))
		}
		return nil
	})
}
