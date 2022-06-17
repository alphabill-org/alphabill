package txsystem

import (
	"google.golang.org/protobuf/proto"
)

func (x *Transaction) Bytes() ([]byte, error) {
	x.unknownFields = []byte{}
	// Setting "Deterministic" option guarantees that repeated serialization of
	// the same message will return the same bytes, and that different
	// processes of the same binary (which may be executing on different
	// machines) will serialize equal messages to the same bytes.
	//
	// NB! Note that the deterministic serialization is NOT canonical across
	// languages!
	// TODO implement a better transaction serialization.
	return proto.MarshalOptions{Deterministic: true}.Marshal(x)
}
