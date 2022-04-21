package partition

import (
	gocrypto "crypto"

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

	// DefaultUnicityCertificateValidator is a default implementation of UnicityCertificateValidator.
	DefaultUnicityCertificateValidator struct {
		systemIdentifier      []byte
		systemDescriptionHash []byte
		trustBase             crypto.Verifier
		algorithm             gocrypto.Hash
	}
)

// NewDefaultUnicityCertificateValidator creates a new instance of DefaultUnicityCertificateValidator.
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

func (d *DefaultUnicityCertificateValidator) Validate(uc *certificates.UnicityCertificate) error {
	return uc.IsValid(d.trustBase, d.algorithm, d.systemIdentifier, d.systemDescriptionHash)
}
