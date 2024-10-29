package observability

import (
	"encoding/hex"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"

	"github.com/alphabill-org/alphabill-go-base/types"
)

const TxTypeKey attribute.Key = "tx.type"
const TxHashKey attribute.Key = "tx.hash"
const UnitIDKey attribute.Key = "unit_id"
const NodeIDKey attribute.Key = "service.node.name" // ECS convention

func Round(round uint64) attribute.KeyValue {
	return attribute.Int64("round", int64(round))
}

func UnitID(id []byte) attribute.KeyValue {
	return UnitIDKey.String(hex.EncodeToString(id))
}

func TxHash(value []byte) attribute.KeyValue {
	return TxHashKey.String(hex.EncodeToString(value))
}

func PeerID(key attribute.Key, id peer.ID) attribute.KeyValue {
	return key.String(id.String())
}

func Partition(id types.PartitionID) attribute.KeyValue {
	return attribute.Int("partition", int(id))
}

/*
ErrStatus returns attribute named "status" with value "ok" if the param
err is nil and "err" when it is not.
*/
func ErrStatus(err error) attribute.KeyValue {
	status := "ok"
	if err != nil {
		status = "err"
	}
	return attribute.String("status", status)
}
