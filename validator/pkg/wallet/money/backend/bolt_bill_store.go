package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	sdk "github.com/alphabill-org/alphabill/validator/pkg/wallet"
)

const BoltBillStoreFileName = "bills.db"

var (
	unitsBucket           = []byte("unitsBucket")           // unitID => unit_bytes
	predicatesBucket      = []byte("predicatesBucket")      // predicate => bucket[unitID]nil
	metaBucket            = []byte("metaBucket")            // block_number_key => block_number_val
	expiredBillsBucket    = []byte("expiredBillsBucket")    // block_number => bucket[unitID]nil
	feeUnitsBucket        = []byte("feeUnitsBucket")        // unitID => unit_bytes (for free credit units)
	lockedFeeCreditBucket = []byte("lockedFeeCreditBucket") // systemID => [unitID => transferFC record]
	closedFeeCreditBucket = []byte("closedFeeCreditBucket")
	sdrBucket             = []byte("sdrBucket") // []genesis.SystemDescriptionRecord
	txProofsBucket        = []byte("txProofs")  // unitID => [txHash => cbor(block proof)]
	txHistoryBucket       = []byte("txHistory") // pubKeyHash => [seqNum => cbor(TxHistoryRecord)]
)

var (
	blockNumberKey = []byte("blockNumberKey")
)

var (
	ErrOwnerPredicateIsNil = errors.New("unit owner predicate is nil")
)

var _ BillStoreTx = (*boltBillStoreTx)(nil)

type (
	boltBillStore struct {
		db *bolt.DB
	}

	boltBillStoreTx struct {
		db *boltBillStore
		tx *bolt.Tx
	}
)

// newBoltBillStore creates new on-disk persistent storage for bills and proofs using bolt db.
// If the file does not exist then it will be created, however, parent directories must exist beforehand.
func newBoltBillStore(dbFile string) (*boltBillStore, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second}) // -rw-------
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt DB: %w", err)
	}
	s := &boltBillStore{db: db}
	err = sdk.CreateBuckets(db.Update,
		unitsBucket, predicatesBucket, metaBucket, expiredBillsBucket, feeUnitsBucket,
		lockedFeeCreditBucket, closedFeeCreditBucket, sdrBucket, txProofsBucket, txHistoryBucket,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}
	err = s.initMetaData()
	if err != nil {
		return nil, fmt.Errorf("failed to init db metadata: %w", err)
	}
	return s, nil
}

func (s *boltBillStore) WithTransaction(fn func(txc BillStoreTx) error) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return fn(&boltBillStoreTx{db: s, tx: tx})
	})
}

func (s *boltBillStore) Do() BillStoreTx {
	return &boltBillStoreTx{db: s, tx: nil}
}

func (s *boltBillStoreTx) GetBill(unitID []byte) (*Bill, error) {
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

func (s *boltBillStoreTx) GetBills(ownerPredicate []byte, includeDCBills bool, startKey []byte, limit int) ([]*Bill, []byte, error) {
	var units []*Bill
	var nextKey []byte
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		unitIDBucket := tx.Bucket(predicatesBucket).Bucket(ownerPredicate)
		if unitIDBucket == nil {
			return nil
		}
		c := unitIDBucket.Cursor()
		for unitID, _ := setPosition(c, startKey); unitID != nil; unitID, _ = c.Next() {
			if limit == 0 {
				nextKey = unitID
				return nil
			}
			unit, err := s.getUnit(tx, unitID)
			if err != nil {
				return err
			}
			if unit == nil {
				return fmt.Errorf("unit in secondary index not found in primary unit bucket unitID=%x", unitID)
			}
			if unit.IsDCBill() && !includeDCBills {
				continue
			}
			units = append(units, unit)
			limit--
		}
		return nil
	}, false)
	if err != nil {
		return nil, nil, err
	}
	return units, nextKey, nil
}

