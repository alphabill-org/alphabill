package statedb

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

var initialAccountAddress = common.BytesToAddress(test.RandomBytes(20))

func TestStateDB_CreateAccount(t *testing.T) {
	tests := []struct {
		name                   string
		tree                   *state.State
		address                common.Address
		expectedAccountBalance *big.Int
	}{
		{

			// TODO override account
			name:                   "account exists",
			tree:                   initState(t),
			address:                initialAccountAddress,
			expectedAccountBalance: big.NewInt(200),
		},
		{
			name:                   "creates an account",
			tree:                   initState(t),
			address:                common.BytesToAddress(test.RandomBytes(20)),
			expectedAccountBalance: big.NewInt(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStateDB(tt.tree, logger.New(t))

			s.CreateAccount(tt.address)
			require.NoError(t, s.errDB)
			u, err := tt.tree.GetUnit(tt.address.Bytes(), false)
			require.NoError(t, err)
			require.NotNil(t, u)
			require.True(t, s.Exist(tt.address))
			require.Equal(t, tt.expectedAccountBalance, s.GetBalance(tt.address))
			require.Equal(t, ([]byte)(nil), s.GetCode(tt.address))
			require.Equal(t, uint64(0), s.GetNonce(tt.address))
			require.Equal(t, common.BytesToHash(emptyCodeHash), s.GetCodeHash(tt.address))
		})
	}
}

func TestStateDB_SubBalance(t *testing.T) {
	tests := []struct {
		name                   string
		tree                   *state.State
		address                common.Address
		subAmount              *big.Int
		expectedAccountBalance *big.Int
	}{
		{
			name:                   "account does not exist",
			tree:                   initState(t),
			address:                common.BytesToAddress(test.RandomBytes(20)),
			subAmount:              big.NewInt(10),
			expectedAccountBalance: big.NewInt(0),
		},
		{
			name:                   "ok",
			tree:                   initState(t),
			address:                initialAccountAddress,
			subAmount:              big.NewInt(10),
			expectedAccountBalance: big.NewInt(190),
		},
		{
			name:                   "subtract zero",
			tree:                   initState(t),
			address:                initialAccountAddress,
			subAmount:              big.NewInt(0),
			expectedAccountBalance: big.NewInt(200),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStateDB(tt.tree, logger.New(t))

			s.SubBalance(tt.address, tt.subAmount)
			require.NoError(t, s.errDB)
			require.Equal(t, tt.expectedAccountBalance, s.GetBalance(tt.address))
		})
	}
}

func TestStateDB_AddBalance(t *testing.T) {
	tests := []struct {
		name                   string
		tree                   *state.State
		address                common.Address
		addAmount              *big.Int
		expectedAccountBalance *big.Int
	}{
		{
			name:                   "account does not exist",
			tree:                   initState(t),
			address:                common.BytesToAddress(test.RandomBytes(20)),
			addAmount:              big.NewInt(10),
			expectedAccountBalance: big.NewInt(0),
		},
		{
			name:                   "ok",
			tree:                   initState(t),
			address:                initialAccountAddress,
			addAmount:              big.NewInt(10),
			expectedAccountBalance: big.NewInt(210),
		},
		{
			name:                   "add zero",
			tree:                   initState(t),
			address:                initialAccountAddress,
			addAmount:              big.NewInt(0),
			expectedAccountBalance: big.NewInt(200),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStateDB(tt.tree, logger.New(t))

			s.AddBalance(tt.address, tt.addAmount)
			require.NoError(t, s.errDB)
			require.Equal(t, tt.expectedAccountBalance, s.GetBalance(tt.address))
		})
	}
}

