package tokens

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
)

func TestParsePredicateArgument(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		input string
		// expectations:
		result tokens.Predicate
		accKey uint64
		err    string
	}{
		{
			input:  "",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "empty",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "true",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "false",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "0x",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "0x5301",
			result: []byte{0x53, 0x01},
		},
		{
			input: "ptpkh:0",
			err:   "invalid key number: 0",
		},
		{
			input:  "ptpkh",
			accKey: uint64(1),
		},
		{
			input:  "ptpkh:1",
			accKey: uint64(1),
		},
		{
			input:  "ptpkh:10",
			accKey: uint64(10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			argument, err := parsePredicate(tt.input, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				if tt.accKey > 0 {
					require.Equal(t, tt.accKey, argument.AccountNumber)
				} else {
					require.Equal(t, tt.result, argument.Argument)
				}
			}
		})
	}
}

func TestParsePredicateClause(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		clause    string
		predicate []byte
		index     uint64
		err       string
	}{
		{
			clause:    "",
			predicate: script.PredicateAlwaysTrue(),
		}, {
			clause: "foo",
			err:    "invalid predicate clause",
		},
		{
			clause:    "0x53510087",
			predicate: []byte{0x53, 0x51, 0x00, 0x87},
		},
		{
			clause:    "true",
			predicate: script.PredicateAlwaysTrue(),
		},
		{
			clause:    "false",
			predicate: script.PredicateAlwaysFalse(),
		},
		{
			clause: "ptpkh:",
			err:    "invalid predicate clause",
		},
		{
			clause:    "ptpkh",
			index:     uint64(0),
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause: "ptpkh:0",
			err:    "invalid key number: 0",
		},
		{
			clause:    "ptpkh:2",
			index:     uint64(1),
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause:    "ptpkh:0x0102",
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause: "ptpkh:0X",
			err:    "invalid predicate clause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.clause, func(t *testing.T) {
			mock.recordedIndex = 0
			predicate, err := ParsePredicateClause(tt.clause, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.predicate, predicate)
			require.Equal(t, tt.index, mock.recordedIndex)
		})
	}
}

func TestDecodeHexOrEmpty(t *testing.T) {
	empty := []byte{}
	tests := []struct {
		input  string
		result []byte
		err    string
	}{
		{
			input:  "",
			result: empty,
		},
		{
			input:  "empty",
			result: empty,
		},
		{
			input:  "0x",
			result: empty,
		},
		{
			input: "0x534",
			err:   "odd length hex string",
		},
		{
			input: "0x53q",
			err:   "invalid byte",
		},
		{
			input:  "53",
			result: []byte{0x53},
		},
		{
			input:  "0x5354",
			result: []byte{0x53, 0x54},
		},
		{
			input:  "5354",
			result: []byte{0x53, 0x54},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := decodeHexOrEmpty(tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.result, res)
			}
		})
	}
}

type accountManagerMock struct {
	keyHash       []byte
	recordedIndex uint64
}

func (a *accountManagerMock) GetAccountKey(accountIndex uint64) (*account.AccountKey, error) {
	a.recordedIndex = accountIndex
	return &account.AccountKey{PubKeyHash: &account.KeyHashes{Sha256: a.keyHash}}, nil
}

func (a *accountManagerMock) GetAll() []account.Account {
	return nil
}

func (a *accountManagerMock) CreateKeys(mnemonic string) error {
	return nil
}

func (a *accountManagerMock) AddAccount() (uint64, []byte, error) {
	return 0, nil, nil
}

func (a *accountManagerMock) GetMnemonic() (string, error) {
	return "", nil
}

func (a *accountManagerMock) GetAccountKeys() ([]*account.AccountKey, error) {
	return nil, nil
}

func (a *accountManagerMock) GetMaxAccountIndex() (uint64, error) {
	return 0, nil
}

func (a *accountManagerMock) GetPublicKey(accountIndex uint64) ([]byte, error) {
	return nil, nil
}

func (a *accountManagerMock) GetPublicKeys() ([][]byte, error) {
	return nil, nil
}

func (a *accountManagerMock) IsEncrypted() (bool, error) {
	return false, nil
}

func (a *accountManagerMock) Close() {
}
