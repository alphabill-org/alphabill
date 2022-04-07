package partition

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

type (

	// TxValidator is used to validate generic transactions (e.g. timeouts, system identifiers, etc)
	TxValidator interface {
		Validate(tx *transaction.Transaction) error
	}

	// UnicityCertificateValidator is used to validate received UnicityCertificate.
	UnicityCertificateValidator interface {
		Validate(uc *certificates.UnicityCertificate) error
	}
)
