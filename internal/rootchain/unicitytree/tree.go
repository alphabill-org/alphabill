package unicitytree

import (
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/smt"
)

const systemIdentifierLength = 4

var ErrInvalidSystemIdentifierLength = errors.New("invalid system identifier length")

type (
	Data struct {
		SystemIdentifier            []byte
		InputRecord                 *certificates.InputRecord
		SystemDescriptionRecordHash []byte
	}

	UnicityTree struct {
		smt    *smt.SMT
		hasher hash.Hash
	}
)

// New creates a new unicity tree with given input records.
func New(hasher hash.Hash, d []*Data) (*UnicityTree, error) {
	data := make([]smt.Data, len(d))
	for i, id := range d {
		data[i] = id
	}
	s, err := smt.New(hasher, systemIdentifierLength, data)
	if err != nil {
		return nil, err
	}
	return &UnicityTree{
		smt:    s,
		hasher: hasher,
	}, nil
}

func (u *UnicityTree) GetRootHash() []byte {
	return u.smt.GetRootHash()
}

// GetCertificate returns an unicity tree certificate for given system identifier.
func (u *UnicityTree) GetCertificate(systemIdentifier []byte) (*certificates.UnicityTreeCertificate, error) {
	if len(systemIdentifier) != systemIdentifierLength {
		return nil, ErrInvalidSystemIdentifierLength
	}
	path, data, err := u.smt.GetAuthPath(systemIdentifier)
	if err != nil {
		return nil, err
	}

	leafData, ok := data.(*Data)
	if !ok {
		return nil, errors.New("invalid data type, unicity tree leaf node is not of type *Data")
	}
	dhash := leafData.SystemDescriptionRecordHash

	return &certificates.UnicityTreeCertificate{
		SystemIdentifier:      systemIdentifier,
		SystemDescriptionHash: dhash,
		SiblingHashes:         path,
	}, nil
}

func (d *Data) Key() []byte {
	return d.SystemIdentifier
}

func (d *Data) AddToHasher(hasher hash.Hash) {
	d.InputRecord.AddToHasher(hasher)
	hasher.Write(d.SystemDescriptionRecordHash)
}
