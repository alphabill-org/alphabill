package partition

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
)

// TODO AB-111
type Genesis struct {
	InputRecord              *certificates.InputRecord
	UnicityCertificateRecord *UnicityCertificate
}
