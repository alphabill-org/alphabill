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
		systemIdentifier        []byte
		inputRecord             InputRecord
		systemDescriptionRecord *SystemDescriptionRecord
	}

	UnicityTree struct {
		smt    *smt.SMT
		hasher hash.Hash
	}

	Certificate struct {
		systemIdentifier      []byte
		siblingHashes         [][]byte
		systemDescriptionHash []byte
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
		smt:    smt,
		hasher: hasher,
	}, nil
}

// GetCertificate returns an unicity tree certificate for given system identifier.
func (u *UnicityTree) GetCertificate(systemIdentifier []byte) (*Certificate, error) {
	if len(systemIdentifier) != systemIdentifierLength {
		return nil, ErrInvalidSystemIdentifierLength
	}
	path, data, err := u.smt.GetAuthPath(systemIdentifier)
	if err != nil {
		return nil, err
	}

	leafData, ok := data.(*Data)
	if !ok {
		return nil, errors.New("invalid data type, unicity tree leaf not is not of type *Data")
	}
	dhash := leafData.systemDescriptionRecord.hash(u.hasher)

	return &Certificate{
		systemIdentifier:      systemIdentifier,
		systemDescriptionHash: dhash,
		siblingHashes:         path,
	}, nil
}

func (d *Data) Key(_ int) []byte {
	return d.systemIdentifier
}

func (d *Data) AddToHasher(hasher hash.Hash) {
	d.inputRecord.AddToHasher(hasher)
	d.systemDescriptionRecord.addToHasher(hasher)
}

func (ir *InputRecord) AddToHasher(hasher hash.Hash) {
	hasher.Write(ir.PreviousHash)
	hasher.Write(ir.Hash)
	hasher.Write(ir.BlockHash)
	ir.SummaryValue.AddToHasher(hasher)
}
