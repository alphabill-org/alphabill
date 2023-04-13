package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	tw "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestListTokensCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		accountNumber uint64
		expectedKind  twb.Kind
		expectedPass  string
	}{
		{
			name:          "list all tokens",
			args:          []string{},
			accountNumber: 0, // all tokens
			expectedKind:  twb.Any,
		},
		{
			name:          "list all tokens, encrypted wallet",
			args:          []string{"--pn", "some pass phrase"},
			accountNumber: 0, // all tokens
			expectedKind:  twb.Any,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list account tokens",
			args:          []string{"--key", "3"},
			accountNumber: 3,
			expectedKind:  twb.Any,
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible"},
			accountNumber: 0,
			expectedKind:  twb.Fungible,
		},
		{
			name:          "list account fungible tokens",
			args:          []string{"fungible", "--key", "4"},
			accountNumber: 4,
			expectedKind:  twb.Fungible,
		},
		{
			name:          "list account fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--key", "4", "--pn", "some pass phrase"},
			accountNumber: 4,
			expectedKind:  twb.Fungible,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible"},
			accountNumber: 0,
			expectedKind:  twb.NonFungible,
		},
		{
			name:          "list account non-fungible tokens",
			args:          []string{"non-fungible", "--key", "5"},
			accountNumber: 5,
			expectedKind:  twb.NonFungible,
		},
		{
			name:          "list account non-fungible tokens, encrypted walled",
			args:          []string{"non-fungible", "--key", "5", "--pn", "some pass phrase"},
			accountNumber: 5,
			expectedKind:  twb.NonFungible,
			expectedPass:  "some pass phrase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdList(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind twb.Kind, accountNumber *uint64) error {
				require.Equal(t, tt.accountNumber, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) > 0 {
					passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
					require.NoError(t, err)
					require.Equal(t, tt.expectedPass, passwordFromArg)
				}
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}

func TestListTokensTypesCommandInputs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedKind twb.Kind
		expectedPass string
	}{
		{
			name:         "list all tokens",
			args:         []string{},
			expectedKind: twb.Any,
		},
		{
			name:         "list all tokens, encrypted wallet",
			args:         []string{"--pn", "test pass phrase"},
			expectedKind: twb.Any,
			expectedPass: "test pass phrase",
		},
		{
			name:         "list all fungible tokens",
			args:         []string{"fungible"},
			expectedKind: twb.Fungible,
		},
		{
			name:         "list all fungible tokens, encrypted wallet",
			args:         []string{"fungible", "--pn", "test pass phrase"},
			expectedKind: twb.Fungible,
			expectedPass: "test pass phrase",
		},
		{
			name:         "list all non-fungible tokens",
			args:         []string{"non-fungible"},
			expectedKind: twb.NonFungible,
		},
		{
			name:         "list all non-fungible tokens, encrypted wallet",
			args:         []string{"non-fungible", "--pn", "test pass phrase"},
			expectedKind: twb.NonFungible,
			expectedPass: "test pass phrase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdListTypes(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind twb.Kind) error {
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) != 0 {
					passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
					require.NoError(t, err)
					require.Equal(t, tt.expectedPass, passwordFromArg)
				}
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}

func TestWalletCreateFungibleTokenTypeCmd_SymbolFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// missing symbol parameter
	_, err := execCommand(homedir, "token new-type fungible --decimals 3")
	require.ErrorContains(t, err, "required flag(s) \"symbol\" not set")
	// symbol parameter not set
	_, err = execCommand(homedir, "token new-type fungible --symbol")
	require.EqualError(t, err, `flag needs an argument: --symbol`)
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
}

