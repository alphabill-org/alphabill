package partition

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
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

func (s *shardStore) StoreShardConf(shardConf *types.PartitionDescriptionRecord) error {
	var prevShardConf *types.PartitionDescriptionRecord
	if shardConf.Epoch > 0 {
		prevEpoch := shardConf.Epoch-1
		var err error
		prevShardConf, err = s.loadShardConf(prevEpoch)
		if err != nil {
			return fmt.Errorf("failed to load shard conf for previous epoch %d: %w", prevEpoch, err)
		}
	}
	if err := shardConf.Verify(prevShardConf); err != nil {
		return fmt.Errorf("failed to verify shard conf for epoch %d: %w", shardConf.Epoch, err)
	}
	if err := s.db.Write(epochToKey(shardConf.Epoch), shardConf); err != nil {
		return fmt.Errorf("saving shard conf for epoch %d: %w", shardConf.Epoch, err)
	}
	return nil
}

func (s *shardStore) LoadEpoch(epoch uint64) error {
	s.log.Info(fmt.Sprintf("Loading shard conf for epoch %d", epoch))
	s.mu.Lock()
	defer s.mu.Unlock()

	shardConf, err := s.loadShardConf(epoch)
	if err != nil {
		return fmt.Errorf("failed to load shard conf for epoch %d: %w", epoch, err)
	}

	validators := make(map[peer.ID]crypto.Verifier, len(shardConf.Validators))
	for _, vi := range shardConf.Validators {
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
	s.epoch = shardConf.Epoch
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

func (s *shardStore) loadShardConf(epoch uint64) (*types.PartitionDescriptionRecord, error) {
	v := &types.PartitionDescriptionRecord{}
	found, err := s.db.Read(epochToKey(epoch), v)
	if err != nil {
		return nil, fmt.Errorf("reading shard conf: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("shard conf not found")
	}
	return v, nil
}

func epochToKey(epoch uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, epoch)
	return key
}
