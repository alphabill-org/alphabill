package wallet

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func CreateBuckets(update func(fn func(*bolt.Tx) error) error, buckets ...[]byte) error {
	return update(func(tx *bolt.Tx) error {
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func EnsureSubBucket(tx *bolt.Tx, parentBucket []byte, bucket []byte, allowAbsent bool) (*bolt.Bucket, error) {
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
