package storage

import (
	"fmt"
	"path"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/rootdb"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/rootdb/boltdb"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/rootdb/memorydb"
)

const (
	RootFile  = "root.db"
	BlockFile = "blocks.db"
	CertFile  = "certificates.db"
)

type (
	Storage struct {
		rootDB   rootdb.KeyValueDB
		blocksDB rootdb.KeyValueDB
		certsDB  rootdb.KeyValueDB
	}
)

func newMemStore() (*Storage, error) {
	return &Storage{
		rootDB:   memorydb.New(),
		blocksDB: memorydb.New(),
		certsDB:  memorydb.New(),
	}, nil
}

func newBoltDB(dbPath string) (*Storage, error) {
	rdb, err := boltdb.New(path.Join(dbPath, RootFile))
	if err != nil {
		return nil, fmt.Errorf("bolt db init failed, %w", err)
	}
	bdb, err := boltdb.New(path.Join(dbPath, BlockFile))
	if err != nil {
		return nil, fmt.Errorf("bolt block db init failed, %w", err)
	}
	cdb, err := boltdb.New(path.Join(dbPath, CertFile))
	if err != nil {
		return nil, fmt.Errorf("bolt certifcates db init failed, %w", err)
	}

	return &Storage{
		rootDB:   rdb,
		blocksDB: bdb,
		certsDB:  cdb,
	}, nil
}

func New(dbPath string) (*Storage, error) {
	if len(dbPath) == 0 {
		return newMemStore()
	} else {
		return newBoltDB(dbPath)
	}
}

func (s *Storage) GetBlocksDB() rootdb.KeyValueDB {
	return s.blocksDB
}

func (s *Storage) GetRootDB() rootdb.KeyValueDB {
	return s.rootDB
}

func (s *Storage) GetCertificatesDB() rootdb.KeyValueDB {
	return s.certsDB
}