func (s *boltBillStoreTx) SetBill(bill *Bill, proof *sdk.Proof) error {
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
		if prevUnit != nil && !bytes.Equal(prevUnit.OwnerPredicate, bill.OwnerPredicate) {
			prevUnitIDBucket, err := sdk.EnsureSubBucket(tx, predicatesBucket, prevUnit.OwnerPredicate, false)
			if err != nil {
				return err
			}
			if err = prevUnitIDBucket.Delete(prevUnit.Id); err != nil {
				return err
			}
		}

		// add to new owner index
		unitIDBucket, err := sdk.EnsureSubBucket(tx, predicatesBucket, bill.OwnerPredicate, false)
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
		return s.storeTxProof(tx, bill.Id, bill.TxHash, proof)
	}, true)
}

func (s *boltBillStoreTx) RemoveBill(unitID []byte) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		return s.removeUnit(tx, unitID)
	}, true)
}

func (s *boltBillStoreTx) SetBillExpirationTime(blockNumber uint64, unitID []byte) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		return s.addExpiredBill(tx, blockNumber, unitID)
	}, true)
}

func (s *boltBillStoreTx) DeleteExpiredBills(maxBlockNumber uint64) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		expiredBills, err := s.getExpiredBills(tx, maxBlockNumber)
		if err != nil {
			return err
		}
		// delete bills if not already deleted/swapped
		for unitIDStr, blockNumber := range expiredBills {
			// delete unit from main bucket
			unitID := []byte(unitIDStr)
			if err := s.removeUnit(tx, unitID); err != nil {
				return err
			}
			// delete expired bill metadata
			if err := tx.Bucket(expiredBillsBucket).DeleteBucket(blockNumber); err != nil {
				if !errors.Is(err, bolt.ErrBucketNotFound) {
					return err
				}
			}
		}
		return nil
	}, true)
}

func (s *boltBillStoreTx) GetBlockNumber() (uint64, error) {
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

func (s *boltBillStoreTx) SetBlockNumber(blockNumber uint64) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		blockNumberBytes := util.Uint64ToBytes(blockNumber)
		err := tx.Bucket(metaBucket).Put(blockNumberKey, blockNumberBytes)
		if err != nil {
			return err
		}
		return nil
	}, true)
}

func (s *boltBillStoreTx) GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	var proof *sdk.Proof
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		var err error
		proof, err = s.getUnitBlockProof(tx, unitID, txHash)
		return err
	}, false)
	return proof, err
}

func (s *boltBillStoreTx) GetFeeCreditBill(unitID []byte) (*Bill, error) {
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

func (s *boltBillStoreTx) SetFeeCreditBill(fcb *Bill, proof *sdk.Proof) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		fcbBytes, err := json.Marshal(fcb)
		if err != nil {
			return err
		}
		if err = tx.Bucket(feeUnitsBucket).Put(fcb.Id, fcbBytes); err != nil {
			return err
		}
		return s.storeTxProof(tx, fcb.Id, fcb.TxHash, proof)
	}, true)
}

func (s *boltBillStoreTx) GetLockedFeeCredit(systemID, fcbID []byte) (*types.TransactionRecord, error) {
	var res *types.TransactionRecord
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		lockedCreditBucket := tx.Bucket(lockedFeeCreditBucket).Bucket(systemID)
		if lockedCreditBucket == nil {
			return nil
		}
		txBytes := lockedCreditBucket.Get(fcbID)
		if txBytes == nil {
			return nil
		}
		return cbor.Unmarshal(txBytes, &res)
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *boltBillStoreTx) SetLockedFeeCredit(systemID, fcbID []byte, txr *types.TransactionRecord) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		txBytes, err := cbor.Marshal(txr)
		if err != nil {
			return err
		}
		lockedCreditBucket, err := tx.Bucket(lockedFeeCreditBucket).CreateBucketIfNotExists(systemID)
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		return lockedCreditBucket.Put(fcbID, txBytes)
	}, true)
}

