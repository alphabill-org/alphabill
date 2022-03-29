package partition

import "gitdc.ee.guardtime.com/alphabill/alphabill/internal/unicitytree"

// TODO AB-111
type Genesis struct {
	InputRecord              *unicitytree.InputRecord
	UnicityCertificateRecord *UnicityCertificateRecord
}