func TestWalletCreateNonFungibleTokenCmd_TypeFlag(t *testing.T) {
	type args struct {
		cmdParams string
	}
	tests := []struct {
		name       string
		args       args
		want       []byte
		wantErrStr string
	}{
		{
			name:       "missing token type parameter",
			args:       args{cmdParams: "token new non-fungible --data 12AB"},
			wantErrStr: "required flag(s) \"type\" not set",
		},
		{
			name:       "missing token type parameter has no value",
			args:       args{cmdParams: "token new non-fungible --type"},
			wantErrStr: "flag needs an argument: --type",
		},
		{
			name:       "type parameter is not hex encoded",
			args:       args{cmdParams: "token new non-fungible --type 11dummy"},
			wantErrStr: "invalid argument \"11dummy\" for \"--type\" flag",
		},
		{
			name:       "type parameter is odd length",
			args:       args{cmdParams: "token new non-fungible --type A8B08"},
			wantErrStr: "invalid argument \"A8B08\" for \"--type\" flag: encoding/hex: odd length hex string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(homedir, tt.args.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletCreateNonFungibleTokenCmd_TokenIdFlag(t *testing.T) {
	//token-identifier parameter is odd length
	homedir := createNewTestWallet(t)
	_, err := execCommand(homedir, "token new non-fungible --type A8B0 --token-identifier A8B09")
	require.ErrorContains(t, err, "invalid argument \"A8B09\" for \"--token-identifier\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(homedir, "token new non-fungible --type A8B0 --token-identifier nothex")
	require.ErrorContains(t, err, "invalid argument \"nothex\" for \"--token-identifier\" flag: encoding/hex: invalid byte")
}

func TestWalletCreateNonFungibleTokenCmd_DataFileFlag(t *testing.T) {
	data := make([]byte, maxBinaryFile64Kb+1)
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdParams  string
		want       []byte
		wantErrStr string
	}{
		{
			name:       "both data and data-file specified",
			cmdParams:  "token new non-fungible --type 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "token new non-fungible --type 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "token new non-fungible --type 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64Kb limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(homedir, tt.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletUpdateNonFungibleTokenDataCmd_Flags(t *testing.T) {
	data := make([]byte, maxBinaryFile64Kb+1)
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdParams  string
		want       []byte
		wantErrStr string
	}{
		{
			name:       "both data and data-file specified",
			cmdParams:  "token update --token-identifier 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "token update --token-identifier 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "token update --token-identifier 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64Kb limit",
		},
		{
			name:       "update nft: both data flags missing",
			cmdParams:  "token update --token-identifier 12AB",
			wantErrStr: "either of ['--data', '--data-file'] flags must be specified",
		},
		{
			name:       "update nft: token id missing",
			cmdParams:  "token update",
			wantErrStr: "required flag(s) \"token-identifier\" not set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(homedir, tt.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// tokenID == nil means first token will be considered as success
func ensureTokenIndexed(t *testing.T, ctx context.Context, api *client.TokenBackend, ownerPubKey []byte, tokenID twb.TokenID) *twb.TokenUnit {
	var res *twb.TokenUnit
	require.Eventually(t, func() bool {
		offsetKey := ""
		var tokens []twb.TokenUnit
		var err error
		for {
			tokens, offsetKey, err = api.GetTokens(ctx, twb.Any, ownerPubKey, offsetKey, 0)
			require.NoError(t, err)
			for _, token := range tokens {
				if tokenID == nil {
					res = &token
					return true
				}
				if bytes.Equal(token.ID, tokenID) {
					res = &token
					return true
				}
			}
			if offsetKey == "" {
				break
			}
		}
		return false
	}, 2*test.WaitDuration, test.WaitTick)
	return res
}

func ensureTokenTypeIndexed(t *testing.T, ctx context.Context, api *client.TokenBackend, creatorPubKey []byte, typeID twb.TokenTypeID) *twb.TokenUnitType {
	var res *twb.TokenUnitType
	require.Eventually(t, func() bool {
		offsetKey := ""
		var types []twb.TokenUnitType
		var err error
		for {
			types, offsetKey, err = api.GetTokenTypes(ctx, twb.Any, creatorPubKey, offsetKey, 0)
			require.NoError(t, err)
			for _, t := range types {
				if bytes.Equal(t.ID, typeID) {
					res = &t
					return true
				}
			}
			if offsetKey == "" {
				break
			}
		}
		return false
	}, 2*test.WaitDuration, test.WaitTick)
	return res
}

func startTokensPartition(t *testing.T) (*testpartition.AlphabillPartition, string) {
	tokensState := rma.NewWithSHA256()
	require.NotNil(t, tokensState)
	network, err := testpartition.NewNetwork(1,
		func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
			system, err := tokens.New(
				tokens.WithState(tokensState),
				tokens.WithTrustBase(tb),
				tokens.WithFeeCalculator(fc.FixedFee(0)), // 0 to disable fee module
			)
			require.NoError(t, err)
			return system
		}, tokens.DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = network.Close()
	})

	listenAddr := fmt.Sprintf(":%d", net.GetFreeRandomPort(t))
	startRPCServer(t, network, listenAddr)
	dialAddr := "localhost" + listenAddr
	return network, dialAddr
}

func startTokensBackend(t *testing.T, nodeAddr string) (srvUri string, restApi *client.TokenBackend, ctx context.Context) {
	port, err := net.GetFreePort()
	require.NoError(t, err)
	host := fmt.Sprintf("localhost:%v", port)
	srvUri = "http://" + host
	cfg := twb.NewConfig(host, nodeAddr, filepath.Join(t.TempDir(), "backend.db"), wlog.GetLogger())
	addr, err := url.Parse(srvUri)
	require.NoError(t, err)
	restApi = client.New(*addr)

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())

	t.Cleanup(cancel)

	go func() {
		err = twb.Run(ctx, cfg)
		fmt.Println("token wallet ended")
	}()

	require.Eventually(t, func() bool {
		rn, _ := restApi.GetRoundNumber(ctx)
		return rn > 0
	}, test.WaitDuration, test.WaitTick)

	return
}

func createNewTokenWallet(t *testing.T, addr string) (*tw.Wallet, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))

	w, err := tw.New(tokens.DefaultTokenTxSystemIdentifier, addr, am, false)
	require.NoError(t, err)
	require.NotNil(t, w)

	return w, homeDir
}

func execTokensCmdWithError(t *testing.T, homedir string, command string, expectedError string) {
	_, err := doExecTokensCmd(homedir, command)
	require.ErrorContains(t, err, expectedError)
}

func execTokensCmd(t *testing.T, homedir string, command string) *testConsoleWriter {
	outputWriter, err := doExecTokensCmd(homedir, command)
	require.NoError(t, err)

	return outputWriter
}

func doExecTokensCmd(homedir string, command string) (*testConsoleWriter, error) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New()
	args := "wallet token --log-level DEBUG --home " + homedir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.addAndExecuteCommand(context.Background())
}

func randomID(t *testing.T) []byte {
	id, err := tw.RandomID()
	require.NoError(t, err)
	return id
}

func verifyStdoutEventually(t *testing.T, exec func() *testConsoleWriter, expectedLines ...string) {
	require.Eventually(t, func() bool {
		joined := strings.Join(exec().lines, "\n")
		res := true
		for _, expectedLine := range expectedLines {
			res = res && strings.Contains(joined, expectedLine)
		}
		return res
	}, test.WaitDuration, test.WaitTick)
}
