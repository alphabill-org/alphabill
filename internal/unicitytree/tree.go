package unicitytree

import (
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/smt"
)

const systemIdentifierLength = 4

type (
	InputRecord struct {
		// TODO AB-114 System Input
		value []byte
	}

	Data struct {
		systemIdentifier []byte
		inputRecord      InputRecord
		// TODO AB-112 add System Description Record
	}

	UnicityTree struct {
		smt *smt.SMT
	}
)

// New creates a new unicity tree with given input records.
func New(hasher hash.Hash, d []*Data) (*UnicityTree, error) {
	data := make([]smt.Data, len(d))
	for i, id := range d {
		data[i] = id
	}
	smt, err := smt.New(hasher, systemIdentifierLength, data)
	if err != nil {
		return nil, err
	}
	return &UnicityTree{
		smt: smt,
	}, nil
}

func (d *Data) Key(_ int) []byte {
	return d.systemIdentifier
}

func (d *Data) Value() []byte {
	return d.inputRecord.Bytes()
}

func (ir *InputRecord) Bytes() []byte {
	return ir.value
}
