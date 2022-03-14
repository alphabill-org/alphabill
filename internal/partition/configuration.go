package partition

import "time"

type Configuration struct {
	SystemIdentifier   []byte
	NodeIdentifier     uint64 // unique within a partition/shard.
	PartitionNodeCount uint64 // number of nodes in the cluster

	// after a node sees a UC which appoints a leader, the leader waits for t1 time units before stopping accepting new
	// transaction orders and creating a block proposal.
	T1Timeout time.Duration

	// TODO AB-122: Partition communication layer configuration

}
