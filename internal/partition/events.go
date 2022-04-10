package partition

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	// TopicPartitionUnicityCertificate topic is used to read unicity certificates received from the root chain.
	TopicPartitionUnicityCertificate = "partition.certificates"
	// TopicPartitionTransaction topic is used to receive transactions from wallets or other partition nodes.
	TopicPartitionTransaction = "partition.transactions"
	// TopicPC1O is used to send and receive block proposals.
	TopicPC1O = "partition.PC1-O"
	// TopicP1 is used to send block certification requests to the root chain
	TopicP1 = "root.P1"
)

type (

	// UnicityCertificateEvent contains an unicity certificate for the block proposal.
	UnicityCertificateEvent struct {
		Certificate *certificates.UnicityCertificate
	}

	// TransactionEvent contains a transaction that will be handled by the partition block proposal component.
	TransactionEvent struct {
		Transaction *transaction.Transaction
	}

	// PC1OEvent is a block proposal event. See Alphabill yellowpaper for more information.
	PC1OEvent struct {
		SystemIdentifier         []byte
		NodeIdentifier           peer.ID
		UnicityCertificateRecord *certificates.UnicityCertificate
		Transactions             []*transaction.Transaction
	}

	// P1Event is a message sent by the partition node to acquire unicity certificate for the block. See Alphabill
	// yellowpaper for more information.
	P1Event struct {
		SystemIdentifier []byte
		NodeIdentifier   peer.ID
		lucRoundNumber   uint64
		inputRecord      *certificates.InputRecord
	}
)
