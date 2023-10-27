package tokens

import (
	"testing"

	"github.com/alphabill-org/alphabill/api/predicates"
	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/account"
	"github.com/stretchr/testify/require"
)

func TestParsePredicateArgument(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		input string
		// expectations:
		result    predicates.PredicateBytes
		accNumber uint64
		err       string
	}{
		{
			input:  "",
			result: nil,
		},
		{
			input:  "empty",
			result: nil,
		},
		{
			input:  "true",
			result: nil,
		},
		{
			input:  "false",
			result: nil,
		},
		{
			input:  "0x",
			result: nil,
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
			input:     "ptpkh",
			accNumber: uint64(1),
		},
		{
			input:     "ptpkh:1",
			accNumber: uint64(1),
		},
		{
			input:     "ptpkh:10",
			accNumber: uint64(10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			argument, err := parsePredicate(tt.input, tt.accNumber, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				if tt.accNumber > 0 {
					require.Equal(t, tt.accNumber, argument.AccountNumber)
				} else {
					require.EqualValues(t, tt.result, argument.Argument)
				}
			}
		})
	}
}

func TestParsePredicateClause(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		// inputs:
		clause    string
		accNumber uint64
		// expectations:
		predicate     []byte
		expectedIndex uint64
		err           string
	}{
		{
			clause:    "",
			predicate: templates.AlwaysTrueBytes(),
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
			predicate: templates.AlwaysTrueBytes(),
		},
		{
			clause:    "false",
			predicate: templates.AlwaysFalseBytes(),
		},
		{
			clause: "ptpkh:",
			err:    "invalid predicate clause",
		},
		{
			clause:        "ptpkh",
			expectedIndex: uint64(0),
			err:           "invalid key number: 0 in 'ptpkh'",
		},
		{
			clause:        "ptpkh",
			accNumber:     2,
			expectedIndex: uint64(1),
			predicate:     templates.NewP2pkh256BytesFromKeyHash(mock.keyHash),
		},
		{
			clause: "ptpkh:0",
			err:    "invalid key number: 0",
		},
		{
			clause:        "ptpkh:2",
			expectedIndex: uint64(1),
			predicate:     templates.NewP2pkh256BytesFromKeyHash(mock.keyHash),
		},
		{
			clause:    "ptpkh:0x0102",
			predicate: templates.NewP2pkh256BytesFromKeyHash(mock.keyHash),
		},
		{
			clause: "ptpkh:0X",
			err:    "invalid predicate clause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.clause, func(t *testing.T) {
			mock.recordedIndex = 0
			predicate, err := ParsePredicateClause(tt.clause, tt.accNumber, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.predicate, predicate)
			require.Equal(t, tt.expectedIndex, mock.recordedIndex)
		})
	}
}

func TestDecodeHexOrEmpty(t *testing.T) {
	tests := []struct {
		input  string
		result []byte
		err    string
	}{
		{
			input:  "",
			result: nil,
		},
		{
			input:  "empty",
			result: nil,
		},
		{
			input:  "0x",
			result: nil,
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
			res, err := DecodeHexOrEmpty(tt.input)
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
