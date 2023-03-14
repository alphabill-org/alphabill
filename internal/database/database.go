package database

// Reader interface for DB
type Reader interface {
	// Empty returns if key value DB is empty
	Empty() bool
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

type DBTx interface {
	StartTx() (DBTransaction, error)
}

// KeyValueDB contains all the methods required by the high level database to not
// only access the key-value data store but also the chain freezer.
type KeyValueDB interface {
	Reader
	Writer
	Iteratee
	DBTx
}

type Iterator interface {
	// Next moves the iterator to the next key value pair
	Next()
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

type ReverseIterator interface {
	// Prev returns previous key/value pair
	Prev()
	// Valid returns state of the iterator, if at the end false it returned
	Valid() bool
	// Key returns the key of the current key/value pair, or nil if done. The caller
	// should not modify the contents of the returned slice, and its contents may
	// change on the next call to Next.
	Key() []byte
	// Value returns the value of the current key/value pair, or nil if done. The
	// caller should not modify the contents of the returned slice, and its contents
	// may change on the next call to Next.
	Value(value any) error
	// Close releases associated resources. Release should always succeed and can
	// be called multiple times without causing error.
	Close() error
}

// Iteratee wraps the NewIterator methods of a backing data store.
type Iteratee interface {
	// First creates a binary-alphabetical forward iterator starting with first item.
	// If the DB is empty the returned iterator returned is not valid (it.Valid() == false)
	First() Iterator
	// Last creates a binary-alphabetical reverse iterator starting with last item.
	// If the DB is empty the returned iterator returned is not valid (it.Valid() == false)
	Last() ReverseIterator
	// Find returns forward iterator to the closest binary-alphabetical match.
	// If no match or DB is empty the returned iterator returned is not valid (it.Valid() == false)
	Find(key []byte) Iterator
}

// DBTransaction key value database transaction
type DBTransaction interface {
	Writer
	// Commit commits all pending changes
	Commit() error
	// Rollback reverts everything and nothing is changed
	Rollback() error
}
