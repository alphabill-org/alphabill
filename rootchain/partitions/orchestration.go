package partitions

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

func NewOrchestration(seed *genesis.RootGenesis) Orchestration {
	return Orchestration{seed}
}

type Orchestration struct {
	seed *genesis.RootGenesis
}

func (o Orchestration) ShardEpoch(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error) {
	for _, pg := range o.seed.Partitions {
		// TODO: support multi-shard partitions
		if pg.PartitionDescription.PartitionIdentifier == partition {
			return pg.Certificate.InputRecord.Epoch, nil
		}
	}
	return 0, fmt.Errorf("no configuration for %s - %s round %d", partition, shard, round)
}

func (o Orchestration) ShardConfig(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error) {
	for _, pg := range o.seed.Partitions {
		// TODO: support multi-shard partitions
		if pg.PartitionDescription.PartitionIdentifier == partition {
			return pg, nil
		}
	}
	return nil, fmt.Errorf("no configuration for %s - %s epoch %d", partition, shard, epoch)
}

func (o Orchestration) RoundPartitions(rootRound uint64) ([]*genesis.GenesisPartitionRecord, error) {
	return o.seed.Partitions, nil
}
