package tokens

import (
	"bytes"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func Test_runChainedPredicates(t *testing.T) {
	// the tx order is only used as argument of the "exec" callback
	// so it is ok to share it
	txo := &types.TransactionOrder{}
	ucErr := errors.New("unexpected call")

	// coverage: 69.3% of statements

	t.Run("nothing to see, move on", func(t *testing.T) {
		// case where there is nothing to eval and no params
		// sent in - IOW valid case of nothing to do
		err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
			nil,
			txo.AuthProofSigBytes,
			nil, // type ID nil signals no root item to start the chain
			nil, // no arguments for chained predicates
			func(pred types.PredicateBytes, arg []byte, sigBytesFn func() ([]byte, error), tec predicates.TxContext) error {
				t.Error("unexpected call")
				return ucErr
			},
			func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
				t.Error("unexpected call")
				return nil, nil
			},
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				t.Error("unexpected call")
				return nil, ucErr
			},
		)
		require.NoError(t, err)
	})

	t.Run("unexpected arguments sent to the runner", func(t *testing.T) {
		// sending in more arguments than there is chained predicates
		err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
			nil,
			txo.AuthProofSigBytes,
			nil,           // type ID nil signals no root item to start the chain
			[][]byte{{0}}, // sending argument should cause error as there is no predicates
			func(pred types.PredicateBytes, arg []byte, sigBytesFn func() ([]byte, error), tec predicates.TxContext) error {
				t.Error("unexpected call")
				return ucErr
			},
			func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
				t.Error("unexpected call")
				return nil, nil
			},
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				t.Error("unexpected call")
				return nil, ucErr
			},
		)
		require.EqualError(t, err, `got arguments for 1 predicates, executed 0 predicates`)
	})

	t.Run("not sending arguments for predicate", func(t *testing.T) {
		// sending in less arguments than there is chained predicates
		err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
			nil,
			txo.AuthProofSigBytes,
			[]byte{0, 0, 1}, // non-nil type ID signals ther is predicates to eval...
			nil,             // ...but we pass in no arguments for chained predicates
			func(pred types.PredicateBytes, arg []byte, sigBytesFn func() ([]byte, error), tec predicates.TxContext) error {
				t.Error("unexpected call")
				return ucErr
			},
			func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
				t.Error("unexpected call")
				return nil, nil
			},
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				t.Error("unexpected call")
				return nil, ucErr
			},
		)
		require.EqualError(t, err, `got arguments for 0 predicates but chain is longer, no argument for unit "000001" predicate`)
	})

	t.Run("unit doesn't exist", func(t *testing.T) {
		// the unit supposedly part of the chain doesn't exist in the state
		noData := errors.New("no such unit")
		err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
			nil,
			txo.AuthProofSigBytes,
			[]byte{0, 0, 1},
			[][]byte{{5}},
			func(pred types.PredicateBytes, arg []byte, sigBytesFn func() ([]byte, error), tec predicates.TxContext) error {
				t.Error("unexpected call")
				return ucErr
			},
			func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) {
				t.Error("unexpected call")
				return nil, nil
			},
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, []byte{0, 0, 1}) {
					t.Errorf("unexpected type id %v", id)
				}
				return nil, noData
			},
		)
		require.ErrorIs(t, err, noData)
		require.EqualError(t, err, `read [0] unit ID "000001" data: no such unit`)
	})

	t.Run("nil predicate", func(t *testing.T) {
		// iterator callback returns nil for the predicate
		d := &tokens.NonFungibleTokenTypeData{}
		unit := state.NewUnit([]byte{7, 7, 7}, d)
		err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
			nil,
			txo.AuthProofSigBytes,
			[]byte{0, 0, 1},
			[][]byte{{5}},
			func(pred types.PredicateBytes, arg []byte, sigBytesFn func() ([]byte, error), tec predicates.TxContext) error {
				t.Error("unexpected call")
				return ucErr
			},
			func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) { return nil, nil }, // returning nil predicate!
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, []byte{0, 0, 1}) {
					t.Errorf("unexpected type id %v", id)
				}
				return unit, nil
			},
		)
		require.EqualError(t, err, `unexpected nil predicate`)
	})

	t.Run("eval predicate fails", func(t *testing.T) {
		// exec predicate callback returns error
		expErr := errors.New("eval predicate fails")
		d := &tokens.NonFungibleTokenTypeData{}
		unit := state.NewUnit([]byte{7, 7, 7}, d)
		err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
			nil,
			txo.AuthProofSigBytes,
			[]byte{0, 0, 1},
			[][]byte{{5}},
			func(pred types.PredicateBytes, arg []byte, sigBytesFn func() ([]byte, error), tec predicates.TxContext) error {
				if !bytes.Equal(pred, []byte("predicate")) {
					t.Errorf("unexpected predicate: %v", pred)
				}
				if !bytes.Equal(arg, []byte{5}) {
					t.Errorf("unexpected predicate argument: %v", arg)
				}
				return expErr
			},
			func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) { return nil, []byte("predicate") }, // no next item
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, []byte{0, 0, 1}) {
					t.Errorf("unexpected type id %v", id)
				}
				return unit, nil
			},
		)
		require.ErrorIs(t, err, expErr)
	})

	t.Run("successful eval of one round", func(t *testing.T) {
		// successfully evaluating chain with one item
		d := &tokens.NonFungibleTokenTypeData{}
		unit := state.NewUnit([]byte{7, 7, 7}, d)
		err := runChainedPredicates[*tokens.NonFungibleTokenTypeData](
			testctx.NewMockExecutionContext(),
			txo.AuthProofSigBytes,
			[]byte{0, 0, 1},
			[][]byte{{5}},
			func(pred types.PredicateBytes, arg []byte, sigBytesFn func() ([]byte, error), env predicates.TxContext) error {
				if !bytes.Equal(pred, []byte("predicate")) {
					t.Errorf("unexpected predicate: %v", pred)
				}
				if !bytes.Equal(arg, []byte{5}) {
					t.Errorf("unexpected predicate argument: %v", arg)
				}
				return nil
			},
			func(d *tokens.NonFungibleTokenTypeData) (types.UnitID, []byte) { return nil, []byte("predicate") }, // no next item
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, []byte{0, 0, 1}) {
					t.Errorf("unexpected type id %v", id)
				}
				return unit, nil
			},
		)
		require.NoError(t, err)
	})
}

