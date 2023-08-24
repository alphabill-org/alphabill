package statedb

import (
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
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
			s := &StateDB{
				tree:       tt.tree,
				accessList: newAccessList(),
			}
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
			s := &StateDB{
				tree:       tt.tree,
				accessList: newAccessList(),
			}
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
			s := &StateDB{
				tree:       tt.tree,
				accessList: newAccessList(),
			}
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
			s := &StateDB{
				tree:       tt.tree,
				accessList: newAccessList(),
			}
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
			s := &StateDB{
				tree:       tt.initialState,
				accessList: newAccessList(),
			}
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
	db := &StateDB{
		tree:       s,
		accessList: newAccessList(),
	}
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

func TestStateDB_Suicide(t *testing.T) {
	s := initState(t)
	db := &StateDB{
		tree:       s,
		accessList: newAccessList(),
	}

	require.False(t, db.Suicide(common.BytesToAddress(test.RandomBytes(20))))
	require.False(t, db.HasSuicided(initialAccountAddress))
	require.True(t, db.Suicide(initialAccountAddress))

	require.NoError(t, db.Finalize())

	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())

	u, err := s.GetUnit(initialAccountAddress.Bytes(), false)
	require.ErrorContains(t, err, "not found")
	require.Nil(t, u)
}

func TestStateDB_RevertSnapshot(t *testing.T) {
	s := &StateDB{
		tree:       initState(t),
		accessList: newAccessList(),
	}
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

func initState(t *testing.T) *state.State {
	s := state.NewEmptyState()
	db := NewStateDB(s)
	db.CreateAccount(initialAccountAddress)
	db.AddBalance(initialAccountAddress, big.NewInt(200))
	require.NoError(t, s.PruneLog(initialAccountAddress.Bytes()))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return s
}
