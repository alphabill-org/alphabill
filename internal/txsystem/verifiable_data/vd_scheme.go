package verifiable_data

import (
	"crypto"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/holiman/uint256"
	"hash"
)

type (
	Register interface {
		txsystem.GenericTransaction
		Attributes() []byte // TODO tbd
	}

	StateTree interface {
		AddItem(id *uint256.Int, owner state.Predicate, data state.UnitData, stateHash []byte) error
		Revert()
		Commit()
		GetRootHash() []byte
		GetBlockNumber() uint64
	}

	vdSchemeState struct {
		systemIdentifier []byte
		stateTree        StateTree
		hashAlgorithm    crypto.Hash
	}

	VerifiableDataUnit struct {
		V uint64
		T uint64
	}

	VerifiableDataSummary struct {
		v uint64
	}
)

var (
	log = logger.CreateForPackage()
)

func NewVDSchemeState(trustBase []string) (*vdSchemeState, error) {
	conf := &state.Config{
		ShardId:       []byte{1},
		HashAlgorithm: crypto.SHA256,
		TrustBase:     trustBase,
	}
	stateTree, err := state.New(conf)
	if err != nil {
		return nil, err
	}

	print(stateTree)

	dvState := &vdSchemeState{
		systemIdentifier: conf.ShardId,
		stateTree:        stateTree,
		hashAlgorithm:    conf.HashAlgorithm,
	}

	return dvState, nil
}

func (d *vdSchemeState) Process(gtx txsystem.GenericTransaction) error {
	println("GTX:", gtx)
	switch tx := gtx.(type) {
	case Register:
		log.Debug("Processing registration transaction %v", tx)
		err := d.stateTree.AddItem(tx.UnitID(), script.PredicateAlwaysFalse(), &VerifiableDataUnit{V: 0, T: d.stateTree.GetBlockNumber()}, tx.Hash(d.hashAlgorithm))
		if err != nil {
			return errors.Wrapf(err, "could not add item")
		}
		return nil
	default:
		return errors.New(fmt.Sprintf("Unknown type %T", gtx))
	}
}

func (u *VerifiableDataUnit) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(u.V))
	hasher.Write(util.Uint64ToBytes(u.T))
}

func (u *VerifiableDataUnit) Value() state.SummaryValue {
	return &VerifiableDataSummary{v: 0}
}

func (u *VerifiableDataSummary) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(u.v))
}

func (u *VerifiableDataSummary) Concatenate(state.SummaryValue, state.SummaryValue) state.SummaryValue {
	return &VerifiableDataSummary{v: 0}
}