func Test_getUnitData(t *testing.T) {
	t.Run("getUnit returns error", func(t *testing.T) {
		expErr := errors.New("no such unit")
		data, err := getUnitData[*mockUnitData](
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				return nil, expErr
			},
			[]byte{1, 2, 3, 4, 5, 6},
		)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, data)
	})

	t.Run("unit data is of wrong type", func(t *testing.T) {
		unitID := []byte{1, 2, 3, 4, 5, 6}
		d := &tokens.NonFungibleTokenTypeData{}
		unit := state.NewUnit([]byte{7, 7, 7}, d)
		data, err := getUnitData[*mockUnitData](
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, unitID) {
					t.Errorf("unexpected unit id: %v", id)
				}
				return unit, nil
			},
			unitID,
		)
		require.EqualError(t, err, `expected unit 010203040506 data to be *tokens.mockUnitData got *tokens.NonFungibleTokenTypeData`)
		require.Nil(t, data)
	})

	t.Run("success", func(t *testing.T) {
		unitID := []byte{1, 2, 3, 4, 5, 6}
		d := &tokens.NonFungibleTokenTypeData{Symbol: "SYM", Name: "my NFT"}
		unit := state.NewUnit([]byte{7, 7, 7}, d)
		data, err := getUnitData[*tokens.NonFungibleTokenTypeData](
			func(id types.UnitID, committed bool) (*state.Unit, error) {
				if !bytes.Equal(id, unitID) {
					t.Errorf("unexpected unit id: %v", id)
				}
				return unit, nil
			},
			unitID,
		)
		require.NoError(t, err)
		require.Equal(t, d, data)
	})
}