func TestStateDB_Nonce(t *testing.T) {
	tests := []struct {
		name          string
		tree          *state.State
		address       common.Address
		expectedNonce uint64
		op            func(s *StateDB)
	}{
		{
			name:          "account does not exist",
			tree:          initState(t),
			address:       common.BytesToAddress(test.RandomBytes(20)),
			expectedNonce: 0,
		},
		{
			name:          "empty account",
			tree:          initState(t),
			address:       initialAccountAddress,
			expectedNonce: 0,
		},
		{
			name: "set nonce",
			tree: initState(t),
			op: func(s *StateDB) {
				s.SetNonce(initialAccountAddress, 2)
			},
			address:       initialAccountAddress,
			expectedNonce: 2,
		},
		{
			name: "assign nonce to a nonexistent account",
			tree: initState(t),
			op: func(s *StateDB) {
				s.SetNonce(common.BytesToAddress(test.RandomBytes(20)), 20)
			},
			address:       initialAccountAddress,
			expectedNonce: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStateDB(tt.tree, logger.New(t))

			if tt.op != nil {
				tt.op(s)
			}
			require.NoError(t, s.errDB)
			require.Equal(t, tt.expectedNonce, s.GetNonce(tt.address))
			require.NoError(t, s.errDB)
		})
	}
}

func TestStateDB_Code(t *testing.T) {
	tests := []struct {
		name             string
		initialState     *state.State
		address          common.Address
		op               func(s *StateDB)
		expectedCode     []byte
		expectedCodeHash common.Hash
	}{
		{
			name:             "account does not exist",
			initialState:     state.NewEmptyState(),
			address:          common.BytesToAddress(test.RandomBytes(20)),
			expectedCode:     nil,
			expectedCodeHash: common.Hash{},
		},
		{
			name:             "empty account",
			initialState:     initState(t),
			address:          initialAccountAddress,
			expectedCode:     nil,
			expectedCodeHash: common.BytesToHash(emptyCodeHash),
		},
		{
			name:         "set code",
			initialState: initState(t),
			op: func(s *StateDB) {
				s.SetCode(initialAccountAddress, []byte("my code"))
			},
			address:          initialAccountAddress,
			expectedCode:     []byte("my code"),
			expectedCodeHash: crypto.Keccak256Hash([]byte("my code")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStateDB(tt.initialState, logger.New(t))
			if tt.op != nil {
				tt.op(s)
			}
			require.Equal(t, len(tt.expectedCode), s.GetCodeSize(tt.address))
			require.NoError(t, s.errDB)
			require.Equal(t, tt.expectedCode, s.GetCode(tt.address))
			require.NoError(t, s.errDB)
			require.Equal(t, tt.expectedCodeHash, s.GetCodeHash(tt.address))
			require.NoError(t, s.errDB)

		})
	}
}

func TestStateDB_ContractStorage(t *testing.T) {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(2))
	key2 := common.BigToHash(big.NewInt(3))
	value2 := common.BigToHash(big.NewInt(4))

	s := initState(t)
	db := NewStateDB(s, logger.New(t))
	db.SetState(initialAccountAddress, key1, value1)
	db.SetState(initialAccountAddress, key2, value2)

	require.Equal(t, value1, db.GetState(initialAccountAddress, key1))
	require.Equal(t, value2, db.GetState(initialAccountAddress, key2))

	require.Equal(t, common.Hash{}, db.GetCommittedState(initialAccountAddress, key1))
	require.Equal(t, common.Hash{}, db.GetCommittedState(initialAccountAddress, key2))

	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	require.Equal(t, value1, db.GetState(initialAccountAddress, key1))
	require.Equal(t, value2, db.GetState(initialAccountAddress, key2))

	require.Equal(t, value1, db.GetCommittedState(initialAccountAddress, key1))
	require.Equal(t, value2, db.GetCommittedState(initialAccountAddress, key2))

}

func TestStateDB_AddLog(t *testing.T) {
	s := initState(t)
	db := NewStateDB(s, logger.New(t))
	l := &types.Log{
		Address: common.BytesToAddress(test.RandomBytes(20)),
		Topics:  []common.Hash{common.BytesToHash(test.RandomBytes(32))},
		Data:    []byte{0, 0, 0, 1},
	}
	db.AddLog(l)
	db.AddLog(nil)
	require.Len(t, db.GetLogs(), 1)
	require.Len(t, db.logs, 1)
	entry := db.logs[0]
	require.Len(t, entry.Topics, 1)
	require.Equal(t, l.Data, entry.Data)
	require.Equal(t, l.Topics[0], entry.Topics[0])
	require.Equal(t, l.Address, entry.Address)

	require.NoError(t, db.Finalize())
	require.Len(t, db.GetLogs(), 0)
}