func (s *boltBillStoreTx) GetClosedFeeCredit(unitID []byte) (*types.TransactionRecord, error) {
	var res *types.TransactionRecord
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		txBytes := tx.Bucket(closedFeeCreditBucket).Get(unitID)
		if txBytes == nil {
			return nil
		}
		return cbor.Unmarshal(txBytes, &res)
	}, false)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *boltBillStoreTx) SetClosedFeeCredit(unitID []byte, txr *types.TransactionRecord) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		txBytes, err := cbor.Marshal(txr)
		if err != nil {
			return err
		}
		return tx.Bucket(closedFeeCreditBucket).Put(unitID, txBytes)
	}, true)
}

func (s *boltBillStoreTx) GetSystemDescriptionRecords() ([]*genesis.SystemDescriptionRecord, error) {
	var sdrs []*genesis.SystemDescriptionRecord
	err := s.withTx(s.tx, func(tx *bolt.Tx) error {
		return tx.Bucket(sdrBucket).ForEach(func(systemID, sdrBytes []byte) error {
			var sdr *genesis.SystemDescriptionRecord
			err := json.Unmarshal(sdrBytes, &sdr)
			if err != nil {
				return err
			}
			sdrs = append(sdrs, sdr)
			return nil
		})
	}, false)
	if err != nil {
		return nil, err
	}
	return sdrs, nil
}

func (s *boltBillStoreTx) SetSystemDescriptionRecords(sdrs []*genesis.SystemDescriptionRecord) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		for _, sdr := range sdrs {
			sdrBytes, err := json.Marshal(sdr)
			if err != nil {
				return err
			}
			err = tx.Bucket(sdrBucket).Put(sdr.SystemIdentifier, sdrBytes)
			if err != nil {
				return err
			}
		}
		return nil
	}, true)
}

