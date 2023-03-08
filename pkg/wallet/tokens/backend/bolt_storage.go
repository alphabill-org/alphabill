package twb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	bucketMetadata = []byte("meta")
	keyBlockNumber = []byte("block-number")

	bucketTokenType   = []byte("token-type")   // TokenTypeID -> json(TokenUnitType)
	bucketTypeCreator = []byte("type-creator") // type creator (pub key) -> [TokenTypeID -> b(kind)]
	bucketTokenUnit   = []byte("token-unit")   // TokenID -> json(TokenUnit)
	bucketTokenOwner  = []byte("token-owner")  // token bearer (p2pkh predicate) -> [TokenID -> b(kind)]
	bucketTxHistory   = []byte("tx-history")   // UnitID(TokenTypeID|TokenID) -> [txHash -> json(block proof)]
)

var errRecordNotFound = errors.New("not found")

type storage struct {
	db *bolt.DB
}

func (s *storage) Close() error { return s.db.Close() }

func (s *storage) SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator PubKey) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := s.ensureSubBucket(tx, bucketTypeCreator, creator, false)
		if err != nil {
			return fmt.Errorf("bucket %s/%X not found", bucketTypeCreator, creator)
		}
		return b.Put(id, []byte{byte(kind)})
	})
}

/*
QueryTokenType loads token types filtered by "kind" and "creator", starting from "startKey" and returning at maximum "count" items.
  - "creator" parameter is optional, when nil result is not filterd by creator.
  - return value "next" just indicates that there is more data, it might not match the (kind) filter!
*/
func (s *storage) QueryTokenType(kind Kind, creator PubKey, startKey TokenTypeID, count int) (rsp []*TokenUnitType, next TokenTypeID, _ error) {
	if creator != nil {
		return s.tokenTypesByCreator(creator, kind, startKey, count)
	}

	// slow path, decode items and check the kind
	return rsp, next, s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketTokenType).Cursor()
		for k, v := setPosition(c, startKey); k != nil; k, v = c.Next() {
			item := &TokenUnitType{}
			if err := json.Unmarshal(v, item); err != nil {
				return fmt.Errorf("failed to deserialize token type data (%x: %s): %w", k, v, err)
			}
			// does the item match our query?
			if kind == Any || kind == item.Kind {
				rsp = append(rsp, item)
				if count--; count == 0 {
					next, _ = c.Next()
					return nil
				}
			}
		}
		return nil
	})
}

func (s *storage) tokenTypesByCreator(creator PubKey, kind Kind, startKey []byte, count int) (rsp []*TokenUnitType, next []byte, _ error) {
	return rsp, next, s.db.View(func(tx *bolt.Tx) error {
		ownerBucket, err := s.ensureSubBucket(tx, bucketTypeCreator, creator, true)
		if err != nil {
			return err
		}
		if ownerBucket == nil {
			return nil
		}

		obc := ownerBucket.Cursor()
		for k, v := setPosition(obc, startKey); k != nil; k, v = obc.Next() {
			if kind == Any || kind == Kind(v[0]) {
				item, err := s.getTokenType(tx, k)
				if err != nil {
					if errors.Is(err, errRecordNotFound) {
						// it is expected that token data may be missing
						continue
					}
					return err
				}
				rsp = append(rsp, item)
				if count--; count == 0 {
					next, _ = obc.Next()
					return nil
				}
			}
		}
		return nil
	})
}

func (s *storage) SaveTokenType(tokenType *TokenUnitType, proof *Proof) error {
	tokenData, err := json.Marshal(tokenType)
	if err != nil {
		return fmt.Errorf("failed to serialize token type data: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		err := tx.Bucket(bucketTokenType).Put(tokenType.ID, tokenData)
		if err != nil {
			return fmt.Errorf("failed to save token type data: %w", err)
		}
		if err := s.storeUnitBlockProof(tx, tokenType.ID, tokenType.TxHash, proof); err != nil {
			return fmt.Errorf("failed to store unit block proof: %w", err)
		}
		return nil
	})
}

func (s *storage) GetTokenType(id TokenTypeID) (*TokenUnitType, error) {
	d := &TokenUnitType{}
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketTokenType).Get(id)
		if data == nil {
			return fmt.Errorf("failed to read token type data %s[%x]: %w", bucketTokenType, id, errRecordNotFound)
		}
		if err := json.Unmarshal(data, d); err != nil {
			return fmt.Errorf("failed to deserialize token type data (%x): %w", id, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *storage) SaveToken(token *TokenUnit, proof *Proof) error {
	tokenData, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to serialize token unit data: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		prevTokenData, err := s.getToken(tx, token.ID)
		if err != nil {
			if !errors.Is(err, errRecordNotFound) {
				return err
			}
		}
		if prevTokenData != nil && !bytes.Equal(prevTokenData.Owner, token.Owner) {
			prevOwnerBucket, err := s.ensureSubBucket(tx, bucketTokenOwner, prevTokenData.Owner, false)
			if err != nil {
				return err
			}
			if err = prevOwnerBucket.Delete(prevTokenData.ID); err != nil {
				return fmt.Errorf("failed to delete token from previous owner bucket: %w", err)
			}
		}
		ownerBucket, err := s.ensureSubBucket(tx, bucketTokenOwner, token.Owner, false)
		if err != nil {
			return err
		}
		if err = ownerBucket.Put(token.ID, []byte{byte(token.Kind)}); err != nil {
			return fmt.Errorf("failed to store token-owner relation: %w", err)
		}
		if err = tx.Bucket(bucketTokenUnit).Put(token.ID, tokenData); err != nil {
			return err
		}
		return s.storeUnitBlockProof(tx, token.ID, token.TxHash, proof)
	})
}

