package partition

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var (
	ErrSystemIdentifierIsNil = errors.New("system identifier is nil")
	ErrStrTxIsNil            = "transaction is nil"
)

type (

	// TxValidator is used to validate generic transactions (e.g. timeouts, system identifiers, etc.). This validator
	// should not contain transaction system specific validation logic.
	TxValidator interface {
		Validate(tx txsystem.GenericTransaction, latestBlockNumber uint64) error
	}

	// UnicityCertificateValidator is used to validate certificates.UnicityCertificate.
	UnicityCertificateValidator interface {

		// Validate validates the given certificates.UnicityCertificate. Returns an error if given unicity certificate
		// is not valid.
		Validate(uc *certificates.UnicityCertificate) error
	}

	// BlockProposalValidator is used to validate block proposals.
	BlockProposalValidator interface {

		// Validate validates the given blockproposal.BlockProposal. Returns an error if given block proposal
		// is not valid.
		Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier) error
	}

	// DefaultUnicityCertificateValidator is a default implementation of UnicityCertificateValidator.
	DefaultUnicityCertificateValidator struct {
		systemIdentifier      []byte
		systemDescriptionHash []byte
		rootTrustBase         map[string]crypto.Verifier
		algorithm             gocrypto.Hash
	}

	// DefaultBlockProposalValidator is a default implementation of UnicityCertificateValidator.
	DefaultBlockProposalValidator struct {
		systemIdentifier      []byte
		systemDescriptionHash []byte
		rootTrustBase         map[string]crypto.Verifier
		algorithm             gocrypto.Hash
	}

	DefaultTxValidator struct {
		systemIdentifier []byte
	}
)

// NewDefaultTxValidator creates a new instance of default TxValidator.
func NewDefaultTxValidator(systemIdentifier []byte) (TxValidator, error) {
	if len(systemIdentifier) != 4 {
		return nil, fmt.Errorf("invalid transaction system identifier: expected 4 bytes, got %d", len(systemIdentifier))
	}
	return &DefaultTxValidator{
		systemIdentifier: systemIdentifier,
	}, nil
}

func (dtv *DefaultTxValidator) Validate(tx txsystem.GenericTransaction, latestBlockNumber uint64) error {
	if tx == nil {
		return errors.New("transaction is nil")
	}
	if !bytes.Equal(dtv.systemIdentifier, tx.SystemID()) {
		// transaction was not sent to correct transaction system
		return fmt.Errorf("expected %X, got %X: %w", dtv.systemIdentifier, tx.SystemID(), errInvalidSystemIdentifier)
	}

	if tx.Timeout() <= latestBlockNumber {
		// transaction is expired
		return fmt.Errorf("transaction has timed out: transaction timeout round is %d, current round is %d", tx.Timeout(), latestBlockNumber)
	}
	return nil
}

// NewDefaultUnicityCertificateValidator creates a new instance of default UnicityCertificateValidator.
func NewDefaultUnicityCertificateValidator(
	systemDescription *genesis.SystemDescriptionRecord,
	rootTrust map[string]crypto.Verifier,
	algorithm gocrypto.Hash,
) (UnicityCertificateValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if len(rootTrust) == 0 {
		return nil, certificates.ErrRootValidatorInfoMissing
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultUnicityCertificateValidator{
		systemIdentifier:      systemDescription.SystemIdentifier,
		rootTrustBase:         rootTrust,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (ucv *DefaultUnicityCertificateValidator) Validate(uc *certificates.UnicityCertificate) error {
	return uc.IsValid(ucv.rootTrustBase, ucv.algorithm, ucv.systemIdentifier, ucv.systemDescriptionHash)
}

// NewDefaultBlockProposalValidator creates a new instance of default BlockProposalValidator.
func NewDefaultBlockProposalValidator(
	systemDescription *genesis.SystemDescriptionRecord,
	rootTrust map[string]crypto.Verifier,
	algorithm gocrypto.Hash,
) (BlockProposalValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if len(rootTrust) == 0 {
		return nil, certificates.ErrRootValidatorInfoMissing
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultBlockProposalValidator{
		systemIdentifier:      systemDescription.SystemIdentifier,
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
		bpv.systemIdentifier,
		bpv.systemDescriptionHash,
	)
}