func TestStateDB_SelfDestruct(t *testing.T) {
	db := NewStateDB(initState(t), logger.New(t))

	db.SelfDestruct(common.BytesToAddress(test.RandomBytes(20)))
	require.False(t, db.HasSelfDestructed(initialAccountAddress))
	db.SelfDestruct(initialAccountAddress)
	require.True(t, db.HasSelfDestructed(initialAccountAddress))

	require.NoError(t, db.Finalize())

	_, _, err := db.tree.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, db.tree.Commit())

	u, err := db.tree.GetUnit(initialAccountAddress.Bytes(), false)
	require.ErrorContains(t, err, "not found")
	require.Nil(t, u)
}

func TestStateDB_TransientStorage(t *testing.T) {
	db := NewStateDB(initState(t), logger.New(t))
	addr := common.BytesToAddress(test.RandomBytes(20))
	var key, val common.Hash
	key.SetBytes([]byte{1, 2, 3})
	val.SetBytes([]byte{5, 6, 7})
	db.SetTransientState(addr, key, val)
	storedVal := db.GetTransientState(addr, key)
	require.EqualValues(t, val, storedVal)
}

func TestStateDB_RefundAndRevert(t *testing.T) {
	db := NewStateDB(initState(t), logger.New(t))
	db.AddRefund(10)
	snapID1 := db.Snapshot()
	db.SubRefund(5)
	db.AddRefund(10)
	require.EqualValues(t, 15, db.refund)
	db.RevertToSnapshot(snapID1)
	require.EqualValues(t, 10, db.refund)
}

func TestStateDB_SelfDestruct6780(t *testing.T) {
	db := NewStateDB(initState(t), logger.New(t))
	db.Selfdestruct6780(initialAccountAddress)
	require.True(t, db.Exist(initialAccountAddress))
	require.False(t, db.HasSelfDestructed(initialAccountAddress))
	// add a new account and call self-destruct immediately
	newAddr := common.BytesToAddress(test.RandomBytes(20))
	db.CreateAccount(newAddr)
	db.AddBalance(newAddr, big.NewInt(10000))
	db.Selfdestruct6780(newAddr)
	require.True(t, db.Exist(newAddr))
	require.True(t, db.HasSelfDestructed(newAddr))
	require.NoError(t, db.Finalize())
	require.False(t, db.Exist(newAddr))
	require.True(t, db.Exist(initialAccountAddress))
}

func TestStateDB_RevertSnapshot(t *testing.T) {
	s := NewStateDB(initState(t), logger.New(t))
	snapID := s.Snapshot()
	s.SetNonce(initialAccountAddress, 1)
	s.AddBalance(initialAccountAddress, big.NewInt(100))
	s.SetCode(initialAccountAddress, []byte("hello world"))
	s.SetState(initialAccountAddress, common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(1)))
	address := common.BytesToAddress(test.RandomBytes(20))
	s.CreateAccount(address)
	s.SetNonce(address, 1)
	require.NoError(t, s.DBError())

	s.RevertToSnapshot(snapID)
	require.False(t, s.Exist(address))

	require.Equal(t, big.NewInt(200), s.GetBalance(initialAccountAddress))
	require.Equal(t, uint64(0), s.GetNonce(initialAccountAddress))
	require.Equal(t, []byte(nil), s.GetCode(initialAccountAddress))
	require.Equal(t, common.Hash{}, s.GetState(initialAccountAddress, common.BigToHash(big.NewInt(1))))
}

func TestStateDB_RevertSnapshot2(t *testing.T) {
	s := NewStateDB(initState(t), logger.New(t))
	snapID := s.Snapshot()
	s.SetNonce(initialAccountAddress, 1)
	s.AddBalance(initialAccountAddress, big.NewInt(100))
	s.SetCode(initialAccountAddress, []byte("hello world"))
	s.SetState(initialAccountAddress, common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(1)))
	address := common.BytesToAddress(test.RandomBytes(20))
	s.CreateAccount(address)
	s.SetNonce(address, 1)
	require.NoError(t, s.DBError())
	// CREATE A SECOND SNAPSHOT
	s.Snapshot()
	// update nonce again
	s.SetNonce(initialAccountAddress, 3)
	s.Snapshot()
	// revert to initial state
	s.RevertToSnapshot(snapID)
	require.False(t, s.Exist(address))

	require.Equal(t, big.NewInt(200), s.GetBalance(initialAccountAddress))
	require.Equal(t, uint64(0), s.GetNonce(initialAccountAddress))
	require.Equal(t, []byte(nil), s.GetCode(initialAccountAddress))
	require.Equal(t, common.Hash{}, s.GetState(initialAccountAddress, common.BigToHash(big.NewInt(1))))
}

