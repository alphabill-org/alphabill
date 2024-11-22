package partition

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/libp2p/go-libp2p/core/peer"
)

type shardStore struct {
	db  keyvaluedb.KeyValueDB
	log *slog.Logger

	// Cached validators for the epoch
	mu              sync.RWMutex
	epoch           uint64
	epochValidators map[peer.ID]crypto.Verifier
}

func newShardStore(db keyvaluedb.KeyValueDB, log *slog.Logger) *shardStore {
	return &shardStore{
		db:  db,
		log: log,
	}
}

func (s *shardStore) StoreValidatorAssignmentRecord(v *ValidatorAssignmentRecord) error {
	s.log.Info(fmt.Sprintf("Registering VAR for epoch %d", v.EpochNumber))

	var prevVAR *ValidatorAssignmentRecord
	if v.EpochNumber > 0 {
		prevEpoch := v.EpochNumber-1
		var err error
		prevVAR, err = s.loadVAR(prevEpoch)
		if err != nil {
			return fmt.Errorf("failed to load VAR for epoch %d: %w", prevEpoch, err)
		}
	}
	if err := v.Verify(prevVAR); err != nil {
		return fmt.Errorf("failed to verify VAR for epoch %d: %w", v.EpochNumber, err)
	}
	if err := s.db.Write(epochToKey(v.EpochNumber), v); err != nil {
		return fmt.Errorf("saving VAR for epoch %d: %w", v.EpochNumber, err)
	}
	return nil
}

func (s *shardStore) LoadEpoch(epoch uint64) error {
	s.log.Info(fmt.Sprintf("Loading VAR for epoch %d", epoch))
	s.mu.Lock()
	defer s.mu.Unlock()

	vaRecord, err := s.loadVAR(epoch)
	if err != nil {
		return fmt.Errorf("failed to load VAR for epoch %d: %w", epoch, err)
	}

	validators := make(map[peer.ID]crypto.Verifier, len(vaRecord.Nodes))
	for _, vi := range vaRecord.Nodes {
		nodeID, err := peer.Decode(vi.NodeID)
		if err != nil {
			return fmt.Errorf("failed to decode nodeID %s: %w", vi.NodeID, err)
		}

		verifier, err := crypto.NewVerifierSecp256k1(vi.SigKey)
		if err != nil {
			return fmt.Errorf("failed to create signature verifier for %s: %w", vi.NodeID, err)
		}
		validators[nodeID] = verifier
	}
	s.epoch = vaRecord.EpochNumber
	s.epochValidators = validators
	return nil
}

func (s *shardStore) LoadedEpoch() uint64 {
	return s.epoch
}

func (s *shardStore) Validators() peer.IDSlice {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Collect(maps.Keys(s.epochValidators))
}

func (s *shardStore) RandomValidator() peer.ID {
	validators, err := randomNodeSelector(s.Validators(), 1)
	if err != nil {
		s.log.Warn("failed to select random validator", logger.Error(err))
		return UnknownLeader
	}
	return validators[0]
}

func (s *shardStore) IsValidator(peerID peer.ID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epochValidators[peerID] != nil
}

func (s *shardStore) Verifier(validator peer.ID) crypto.Verifier {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epochValidators[validator]
}

func (s *shardStore) loadVAR(epoch uint64) (*ValidatorAssignmentRecord, error) {
	v := &ValidatorAssignmentRecord{}
	found, err := s.db.Read(epochToKey(epoch), v)
	if err != nil {
		return nil, fmt.Errorf("reading VAR: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("VAR not found")
	}
	return v, nil
}

func epochToKey(epoch uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, epoch)
	return key
}
