package certification

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

/*
Certification response is sent by the root partition to validators of a shard of a partition
as a response to a certification request message.
*/
type CertificationResponse struct {
	_         struct{} `cbor:",toarray"`
	Partition types.SystemID
	Shard     types.ShardID
	Technical TechnicalRecord
	UC        types.UnicityCertificate
}

func (cr *CertificationResponse) IsValid() error {
	if cr == nil {
		return errors.New("nil CertificationResponse")
	}
	if cr.Partition == 0 {
		return errors.New("partition ID is unassigned")
	}
	if cr.UC.UnicityTreeCertificate == nil {
		return errors.New("UnicityTreeCertificate is unassigned")
	}
	if utcP := cr.UC.UnicityTreeCertificate.SystemIdentifier; utcP != cr.Partition {
		return fmt.Errorf("partition %s doesn't match UnicityTreeCertificate partition %s", cr.Partition, utcP)
	}

	if err := cr.Technical.IsValid(); err != nil {
		return fmt.Errorf("invalid TechnicalRecord: %w", err)
	}
	if err := cr.Technical.HashMatches(cr.UC.TRHash); err != nil {
		return fmt.Errorf("comparing TechnicalRecord hash to UC.TRHash: %w", err)
	}

	return nil
}

func (cr *CertificationResponse) SetTechnicalRecord(tr TechnicalRecord) error {
	h, err := tr.Hash()
	if err != nil {
		return fmt.Errorf("calculating TR hash: %w", err)
	}
	cr.UC.TRHash = h
	cr.Technical = tr
	return nil
}
