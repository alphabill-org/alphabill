package state

import (
	"crypto"
	"fmt"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"github.com/holiman/uint256"
)

var (
	ZeroIdentifier = uint256.NewInt(0)

	ErrIdIsNil            = errors.New("id is nil")
	ErrZIsNil             = errors.New("z is nil")
	ErrSummaryValueIsNil  = errors.New("summary value is nil")
	ErrRootNodeIsNil      = errors.New("root is nil")
	ErrRootNotCalculated  = errors.New("root not calculated")
	ErrInvalidCertificate = errors.New("invalid certificate")
)

type certificate struct {
	leftChildHash   []byte
	leftChildValue  SummaryValue
	rightChildHash  []byte
	rightChildValue SummaryValue
	path            []*pathItem // authentication path
}

type pathItem struct {
	id                *uint256.Int
	z                 []byte // H(x,H(ι′,φ,D))
	summaryValue      SummaryValue
	childStateHash    []byte
	childSummaryValue SummaryValue
}

// ExtractCertificate creates a RMA tree certificate.
func (tree *rmaTree) ExtractCertificate(id *uint256.Int) (*certificate, error) {
	if id == nil {
		return nil, ErrIdIsNil
	}
	if tree.root == nil {
		return nil, ErrRootNodeIsNil
	}
	if tree.root.recompute {
		return nil, ErrRootNotCalculated
	}
	if !tree.exists(id) {
		return nil, errors.Errorf(errStrItemDoesntExist, id)
	}
	var path []*pathItem
	n := tree.root

	hasher := tree.hashAlgorithm.New()
	for n != nil && !n.ID.Eq(id) && !n.ID.Eq(ZeroIdentifier) {
		// calculate  H(ι′,φ,D)
		idBytes := n.ID.Bytes32()
		hasher.Write(idBytes[:])
		hasher.Write(n.Content.Bearer)
		n.Content.Data.AddToHasher(hasher)
		// h= H(ι′,φ,D)
		h := hasher.Sum(nil)
		hasher.Reset()

		hasher.Write(n.Content.StateHash)
		hasher.Write(h)
		// z = H(x,H(ι′,φ,D))
		z := hasher.Sum(nil)
		hasher.Reset()
		c := compare(id, n.ID)
		var i *pathItem

		if c < 0 {
			i = &pathItem{
				id:                n.ID,
				z:                 z,
				summaryValue:      n.Content.Data.Value(),
				childStateHash:    n.RightChildHash(),
				childSummaryValue: n.RightChildSummary(),
			}
			n = n.Children[0]
		} else {
			i = &pathItem{
				id:                n.ID,
				z:                 z,
				summaryValue:      n.Content.Data.Value(),
				childStateHash:    n.LeftChildHash(),
				childSummaryValue: n.LeftChildSummary(),
			}
			n = n.Children[1]
		}
		path = append([]*pathItem{i}, path...)
	}
	cert := &certificate{
		path: path,
	}
	if n != nil && !n.ID.Eq(ZeroIdentifier) {
		left := n.Children[0]
		if left != nil {
			cert.leftChildHash = left.Hash
			cert.leftChildValue = left.SummaryValue
		} else {
			cert.leftChildHash = make([]byte, hasher.Size())
			cert.leftChildValue = nil
		}

		right := n.Children[1]
		if right != nil {
			cert.rightChildHash = right.Hash
			cert.rightChildValue = right.SummaryValue
		} else {
			cert.rightChildHash = make([]byte, hasher.Size())
			cert.rightChildValue = nil
		}

	}
	return cert, nil
}

// CompTreeCert calculates the root hash and summary value.
func (c *certificate) CompTreeCert(id *uint256.Int, z []byte, summary SummaryValue, hashAlgorithm crypto.Hash) ([]byte, SummaryValue, error) {
	if id == nil {
		return nil, nil, ErrIdIsNil
	}
	if z == nil {
		return nil, nil, ErrZIsNil
	}
	if summary == nil {
		return nil, nil, ErrSummaryValueIsNil
	}
	hasher := hashAlgorithm.New()
	var hash []byte
	var value SummaryValue

	// h←H(ι,z0,V0;hL,VL;hR,VR)
	idBytes := id.Bytes32()
	// Main hash
	hasher.Write(idBytes[:])
	hasher.Write(z)
	summary.AddToHasher(hasher)
	hasher.Write(c.leftChildHash)
	if c.leftChildValue != nil {
		c.leftChildValue.AddToHasher(hasher)
	}

	hasher.Write(c.rightChildHash)
	if c.rightChildValue != nil {
		c.rightChildValue.AddToHasher(hasher)
	}

	hash = hasher.Sum(nil)

	// V←FS(V0,VL,VR)
	value = summary.Concatenate(c.leftChildValue, c.rightChildValue)

	for _, p := range c.path {
		hasher.Reset()

		if id.Lt(p.id) {
			idBytes := p.id.Bytes32()
			hasher.Write(idBytes[:])
			hasher.Write(p.z)
			p.summaryValue.AddToHasher(hasher)
			hasher.Write(hash)
			value.AddToHasher(hasher)
			hasher.Write(p.childStateHash)
			p.childSummaryValue.AddToHasher(hasher)
			hash = hasher.Sum(nil)
			hasher.Reset()
			value = p.summaryValue.Concatenate(value, p.childSummaryValue)
		} else if id.Gt(p.id) {
			idBytes := p.id.Bytes32()
			hasher.Write(idBytes[:])
			hasher.Write(p.z)
			p.summaryValue.AddToHasher(hasher)
			hasher.Write(p.childStateHash)
			p.childSummaryValue.AddToHasher(hasher)
			hasher.Write(hash)
			value.AddToHasher(hasher)
			hash = hasher.Sum(nil)
			hasher.Reset()
			value = p.summaryValue.Concatenate(p.childSummaryValue, value)
		} else {
			return nil, nil, ErrInvalidCertificate
		}
	}

	return hash, value, nil
}

func (c *certificate) String() string {
	return fmt.Sprintf("certificate{path=%v, leftChildHash=%v, leftChildSummary=%v, rightChildHash=%v, rightChildSummary=%v}", c.path, c.leftChildHash, c.leftChildValue, c.rightChildValue, c.rightChildValue)
}

func (p *pathItem) String() string {
	return fmt.Sprintf("path{id=%v, z=%v, summary=%v, childStateHash=%v, childSummary=%v}", p.id, p.z, p.summaryValue, p.childStateHash, p.childSummaryValue)
}
