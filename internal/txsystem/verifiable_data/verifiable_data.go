package verifiable_data

import (
	"crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	hasherUtil "gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type (
	RegisterTx interface {
		transaction.GenericTransaction
	}

	StateTree interface {
		AddItem(id *uint256.Int, owner state.Predicate, data state.UnitData, stateHash []byte) error
		GetRootHash() []byte
		GetBlockNumber() uint64
	}

	vdState struct {
		systemIdentifier []byte
		stateTree        StateTree
		hashAlgorithm    crypto.Hash
	}

	VerifiableDataUnit struct {
		DataHash    []byte
		BlockNumber uint64
	}
)

const zeroSummaryValue = state.Uint64SummaryValue(0)

var (
	log = logger.CreateForPackage()
)

func NewVerifiableDataTxSystem(trustBase []string) (*vdState, error) {
	conf := &state.Config{
		HashAlgorithm: crypto.SHA256,
		TrustBase:     trustBase,
	}
	stateTree, err := state.New(conf)
	if err != nil {
		return nil, err
	}

	vdState := &vdState{
		systemIdentifier: []byte{1}, // TODO AB-178 get system identifier somewhere
		stateTree:        stateTree,
		hashAlgorithm:    conf.HashAlgorithm,
	}

	return vdState, nil
}

func (d *vdState) Process(gtx transaction.GenericTransaction) error {
	err := txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{Tx: gtx, Bd: nil, SystemIdentifier: d.systemIdentifier, BlockNumber: d.stateTree.GetBlockNumber()})
	if err != nil {
		return err
	}
	switch tx := gtx.(type) {
	case RegisterTx:
		err = d.validateRegTx(tx)
		if err != nil {
			return err
		}
		log.Debug("Processing registration transaction %v", tx)
		err = d.stateTree.AddItem(tx.UnitID(), script.PredicateAlwaysFalse(), &VerifiableDataUnit{DataHash: hasherUtil.Sum256(tx.UnitID().Bytes()), BlockNumber: d.stateTree.GetBlockNumber()}, tx.Hash(d.hashAlgorithm))
		if err != nil {
			return errors.Wrap(err, "could not add item")
		}
		return nil
	default:
		return errors.Errorf("Unknown type %T", gtx)
	}
}

func (d *vdState) validateRegTx(tx RegisterTx) error {
	if len(tx.OwnerProof()) > 0 {
		return errors.New("Register transaction cannot have an owner proof")
	}
	return nil
}

func (u *VerifiableDataUnit) AddToHasher(hasher hash.Hash) {
	hasher.Write(u.DataHash)
	hasher.Write(util.Uint64ToBytes(u.BlockNumber))
}

func (u *VerifiableDataUnit) Value() state.SummaryValue {
	return zeroSummaryValue
}
