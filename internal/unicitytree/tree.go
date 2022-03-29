package unicitytree

import (
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/smt"
)

const systemIdentifierLength = 4

var ErrInvalidSystemIdentifierLength = errors.New("invalid system identifier length")

type (
	InputRecord struct {
		PreviousHash []byte             // previously certified root hash
		Hash         []byte             // root hash to be certified
		BlockHash    []byte             // hash of the block
		SummaryValue state.SummaryValue // summary value to be certified
	}

	Data struct {
		SystemIdentifier []byte
		InputRecord      *InputRecord
		// TODO AB-112 add System Description Record
	}

	UnicityTree struct {
		smt *smt.SMT
	}

	Certificate struct {
		SystemIdentifier []byte
		siblingHashes    [][]byte
		// TODO AB-112 add System Description Record hash
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

// GetCertificate returns an unicity tree certificate for given system identifier.
func (u *UnicityTree) GetCertificate(systemIdentifier []byte) (*Certificate, error) {
	if len(systemIdentifier) != systemIdentifierLength {
		return nil, ErrInvalidSystemIdentifierLength
	}
	path, err := u.smt.GetAuthPath(systemIdentifier)
	if err != nil {
		return nil, err
	}
	return &Certificate{
		SystemIdentifier: systemIdentifier,
		siblingHashes:    path,
	}, nil
}

func (d *Data) Key(_ int) []byte {
	return d.SystemIdentifier
}

func (d *Data) AddToHasher(hasher hash.Hash) {
	d.InputRecord.AddToHasher(hasher)
	// TODO AB-112 add system description record hash
}

func (ir *InputRecord) AddToHasher(hasher hash.Hash) {
	hasher.Write(ir.PreviousHash)
	hasher.Write(ir.Hash)
	hasher.Write(ir.BlockHash)
	ir.SummaryValue.AddToHasher(hasher)
}
