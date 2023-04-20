package keyvaluedb

import "fmt"

// Reader interface for DB
type Reader interface {
	// Read reads the value for key stored in the DB
	Read(key []byte, value any) (bool, error)
}

// Writer interface for DB
type Writer interface {
	// Write inserts the given value into the DB.
	Write(key []byte, value any) error
	// Delete removes the key from the key-value data store.
	Delete(key []byte) error
}

// DBTx interface for database transactions
// NB! all transactions MUST be completed by either calling Commit() or Rollback() which releases
// the transaction.  Only one read-write transaction is allowed at a time.
type DBTx interface {
	StartTx() (DBTransaction, error)
}

// KeyValueDB contains all the methods required by the high level database to not
// only access the key-value data store but also the chain freezer.
type KeyValueDB interface {
	Reader
	Writer
	Iterable
	DBTx
}

type Iterator interface {
	// Next moves the iterator to the next key value pair
	Next()
	// Prev returns previous key/value pair
	Prev()
	// Valid returns state of the iterator, if at the end false it returned
	Valid() bool
	// Key returns the key of the current key/value pair, or nil if not valid.
	Key() []byte
	// Value returns the value of the current key/value pair, or error if not valid.
	Value(value any) error
	// Close releases associated resources. Release should always succeed and can
	// be called multiple times without causing error.
	Close() error
}

// Iterable wraps the NewIterator methods of a backing data store.
type Iterable interface {
	// First creates a binary-alphabetical forward iterator starting with first item.
	// If the DB is empty the returned iterator returned is not valid (it.Valid() == false)
	// NB! when done iterator MUST be released with Close() or next DB operation will result in deadlock
	First() Iterator
	// Last creates a binary-alphabetical reverse iterator starting with last item.
	// If the DB is empty the returned iterator returned is not valid (it.Valid() == false)
	// NB! when done iterator MUST be released with Close() or next DB operation will result in deadlock
	Last() Iterator
	// Find returns forward iterator to the closest binary-alphabetical match.
	// If no match or DB is empty the returned iterator returned is not valid (it.Valid() == false)
	// NB! when done iterator MUST be released with Close() or next DB operation will result in deadlock
	Find(key []byte) Iterator
}

// DBTransaction key value database transaction
type DBTransaction interface {
	Writer
	Reader
	// Commit commits all pending changes
	Commit() error
	// Rollback reverts everything and nothing is changed
	Rollback() error
}

// IsEmpty is returns true if the key value DB is empty
func IsEmpty(db KeyValueDB) (empty bool, err error) {
	if db == nil {
		return true, fmt.Errorf("db is nil")
	}
	it := db.First()
	defer func() { err = it.Close() }()
	return !it.Valid(), err
}
