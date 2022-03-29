package partition

import "gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"

type (

	// TxValidator is used to validate generic transactions (e.g. timeouts, system identifiers, etc)
	TxValidator interface {
		Validate(tx *transaction.Transaction) error
	}

	// UnicityCertificateRecordValidator is used to validate received UnicityCertificateRecord.
	UnicityCertificateRecordValidator interface {
		Validate(ucr *UnicityCertificateRecord) error
	}
)
