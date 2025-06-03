package observability

import (
	"encoding/hex"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill-go-base/types"
)

const TxTypeKey attribute.Key = "tx.type"
const TxHashKey attribute.Key = "tx.hash"
const UnitIDKey attribute.Key = "unit_id"
const NodeIDKey attribute.Key = "service.node.name" // ECS convention

func Round(round uint64) attribute.KeyValue {
	return attribute.Int64("round", int64(round)) /* #nosec G115 its unlikely that value of round exceeds int64 max value */
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

func Shard(partition types.PartitionID, shard types.ShardID, extra ...attribute.KeyValue) metric.MeasurementOption {
	return metric.WithAttributeSet(attribute.NewSet(
		append(
			extra,
			attribute.Int64("partition", int64(partition)),
			attribute.String("shard", shard.String()),
		)...,
	))
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
