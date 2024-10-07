package certification

import "github.com/alphabill-org/alphabill-go-base/types"

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
