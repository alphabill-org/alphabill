package state

import (
	"github.com/alphabill-org/alphabill/validator/internal/types"
)

type LogPruner struct {
	unitsToPrune map[uint64]map[string]types.UnitID
	state        *State
}

func NewLogPruner(s *State) *LogPruner {
	return &LogPruner{
		unitsToPrune: map[uint64]map[string]types.UnitID{},
		state:        s,
	}
}

func (s *LogPruner) Count(blockNr uint64) int {
	return len(s.unitsToPrune[blockNr])
}

func (s *LogPruner) Add(blockNr uint64, id types.UnitID) {
	if _, f := s.unitsToPrune[blockNr]; !f {
		s.unitsToPrune[blockNr] = map[string]types.UnitID{}
	}
	s.unitsToPrune[blockNr][id.String()] = id
}

func (s *LogPruner) Prune(blockNr uint64) error {
	units := s.unitsToPrune[blockNr]
	for _, id := range units {
		if err := s.state.PruneLog(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *LogPruner) Remove(blockNr uint64) {
	delete(s.unitsToPrune, blockNr)
}
