package partition

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
)

type (
	// TxValidator is used to validate generic transactions (e.g. timeouts, partition identifiers, etc.). This validator
	// should not contain transaction system specific validation logic.
	TxValidator interface {
		Validate(tx *types.TransactionOrder, currentRoundNumber uint64) error
	}

	// UnicityCertificateValidator is used to validate certificates.UnicityCertificate.
	UnicityCertificateValidator interface {
		// Validate validates the given UC. Returns an error if UC is not valid.
		Validate(uc *types.UnicityCertificate, shardConfHash []byte) error
	}

	// BlockProposalValidator is used to validate block proposals.
	BlockProposalValidator interface {
		// Validate validates the given blockproposal.BlockProposal. Returns an error if given block proposal
		// is not valid.
		Validate(bp *blockproposal.BlockProposal, sigVerifier crypto.Verifier, shardConfHash []byte) error
	}

	// DefaultUnicityCertificateValidator is a default implementation of UnicityCertificateValidator.
	DefaultUnicityCertificateValidator struct {
		partitionID types.PartitionID
		shardID     types.ShardID
		trustBase   types.RootTrustBase
		hashAlg     gocrypto.Hash
	}

	// DefaultBlockProposalValidator is a default implementation of UnicityCertificateValidator.
	DefaultBlockProposalValidator struct {
		partitionID types.PartitionID
		shardID     types.ShardID
		trustBase   types.RootTrustBase
		hashAlg     gocrypto.Hash
	}

	DefaultTxValidator struct {
		partitionID types.PartitionID
	}
)

var ErrTxTimeout = errors.New("transaction has timed out")
var errInvalidPartitionID = errors.New("invalid transaction partition identifier")

// NewDefaultTxValidator creates a new instance of default TxValidator.
func NewDefaultTxValidator(partitionID types.PartitionID) (TxValidator, error) {
	if partitionID == 0 {
		return nil, fmt.Errorf("invalid transaction partition identifier: %s", partitionID)
	}
	return &DefaultTxValidator{
		partitionID: partitionID,
	}, nil
}

func (dtv *DefaultTxValidator) Validate(tx *types.TransactionOrder, currentRoundNumber uint64) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}
	if dtv.partitionID != tx.PartitionID {
		// transaction was not sent to correct transaction system
		return fmt.Errorf("expected %s, got %s: %w", dtv.partitionID, tx.PartitionID, errInvalidPartitionID)
	}

	if tx.Timeout() < currentRoundNumber {
		// transaction is expired
		return fmt.Errorf("transaction timeout round is %d, current round is %d: %w", tx.Timeout(), currentRoundNumber, ErrTxTimeout)
	}

	if n := len(tx.ReferenceNumber()); n > 32 {
		return fmt.Errorf("maximum allowed length of the ReferenceNumber is 32 bytes, got %d bytes", n)
	}

	return nil
}

// NewDefaultUnicityCertificateValidator creates a new instance of default UnicityCertificateValidator.
func NewDefaultUnicityCertificateValidator(
	partitionID types.PartitionID,
	shardID types.ShardID,
	trustBase types.RootTrustBase,
	hashAlg gocrypto.Hash,
) (UnicityCertificateValidator, error) {
	if trustBase == nil {
		return nil, types.ErrRootValidatorInfoMissing
	}
	return &DefaultUnicityCertificateValidator{
		partitionID: partitionID,
		shardID:     shardID,
		trustBase:   trustBase,
		hashAlg:     hashAlg,
	}, nil
}

func (ucv *DefaultUnicityCertificateValidator) Validate(uc *types.UnicityCertificate, shardConfHash []byte) error {
	return uc.Verify(ucv.trustBase, ucv.hashAlg, ucv.partitionID, shardConfHash)
}

// NewDefaultBlockProposalValidator creates a new instance of default BlockProposalValidator.
func NewDefaultBlockProposalValidator(
	partitionID types.PartitionID,
	shardID types.ShardID,
	trustBase types.RootTrustBase,
	hashAlg gocrypto.Hash,
) (BlockProposalValidator, error) {
	if trustBase == nil {
		return nil, types.ErrRootValidatorInfoMissing
	}
	return &DefaultBlockProposalValidator{
		partitionID: partitionID,
		shardID:     shardID,
		trustBase:   trustBase,
		hashAlg:     hashAlg,
	}, nil
}

func (bpv *DefaultBlockProposalValidator) Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier, shardConfHash []byte) error {
	return bp.IsValid(
		nodeSignatureVerifier,
		bpv.trustBase,
		bpv.hashAlg,
		bpv.partitionID,
		shardConfHash,
	)
}
