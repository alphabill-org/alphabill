package monolithic

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/types"
)

const (
	roundKey   = "round"
	certPrefix = "cert"
)

type (
	StateStore struct {
		db keyvaluedb.KeyValueDB
		mu sync.Mutex
	}
)

func certKey(id []byte) []byte {
	return append([]byte(certPrefix), id...)
}

func NewStateStore(storage keyvaluedb.KeyValueDB) *StateStore {
	return &StateStore{db: storage}
}

func (s *StateStore) IsEmpty() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return keyvaluedb.IsEmpty(s.db)
}

func (s *StateStore) save(newRound uint64, certificates map[types.SystemID32]*types.UnicityCertificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.StartTx()
	if err != nil {
		return fmt.Errorf("root state persist failed, %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err = tx.Write([]byte(roundKey), newRound); err != nil {
		return fmt.Errorf("root state failed to persist round  %v, %w, rollback", newRound, err)
	}
	// update certificates
	for id, uc := range certificates {
		key := certKey(id.ToSystemID())
		if err = tx.Write(key, uc); err != nil {
			return fmt.Errorf("root state failed to persist certificate for  %s, %w", id, err)
		}
	}
	// persist state
	return tx.Commit()
}

func (s *StateStore) Init(rg *genesis.RootGenesis) error {
	var certs = make(map[types.SystemID32]*types.UnicityCertificate)
	if rg == nil {
		return fmt.Errorf("store init failed, root genesis is nil")
	}
	for _, partition := range rg.Partitions {
		sysID, err := partition.SystemDescriptionRecord.SystemIdentifier.Id32()
		if err != nil {
			return err
		}
		certs[sysID] = partition.Certificate
	}
	return s.save(rg.GetRoundNumber(), certs)
}

func (s *StateStore) Update(newRound uint64, certificates map[types.SystemID32]*types.UnicityCertificate) error {
	// sanity check
	round, err := s.GetRound()
	if err != nil {
		return fmt.Errorf("failed to read root round")
	}
	if round >= newRound {
		return fmt.Errorf("error new round %v is in the past, latest stored round %v", newRound, round)
	}
	return s.save(newRound, certificates)
}

func (s *StateStore) GetLastCertifiedInputRecords() (ir map[types.SystemID32]*types.InputRecord, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ir = make(map[types.SystemID32]*types.InputRecord)
	it := s.db.Find([]byte(certPrefix))
	defer func() { err = errors.Join(err, it.Close()) }()
	for ; it.Valid() && strings.HasPrefix(string(it.Key()), certPrefix); it.Next() {
		var cert types.UnicityCertificate
		if err = it.Value(&cert); err != nil {
			return nil, fmt.Errorf("read certificate %v failed, %w", it.Key(), err)
		}
		// conversion to Id32 can only fail if system identifier length is not valid, this should never happen
		sysID, idErr := cert.UnicityTreeCertificate.SystemIdentifier.Id32()
		if idErr != nil {
			err = errors.Join(err, idErr)
			continue
		}
		ir[sysID] = cert.InputRecord
	}
	return ir, err
}

func (s *StateStore) GetCertificate(id types.SystemID32) (*types.UnicityCertificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cert types.UnicityCertificate
	sysID := id.ToSystemID()
	cKey := certKey(sysID)
	found, err := s.db.Read(cKey, &cert)
	if !found {
		return nil, fmt.Errorf("id %X not in DB", sysID)
	}
	if err != nil {
		return nil, fmt.Errorf("certificate id %X read failed, %w", sysID, err)
	}
	return &cert, nil
}

func (s *StateStore) GetRound() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	round := uint64(0)
	found, err := s.db.Read([]byte(roundKey), &round)
	if !found {
		return 0, fmt.Errorf("round not stored in db")
	}
	if err != nil {
		return 0, fmt.Errorf("round read failed, %w", err)
	}
	return round, nil
}
