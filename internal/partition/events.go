package partition

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/unicitytree"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	TopicPartitionUnicityCertificate = "partition.certificates"
	TopicPartitionTransaction        = "partition.transactions"
	TopicPC10                        = "partition.PC1-O"
	TopicP1                          = "root.P1"
)

type (
	UnicityCertificateRecordEvent struct {
		Certificate *UnicityCertificateRecord
	}

	TransactionEvent struct {
		Transaction *transaction.Transaction
	}

	PC1OEvent struct {
		SystemIdentifier         []byte
		NodeIdentifier           peer.ID
		UnicityCertificateRecord *UnicityCertificateRecord
		Transactions             []*transaction.Transaction
	}

	P1Event struct {
		SystemIdentifier []byte
		NodeIdentifier   peer.ID
		lucRoundNumber   uint64
		inputRecord      *unicitytree.InputRecord
	}
)
