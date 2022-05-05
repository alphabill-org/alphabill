package eventbus

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (

	// TopicPartitionUnicityCertificate topic is used to read unicity certificates received from the root chain.
	TopicPartitionUnicityCertificate = "partition.certificates"

	// TopicPartitionTransaction topic is used to receive transactions from wallets or other partition nodes.
	TopicPartitionTransaction = "partition.transactions"

	// TopicBlockProposalInput is used to receive block proposals from the leader.
	TopicBlockProposalInput = "partition.proposal.input"

	// TopicBlockProposalOutput is used to send block proposals to followers.
	TopicBlockProposalOutput = "partition.proposal.output"

	// TopicLeaders is used to receive leader change events.
	TopicLeaders = "partition.leaders"

	// TopicP1 is used to send block certification requests to the root chain
	TopicP1 = "root.P1"
)

var defaultTopics = []string{
	TopicPartitionUnicityCertificate,
	TopicPartitionTransaction,
	TopicBlockProposalInput,
	TopicBlockProposalOutput,
	TopicLeaders,
	TopicP1,
}

type (

	// UnicityCertificateEvent contains an unicity certificate for the block proposal.
	UnicityCertificateEvent struct {
		Certificate *certificates.UnicityCertificate
	}

	// TransactionEvent contains a transaction that will be handled by the partition block proposal component.
	TransactionEvent struct {
		Transaction *transaction.Transaction
	}

	NewLeaderEvent struct {
		NewLeader peer.ID
	}

	// BlockProposalEvent is a block proposal event. See Alphabill yellowpaper for more information.
	BlockProposalEvent struct {
		*blockproposal.BlockProposal
	}

	// BlockCertificationEvent is a message sent by the partition node to acquire unicity certificate for the block. See Alphabill
	// yellowpaper for more information.
	BlockCertificationEvent struct {
		Req *p1.P1Request
		/*SystemIdentifier []byte
		NodeIdentifier   peer.ID
		LucRoundNumber   uint64
		InputRecord      *certificates.InputRecord*/
	}
)
