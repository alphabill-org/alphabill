package state

import (
	"fmt"
	"io"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/tree/avl"
)

func readState(stateData io.Reader, udc UnitDataConstructor, opts ...Option) (*State, *Header, error) {
	options := loadOptions(opts...)
	crc32Reader := NewCRC32Reader(stateData, CBORChecksumLength)
	decoder := cbor.GetDecoder(crc32Reader)

	var header Header
	err := decoder.Decode(&header)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to decode header: %w", err)
	}

	root, err := readNodeRecords(decoder, udc, header.NodeRecordCount, options.hashAlgorithm)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to decode node records: %w", err)
	}

	var checksum []byte
	if err = decoder.Decode(&checksum); err != nil {
		return nil, nil, fmt.Errorf("unable to decode checksum: %w", err)
	}
	if util.BytesToUint32(checksum) != crc32Reader.Sum() {
		return nil, nil, fmt.Errorf("checksum mismatch")
	}

	hasher := newStateHasher(options.hashAlgorithm)
	t := avl.NewWithTraverserAndRoot[types.UnitID, Unit](hasher, root)
	state := &State{
		hashAlgorithm: options.hashAlgorithm,
		savepoints:    []*tree{t},
	}
	if _, _, err := state.CalculateRoot(); err != nil {
		return nil, nil, err
	}
	if header.UnicityCertificate != nil {
		if err := state.Commit(header.UnicityCertificate); err != nil {
			return nil, nil, fmt.Errorf("unable to commit recovered state: %w", err)
		}
	} else {
		// Must be genesis state, save it as committed tree, so it's never reverted.
		state.committedTree = t
	}

	return state, &header, nil
}