func (s *boltBillStoreTx) removeUnit(tx *bolt.Tx, unitID []byte) error {
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

func (s *boltBillStoreTx) getUnit(tx *bolt.Tx, unitID []byte) (*Bill, error) {
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

// getExpiredBills returns map[bill_id_string]block_number_bytes of all bills that expiry block number is less than or equal to the given block number
func (s *boltBillStoreTx) getExpiredBills(tx *bolt.Tx, maxBlockNumber uint64) (map[string][]byte, error) {
	res := make(map[string][]byte)
	expiredBillBucket := tx.Bucket(expiredBillsBucket)
	c := expiredBillBucket.Cursor()
	for blockNumber, _ := c.First(); blockNumber != nil && util.BytesToUint64(blockNumber) <= maxBlockNumber; blockNumber, _ = c.Next() {
		expiredUnitIDsBucket := expiredBillBucket.Bucket(blockNumber)
		err := expiredUnitIDsBucket.ForEach(func(unitID, _ []byte) error {
			res[string(unitID)] = blockNumber
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *boltBillStoreTx) addExpiredBill(tx *bolt.Tx, blockNumber uint64, unitID []byte) error {
	b, err := tx.Bucket(expiredBillsBucket).CreateBucketIfNotExists(util.Uint64ToBytes(blockNumber))
	if err != nil {
		return err
	}
	return b.Put(unitID, nil)
}

func (s *boltBillStoreTx) StoreTxProof(unitID types.UnitID, txHash sdk.TxHash, txProof *sdk.Proof) error {
	if unitID == nil {
		return errors.New("unit id is nil")
	}
	if txHash == nil {
		return errors.New("tx hash is nil")
	}
	if txProof == nil {
		return errors.New("tx proof is nil")
	}
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		return s.storeTxProof(tx, unitID, txHash, txProof)
	}, true)
}

func (s *boltBillStoreTx) storeTxProof(dbTx *bolt.Tx, unitID types.UnitID, txHash sdk.TxHash, txProof *sdk.Proof) error {
	if txHash == nil || txProof == nil {
		return nil
	}
	txProofBytes, err := cbor.Marshal(txProof)
	if err != nil {
		return fmt.Errorf("failed to serialize tx proof: %w", err)
	}
	b, err := sdk.EnsureSubBucket(dbTx, txProofsBucket, unitID, false)
	if err != nil {
		return err
	}
	return b.Put(txHash, txProofBytes)
}

func (s *boltBillStoreTx) StoreTxHistoryRecord(hash sdk.PubKeyHash, rec *sdk.TxHistoryRecord) error {
	return s.withTx(s.tx, func(tx *bolt.Tx) error {
		return s.storeTxHistoryRecord(tx, hash, rec)
	}, true)
}

func (s *boltBillStoreTx) storeTxHistoryRecord(tx *bolt.Tx, hash sdk.PubKeyHash, rec *sdk.TxHistoryRecord) error {
	if len(hash) == 0 {
		return errors.New("sender is nil")
	}
	if rec == nil {
		return errors.New("record is nil")
	}
	b, err := sdk.EnsureSubBucket(tx, txHistoryBucket, hash, false)
	if err != nil {
		return err
	}
	id, _ := b.NextSequence()
	recBytes, err := cbor.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to serialize tx history record: %w", err)
	}
	return b.Put(util.Uint64ToBytes(id), recBytes)
}

func (s *boltBillStoreTx) GetTxHistoryRecords(hash sdk.PubKeyHash, dbStartKey []byte, count int) (res []*sdk.TxHistoryRecord, key []byte, err error) {
	return res, key, s.withTx(s.tx, func(tx *bolt.Tx) error {
		var err error
		res, key, err = s.getTxHistoryRecords(tx, hash, dbStartKey, count)
		return err
	}, false)
}

func (s *boltBillStoreTx) getTxHistoryRecords(tx *bolt.Tx, hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
	b, err := sdk.EnsureSubBucket(tx, txHistoryBucket, hash, true)
	if err != nil {
		return nil, nil, err
	}
	if b == nil {
		return nil, nil, nil
	}
	c := b.Cursor()
	if len(dbStartKey) == 0 {
		dbStartKey, _ = c.Last()
	}
	var res []*sdk.TxHistoryRecord
	var prevKey []byte
	for k, v := c.Seek(dbStartKey); k != nil && count > 0; k, v = c.Prev() {
		rec := &sdk.TxHistoryRecord{}
		if err := cbor.Unmarshal(v, rec); err != nil {
			return nil, nil, fmt.Errorf("failed to deserialize tx history record: %w", err)
		}
		res = append(res, rec)
		if count--; count == 0 {
			prevKey, _ = c.Prev()
			break
		}
	}
	return res, prevKey, nil
}

func (s *boltBillStoreTx) getUnitBlockProof(dbTx *bolt.Tx, id []byte, txHash sdk.TxHash) (*sdk.Proof, error) {
	b, err := sdk.EnsureSubBucket(dbTx, txProofsBucket, id, true)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, nil
	}
	proofData := b.Get(txHash)
	if proofData == nil {
		return nil, nil
	}
	proof := &sdk.Proof{}
	if err := cbor.Unmarshal(proofData, proof); err != nil {
		return nil, fmt.Errorf("failed to deserialize proof data: %w", err)
	}
	return proof, nil
}

func (s *boltBillStoreTx) withTx(dbTx *bolt.Tx, myFunc func(tx *bolt.Tx) error, writeTx bool) error {
	if dbTx != nil {
		return myFunc(dbTx)
	} else if writeTx {
		return s.db.db.Update(myFunc)
	} else {
		return s.db.db.View(myFunc)
	}
}

func (s *boltBillStore) initMetaData() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		val := tx.Bucket(metaBucket).Get(blockNumberKey)
		if val == nil {
			return tx.Bucket(metaBucket).Put(blockNumberKey, util.Uint64ToBytes(0))
		}
		return nil
	})
}

func setPosition(c *bolt.Cursor, key []byte) ([]byte, []byte) {
	if key != nil {
		k, v := c.Seek(key)
		if !bytes.Equal(k, key) {
			return nil, nil
		}
		return k, v
	}
	return c.First()
}