func (s *storage) GetToken(id TokenID) (token *TokenUnit, _ error) {
	return token, s.db.View(func(tx *bolt.Tx) (err error) {
		token, err = s.getToken(tx, id)
		return err
	})
}

/*
QueryTokens loads tokens filtered by "kind" and "owner", starting from "startKey" and returning at maximum "count" items.
  - owner is required, ie can't query "any owner".
  - return value "next" just indicates that there is more data, it might not match the (kind) filter!
*/
func (s *storage) QueryTokens(kind Kind, owner Predicate, startKey TokenID, count int) (rsp []*TokenUnit, next TokenID, _ error) {
	return rsp, next, s.db.View(func(tx *bolt.Tx) error {
		ownerBucket, err := s.ensureSubBucket(tx, bucketTokenOwner, owner, true)
		if err != nil {
			return err
		}
		if ownerBucket == nil {
			return nil
		}

		obc := ownerBucket.Cursor()
		for k, v := setPosition(obc, startKey); k != nil; k, v = obc.Next() {
			if kind == Any || kind == Kind(v[0]) {
				item, err := s.getToken(tx, k)
				if err != nil {
					return err
				}
				rsp = append(rsp, item)
				if count--; count == 0 {
					next, _ = obc.Next()
					return nil
				}
			}
		}
		return nil
	})
}

func (s *storage) GetBlockNumber() (uint64, error) {
	var blockNumber uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		blockNumberBytes := tx.Bucket(bucketMetadata).Get(keyBlockNumber)
		if blockNumberBytes == nil {
			return fmt.Errorf("block number not stored (%s->%s)", bucketMetadata, keyBlockNumber)
		}
		blockNumber = util.BytesToUint64(blockNumberBytes)
		return nil
	})
	return blockNumber, err
}

func (s *storage) SetBlockNumber(blockNumber uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMetadata).Put(keyBlockNumber, util.Uint64ToBytes(blockNumber))
	})
}

func (s *storage) getTokenType(tx *bolt.Tx, id TokenTypeID) (*TokenUnitType, error) {
	var data []byte
	if data = tx.Bucket(bucketTokenType).Get(id); data == nil {
		return nil, fmt.Errorf("failed to read token type data %s[%X]: %w", bucketTokenType, id, errRecordNotFound)
	}
	tokenType := &TokenUnitType{}
	if err := json.Unmarshal(data, tokenType); err != nil {
		return nil, fmt.Errorf("failed to deserialize token type data (%X): %w", id, err)
	}
	return tokenType, nil
}

func (s *storage) getToken(tx *bolt.Tx, id TokenID) (*TokenUnit, error) {
	var data []byte
	if data = tx.Bucket(bucketTokenUnit).Get(id); data == nil {
		return nil, fmt.Errorf("failed to read token data %s[%X]: %w", bucketTokenUnit, id, errRecordNotFound)
	}
	token := &TokenUnit{}
	if err := json.Unmarshal(data, token); err != nil {
		return nil, fmt.Errorf("failed to deserialize token data (%x): %w", id, err)
	}
	return token, nil
}

func (s *storage) storeUnitBlockProof(tx *bolt.Tx, unitID []byte, txHash []byte, proof *Proof) error {
	proofData, err := json.Marshal(proof)
	if err != nil {
		return fmt.Errorf("failed to serialize proof data: %w", err)
	}
	b, err := s.ensureSubBucket(tx, bucketTxHistory, unitID, false)
	if err != nil {
		return err
	}
	return b.Put(txHash, proofData)
}

func (s *storage) ensureSubBucket(tx *bolt.Tx, parentBucket []byte, bucket []byte, allowAbsent bool) (*bolt.Bucket, error) {
	pb := tx.Bucket(parentBucket)
	if pb == nil {
		return nil, fmt.Errorf("bucket %s not found", parentBucket)
	}
	b := pb.Bucket(bucket)
	if b == nil {
		if tx.Writable() {
			return pb.CreateBucket(bucket)
		}
		if allowAbsent {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to ensure bucket %s/%X", parentBucket, bucket)
	}
	return b, nil
}

func (s *storage) createBuckets(buckets ...[]byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range buckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return fmt.Errorf("failed to create bucket %q: %w", b, err)
			}
		}
		return nil
	})
}

func (s *storage) initMetaData() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		val := tx.Bucket(bucketMetadata).Get(keyBlockNumber)
		if val == nil {
			return tx.Bucket(bucketMetadata).Put(keyBlockNumber, util.Uint64ToBytes(0))
		}
		return nil
	})
}

func setPosition(c *bolt.Cursor, key []byte) (k, v []byte) {
	if key != nil {
		return c.Seek(key)
	}
	return c.First()
}

func newBoltStore(dbFile string) (*storage, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second}) // -rw-------
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt DB: %w", err)
	}
	s := &storage{db: db}

	if err := s.createBuckets(bucketMetadata, bucketTokenType, bucketTokenUnit, bucketTypeCreator, bucketTokenOwner, bucketTxHistory); err != nil {
		return nil, fmt.Errorf("failed to create db buckets: %w", err)
	}

	if err := s.initMetaData(); err != nil {
		return nil, fmt.Errorf("failed to init db metadata: %w", err)
	}

	return s, nil
}