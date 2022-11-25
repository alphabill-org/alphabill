package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalletCreateFungibleTokenTypeCmd_SymbolFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// missing symbol parameter
	_, err := execCommand(homedir, "token new-type fungible --decimals 3")
	require.ErrorContains(t, err, "required flag(s) \"symbol\" not set")
	// symbol parameter not set
	_, err = execCommand(homedir, "flag needs an argument: --symbol")
	// there currently are no restrictions on symbol length on CLI side
}

func TestWalletCreateFungibleTokenTypeCmd_TypeIdlFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// hidden parameter type (not a mandatory parameter)
	_, err := execCommand(homedir, "token new-type fungible --symbol \"@1\" --type")
	require.ErrorContains(t, err, "flag needs an argument: --type")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --type 011")
	require.ErrorContains(t, err, "invalid argument \"011\" for \"--type\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --type foo")
	require.ErrorContains(t, err, "invalid argument \"foo\" for \"--type\" flag")
	// there currently are no restrictions on type length on CLI side
}

func TestWalletCreateFungibleTokenTypeCmd_DecimalsFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// hidden parameter type (not a mandatory parameter)
	_, err := execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals")
	require.ErrorContains(t, err, "flag needs an argument: --decimals")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals foo")
	require.ErrorContains(t, err, "invalid argument \"foo\" for \"--decimals\" flag")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals -1")
	require.ErrorContains(t, err, "invalid argument \"-1\" for \"--decimals\"")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals 9")
	require.ErrorContains(t, err, "argument \"9\" for \"--decimals\" flag is out of range, max value 8")
}

func TestWalletCreateFungibleTokenTypeCmd_ParentTypeFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// hidden parameter type (not a mandatory parameter)
	_, err := execCommand(homedir, "token new-type fungible --symbol \"@1\" --parent-type 01")
	require.ErrorContains(t, err, "missing mandatory flag \"--creation-input\"")
}

func TestWalletCreateFungibleTokenCmd_TypeFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	_, err := execCommand(homedir, "token new fungible --type A8B")
	require.ErrorContains(t, err, "invalid argument \"A8B\" for \"--type\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(homedir, "token new fungible --type nothex")
	require.ErrorContains(t, err, "invalid argument \"nothex\" for \"--type\" flag: encoding/hex: invalid byte")
	_, err = execCommand(homedir, "token new fungible --amount 4")
	require.ErrorContains(t, err, "required flag(s) \"type\" not set")
}

func TestWalletCreateFungibleTokenCmd_AmountFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	_, err := execCommand(homedir, "token new fungible --type A8BB")
	require.ErrorContains(t, err, "required flag(s) \"amount\" not set")
	_, err = execCommand(homedir, "token new fungible --type A8BB --amount abba")
	require.ErrorContains(t, err, "invalid argument \"abba\" for \"--amount\" flag")
	_, err = execCommand(homedir, "token new fungible --type A8BB --amount -2")
	require.ErrorContains(t, err, "invalid argument \"-2\" for \"--amount\" flag")
	//	_, err = execCommand(homedir, "token new fungible --type A8BB --amount 0x30")
	//	require.ErrorContains(t, err, "invalid argument \"0x30\" for \"--amount\" flag")
	_, err = execCommand(homedir, "token new fungible --type A8BB --amount 18446744073709551616")
	require.ErrorContains(t, err, "invalid argument \"18446744073709551616\" for \"--amount\" flag")
}
