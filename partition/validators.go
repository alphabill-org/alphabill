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
		// Validate validates the given certificates.UnicityCertificate. Returns an error if given unicity certificate
		// is not valid.
		Validate(uc *types.UnicityCertificate) error
	}

	// BlockProposalValidator is used to validate block proposals.
	BlockProposalValidator interface {
		// Validate validates the given blockproposal.BlockProposal. Returns an error if given block proposal
		// is not valid.
		Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier) error
	}

	// DefaultUnicityCertificateValidator is a default implementation of UnicityCertificateValidator.
	DefaultUnicityCertificateValidator struct {
		partitionID           types.PartitionID
		systemDescriptionHash []byte
		rootTrustBase         types.RootTrustBase
		algorithm             gocrypto.Hash
	}

	// DefaultBlockProposalValidator is a default implementation of UnicityCertificateValidator.
	DefaultBlockProposalValidator struct {
		partitionID           types.PartitionID
		systemDescriptionHash []byte
		rootTrustBase         types.RootTrustBase
		algorithm             gocrypto.Hash
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
	partitionDescription *types.PartitionDescriptionRecord,
	trustBase types.RootTrustBase,
	algorithm gocrypto.Hash,
) (UnicityCertificateValidator, error) {
	if err := partitionDescription.IsValid(); err != nil {
		return nil, err
	}
	if trustBase == nil {
		return nil, types.ErrRootValidatorInfoMissing
	}
	h, err := partitionDescription.Hash(algorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to hash partition description: %w", err)
	}
	return &DefaultUnicityCertificateValidator{
		partitionID:   partitionDescription.PartitionID,
		rootTrustBase:         trustBase,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (ucv *DefaultUnicityCertificateValidator) Validate(uc *types.UnicityCertificate) error {
	return uc.Verify(ucv.rootTrustBase, ucv.algorithm, ucv.partitionID, ucv.systemDescriptionHash)
}

// NewDefaultBlockProposalValidator creates a new instance of default BlockProposalValidator.
func NewDefaultBlockProposalValidator(
	partitionDescription *types.PartitionDescriptionRecord,
	rootTrust types.RootTrustBase,
	algorithm gocrypto.Hash,
) (BlockProposalValidator, error) {
	if err := partitionDescription.IsValid(); err != nil {
		return nil, err
	}
	if rootTrust == nil {
		return nil, types.ErrRootValidatorInfoMissing
	}
	h, err := partitionDescription.Hash(algorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to hash partition description: %w", err)
	}
	return &DefaultBlockProposalValidator{
		partitionID:   partitionDescription.PartitionID,
		rootTrustBase:         rootTrust,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (bpv *DefaultBlockProposalValidator) Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier) error {
	return bp.IsValid(
		nodeSignatureVerifier,
		bpv.rootTrustBase,
		bpv.algorithm,
		bpv.partitionID,
		bpv.systemDescriptionHash,
	)
}
