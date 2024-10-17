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
	if err := cr.Technical.IsValid(); err != nil {
		return fmt.Errorf("invalid TechnicalRecord: %w", err)
	}
	return nil
}
