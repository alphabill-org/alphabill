package observability

import (
	"encoding/hex"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
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
