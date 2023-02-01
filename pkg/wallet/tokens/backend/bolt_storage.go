package twb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"

	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
)

const BoltTokenStoreFileName = "tokens.db"

var (
	bucketMetadata = []byte("meta")
	keyBlockNumber = []byte("block-number")

	bucketTokenType   = []byte("token-type")   // TokenTypeID -> TokenUnitType
	bucketTypeCreator = []byte("type-creator") // type creator (pub key) -> [TokenTypeID]
	bucketTokenUnit   = []byte("token-unit")   // TokenID -> TokenUnit
	bucketTokenOwner  = []byte("token-owner")  // token bearer (p2pkh predicate) -> [TokenID]
	bucketTxHistory   = []byte("tx-history")   // UnitID(TokenTypeID|TokenID) -> [txHash -> block proof]
)

//submit tx endpoint:
// 1. read creator public key
// 2. parse tx
// 3. if it's a 'create type' tx, save type data to "type-creator" bucket

//list types endpoint:
// 1. read creator public key, create a list of type ids from "type-creator" bucket
// 2. read type data from "token-type" bucket
// 3. group types by kind

//list fungible/nft tokens endpoint:
// 1. read creator public key, fetch token ids from "token-owner" bucket
// 2. read token data from "token-unit" bucket, filter fungible/nft tokens by kind

//list tx proofs endpoint:
// 1. read unit id and tx hash from request
// 2. read tx proof from "tx-history" bucket

//list tx history endpoint:
// 1. read unit id from request
// 2. read tx history from "tx-history" bucket, optionally include proofs
// 3. additionally, filter units by owner using "token-owner" bucket

var errRecordNotFound = errors.New("not found")

type storage struct {
	db *bolt.DB
}

func (s *storage) SaveTokenTypeCreator(id TokenTypeID, creator PubKey) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := s.ensureSubBucket(tx, bucketTypeCreator, creator, false)
		if err != nil {
			return fmt.Errorf("bucket %s/%X not found", bucketTypeCreator, creator)
		}
		return b.Put(id, nil)
	})
}

func (s *storage) GetTokenTypesByCreator(creator PubKey) ([]*TokenUnitType, error) {
	var types []*TokenUnitType
	err := s.db.View(func(tx *bolt.Tx) error {
		ids, err := s.getTokenTypeIDsByCreator(tx, creator)
		if err != nil {
			return err
		}
		for _, id := range ids {
			t, err := s.getTokenType(tx, id)
			if err != nil {
				if aberrors.ErrorCausedBy(err, errRecordNotFound) {
					// it is expected that token data may be missing
					continue
				}
				return err
			}
			types = append(types, t)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return types, nil
}

func (s *storage) getTokenTypeIDsByCreator(tx *bolt.Tx, creator PubKey) ([]TokenTypeID, error) {
	b, err := s.ensureSubBucket(tx, bucketTypeCreator, creator, true)
	if err != nil {
		return nil, err
	}
	ids := make([]TokenTypeID, 0)
	if b != nil {
		err = b.ForEach(func(k, _ []byte) error {
			ids = append(ids, k)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return ids, nil
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
		return s.storeUnitBlockProof(tx, tokenType.ID, tokenType.TxHash, proof)
	})
}

func (s *storage) GetTokenType(id TokenTypeID) (*TokenUnitType, error) {
	var data []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		if data = tx.Bucket(bucketTokenType).Get(id); data == nil {
			return fmt.Errorf("failed to read token type data %s[%x]: %w", bucketTokenType, id, errRecordNotFound)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	d := &TokenUnitType{}
	if err := json.Unmarshal(data, d); err != nil {
		return nil, fmt.Errorf("failed to deserialize token type data (%x): %w", id, err)
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
			if !aberrors.ErrorCausedBy(err, errRecordNotFound) {
				return err
			}
		}
		if prevTokenData != nil && !bytes.Equal(prevTokenData.Owner, token.Owner) {
			prevOwnerBucket, err := s.ensureSubBucket(tx, bucketTokenOwner, prevTokenData.Owner, false)
			if err != nil {
				return err
			}
			if err = prevOwnerBucket.Delete(prevTokenData.ID); err != nil {
				return err
			}
		}
		ownerBucket, err := s.ensureSubBucket(tx, bucketTokenOwner, token.Owner, false)
		if err != nil {
			return err
		}
		if err = ownerBucket.Put(token.ID, nil); err != nil {
			return err
		}
		if err = tx.Bucket(bucketTokenUnit).Put(token.ID, tokenData); err != nil {
			return err
		}
		return s.storeUnitBlockProof(tx, token.ID, token.TxHash, proof)
	})
}

func (s *storage) GetToken(id TokenID) (*TokenUnit, error) {
	var token *TokenUnit
	if err := s.db.View(func(tx *bolt.Tx) error {
		result, err := s.getToken(tx, id)
		if err != nil {
			return err
		}
		token = result
		return nil
	}); err != nil {
		return nil, err
	}
	return token, nil
}

func (s *storage) GetTokensByOwner(owner Predicate) ([]*TokenUnit, error) {
	tokens := make([]*TokenUnit, 0)
	if err := s.db.View(func(tx *bolt.Tx) error {
		ownerBucket, err := s.ensureSubBucket(tx, bucketTokenOwner, owner, true)
		if err != nil {
			return err
		}
		if ownerBucket == nil {
			return nil
		}
		return ownerBucket.ForEach(func(k, _ []byte) error {
			token, err := s.getToken(tx, k)
			if err != nil {
				return err
			}
			tokens = append(tokens, token)
			return nil
		})
	}); err != nil {
		return nil, err
	}
	return tokens, nil
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
		prev := util.BytesToUint64(tx.Bucket(bucketMetadata).Get(keyBlockNumber))
		if prev >= blockNumber {
			return fmt.Errorf("block number must be greater than previous one (%d >= %d)", prev, blockNumber)
		}
		return tx.Bucket(bucketMetadata).Put(keyBlockNumber, util.Uint64ToBytes(blockNumber))
	})
}

func (s *storage) Close() error { return s.db.Close() }

func (s *storage) getTokenType(tx *bolt.Tx, id TokenTypeID) (*TokenUnitType, error) {
	var data []byte
	if data = tx.Bucket(bucketTokenType).Get(id); data == nil {
		return nil, aberrors.Wrapf(errRecordNotFound, "failed to read token type data %s[%X]", bucketTokenType, id)
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
		return nil, aberrors.Wrapf(errRecordNotFound, "failed to read token data %s[%X]", bucketTokenUnit, id)
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

func newBoltStore(dbFile string) (*storage, error) {
	db, err := bolt.Open(dbFile, 0600, nil) // -rw-------
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
