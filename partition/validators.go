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
		Validate(tx *types.TransactionOrder, latestBlockNumber uint64) error
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
		partitionIdentifier   types.PartitionID
		systemDescriptionHash []byte
		rootTrustBase         types.RootTrustBase
		algorithm             gocrypto.Hash
	}

	// DefaultBlockProposalValidator is a default implementation of UnicityCertificateValidator.
	DefaultBlockProposalValidator struct {
		partitionIdentifier   types.PartitionID
		systemDescriptionHash []byte
		rootTrustBase         types.RootTrustBase
		algorithm             gocrypto.Hash
	}

	DefaultTxValidator struct {
		partitionIdentifier types.PartitionID
	}
)

var ErrTxTimeout = errors.New("transaction has timed out")
var errInvalidPartitionIdentifier = errors.New("invalid transaction partition identifier")

// NewDefaultTxValidator creates a new instance of default TxValidator.
func NewDefaultTxValidator(partitionIdentifier types.PartitionID) (TxValidator, error) {
	if partitionIdentifier == 0 {
		return nil, fmt.Errorf("invalid transaction partition identifier: %s", partitionIdentifier)
	}
	return &DefaultTxValidator{
		partitionIdentifier: partitionIdentifier,
	}, nil
}

func (dtv *DefaultTxValidator) Validate(tx *types.TransactionOrder, latestBlockNumber uint64) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}
	if dtv.partitionIdentifier != tx.PartitionID {
		// transaction was not sent to correct transaction system
		return fmt.Errorf("expected %s, got %s: %w", dtv.partitionIdentifier, tx.PartitionID, errInvalidPartitionIdentifier)
	}

	if tx.Timeout() <= latestBlockNumber {
		// transaction is expired
		return fmt.Errorf("transaction timeout round is %d, current round is %d: %w", tx.Timeout(), latestBlockNumber, ErrTxTimeout)
	}

	if n := len(tx.ReferenceNumber()); n > 32 {
		return fmt.Errorf("maximum allowed length of the ReferenceNumber is 32 bytes, got %d bytes", n)
	}

	return nil
}

// NewDefaultUnicityCertificateValidator creates a new instance of default UnicityCertificateValidator.
func NewDefaultUnicityCertificateValidator(
	systemDescription *types.PartitionDescriptionRecord,
	trustBase types.RootTrustBase,
	algorithm gocrypto.Hash,
) (UnicityCertificateValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if trustBase == nil {
		return nil, types.ErrRootValidatorInfoMissing
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultUnicityCertificateValidator{
		partitionIdentifier:   systemDescription.PartitionIdentifier,
		rootTrustBase:         trustBase,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (ucv *DefaultUnicityCertificateValidator) Validate(uc *types.UnicityCertificate) error {
	return uc.Verify(ucv.rootTrustBase, ucv.algorithm, ucv.partitionIdentifier, ucv.systemDescriptionHash)
}

// NewDefaultBlockProposalValidator creates a new instance of default BlockProposalValidator.
func NewDefaultBlockProposalValidator(
	systemDescription *types.PartitionDescriptionRecord,
	rootTrust types.RootTrustBase,
	algorithm gocrypto.Hash,
) (BlockProposalValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if rootTrust == nil {
		return nil, types.ErrRootValidatorInfoMissing
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultBlockProposalValidator{
		partitionIdentifier:   systemDescription.PartitionIdentifier,
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
		bpv.partitionIdentifier,
		bpv.systemDescriptionHash,
	)
}
