package storage

import (
	"github.com/alphabill-org/alphabill/internal/util"
)

const (
	certPrefix  = "cert_"
	blockPrefix = "block_"
)

func certKey(id []byte) []byte {
	return append([]byte(certPrefix), id...)
}

func blockKey(round uint64) []byte {
	return append([]byte(blockPrefix), util.Uint64ToBytes(round)...)
}
