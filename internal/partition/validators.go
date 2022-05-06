package partition

import (
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
)

type (

	// TxValidator is used to validate generic transactions (e.g. timeouts, system identifiers, etc)
	TxValidator interface {
		Validate(tx *transaction.Transaction) error
	}

	// UnicityCertificateValidator is used to validate certificates.UnicityCertificate.
	UnicityCertificateValidator interface {

		// Validate validates the given certificates.UnicityCertificate. Returns an error if given unicity certificate
		// is invalid.
		Validate(uc *certificates.UnicityCertificate) error
	}

	// BlockProposalValidator is used to validate block proposals.
	BlockProposalValidator interface {

		// Validate validates the given blockproposal.BlockProposal. Returns an error if given block proposal
		// is invalid.
		Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier) error
	}

	// DefaultUnicityCertificateValidator is a default implementation of UnicityCertificateValidator.
	DefaultUnicityCertificateValidator struct {
		systemIdentifier      []byte
		systemDescriptionHash []byte
		trustBase             crypto.Verifier
		algorithm             gocrypto.Hash
	}

	// DefaultBlockProposalValidator is a default implementation of UnicityCertificateValidator.
	DefaultBlockProposalValidator struct {
		systemIdentifier      []byte
		systemDescriptionHash []byte
		trustBase             crypto.Verifier
		algorithm             gocrypto.Hash
	}
)

// NewDefaultUnicityCertificateValidator creates a new instance of BlockProposalValidator.
func NewDefaultUnicityCertificateValidator(
	systemDescription *genesis.SystemDescriptionRecord,
	trustBase crypto.Verifier,
	algorithm gocrypto.Hash,
) (*DefaultUnicityCertificateValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if trustBase == nil {
		return nil, certificates.ErrVerifierIsNil
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultUnicityCertificateValidator{
		systemIdentifier:      systemDescription.SystemIdentifier,
		trustBase:             trustBase,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (ucv *DefaultUnicityCertificateValidator) Validate(uc *certificates.UnicityCertificate) error {
	return uc.IsValid(ucv.trustBase, ucv.algorithm, ucv.systemIdentifier, ucv.systemDescriptionHash)
}

// NewDefaultBlockProposalValidator creates a new instance of DefaultBlockProposalValidator.
func NewDefaultBlockProposalValidator(
	systemDescription *genesis.SystemDescriptionRecord,
	trustBase crypto.Verifier,
	algorithm gocrypto.Hash,
) (*DefaultBlockProposalValidator, error) {
	if err := systemDescription.IsValid(); err != nil {
		return nil, err
	}
	if trustBase == nil {
		return nil, certificates.ErrVerifierIsNil
	}
	h := systemDescription.Hash(algorithm)
	return &DefaultBlockProposalValidator{
		systemIdentifier:      systemDescription.SystemIdentifier,
		trustBase:             trustBase,
		systemDescriptionHash: h,
		algorithm:             algorithm,
	}, nil
}

func (bpv *DefaultBlockProposalValidator) Validate(bp *blockproposal.BlockProposal, nodeSignatureVerifier crypto.Verifier) error {
	return bp.IsValid(
		nodeSignatureVerifier,
		bpv.trustBase,
		bpv.algorithm,
		bpv.systemIdentifier,
		bpv.systemDescriptionHash,
	)
}