func TestStateDB_GetUpdatedUnits(t *testing.T) {
	s := state.NewEmptyState()
	db := NewStateDB(s, logger.New(t))
	require.NoError(t, db.Finalize())
	units := db.GetUpdatedUnits()
	require.Empty(t, units)
	db.CreateAccount(initialAccountAddress)
	snapID := db.Snapshot()
	db.SetNonce(initialAccountAddress, 1)
	db.AddBalance(initialAccountAddress, big.NewInt(100))
	db.SetCode(initialAccountAddress, []byte("hello world"))
	db.SetState(initialAccountAddress, common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(1)))
	address := common.BytesToAddress(test.RandomBytes(20))
	db.CreateAccount(address)
	db.SetNonce(address, 1)
	require.NoError(t, db.DBError())
	units = db.GetUpdatedUnits()
	require.Len(t, units, 2)
	db.RevertToSnapshot(snapID)
	require.True(t, db.Exist(initialAccountAddress))
	require.False(t, db.Exist(address))
	units = db.GetUpdatedUnits()
	require.Len(t, units, 1)
	require.NoError(t, db.Finalize())
	units = db.GetUpdatedUnits()
	require.Empty(t, units)
}

func TestStateDB_RollbackMultipleSnapshots(t *testing.T) {
	s := state.NewEmptyState()
	db := NewStateDB(s, logger.New(t))
	// snapshot 1, empty DB, no units
	snapID1 := db.Snapshot()
	require.EqualValues(t, 1, snapID1)
	units := db.GetUpdatedUnits()
	require.Len(t, units, 0)
	db.CreateAccount(initialAccountAddress)
	db.SetNonce(initialAccountAddress, 1)
	db.AddBalance(initialAccountAddress, big.NewInt(100))
	db.SetCode(initialAccountAddress, []byte("hello world"))
	db.SetState(initialAccountAddress, common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(1)))
	// snapID1 snapshot has 1 account created
	snapID2 := db.Snapshot()
	require.EqualValues(t, 2, snapID2)
	units = db.GetUpdatedUnits()
	require.Len(t, units, 1)
	address := common.BytesToAddress(test.RandomBytes(20))
	db.CreateAccount(address)
	db.SetNonce(address, 1)
	address = common.BytesToAddress(test.RandomBytes(20))
	db.CreateAccount(address)
	db.SetNonce(address, 2)
	// snapID3 snapshot adds 2 new units
	snapID3 := db.Snapshot()
	require.EqualValues(t, 3, snapID3)
	units = db.GetUpdatedUnits()
	require.Len(t, units, 3)
	address = common.BytesToAddress(test.RandomBytes(20))
	db.CreateAccount(address)
	db.SetNonce(address, 1)
	address = common.BytesToAddress(test.RandomBytes(20))
	db.CreateAccount(address)
	db.SetNonce(address, 1)
	db.SetNonce(initialAccountAddress, 3)
	// snapID4 snapshot, adds 2 new units and modifies an existing unit
	snapID4 := db.Snapshot()
	require.EqualValues(t, 4, snapID4)
	require.NoError(t, db.DBError())
	units = db.GetUpdatedUnits()
	require.Len(t, units, 5)
	db.RevertToSnapshot(snapID2)
	require.True(t, db.Exist(initialAccountAddress))
	units = db.GetUpdatedUnits()
	require.Len(t, units, 1)
	require.EqualValues(t, 1, db.GetNonce(initialAccountAddress))
}

func initState(t *testing.T) *state.State {
	s := state.NewEmptyState()
	db := NewStateDB(s, logger.New(t))
	db.CreateAccount(initialAccountAddress)
	db.AddBalance(initialAccountAddress, big.NewInt(200))
	require.NoError(t, s.PruneLog(initialAccountAddress.Bytes()))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, db.Finalize())
	return s
}
