package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"

	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/stretchr/testify/require"
)

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
	tmpfile, err := ioutil.TempFile("", "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	// remember to clean up the file
	defer os.Remove(tmpfile.Name())

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
			name:       "both data and data-file specified",
			args:       args{cmdParams: "token new non-fungible --type 12AB --data 1122aabb --data-file=/tmp/test/foo.bin"},
			wantErrStr: "flags \"--data\" and \"--data-file\" are mutually exclusive",
		},
		{
			name:       "data-file not found",
			args:       args{cmdParams: "token new non-fungible --type 12AB --data-file=/tmp/test/foo.bin"},
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			args:       args{cmdParams: "token new non-fungible --type 12AB --data-file=" + tmpfile.Name()},
			wantErrStr: "data-file read error: file size over 64Kb limit",
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

func TestWalletCreateNonFungibleTokenCmd_DataFileFlagIntegrationTest(t *testing.T) {
	partition, unitState := startTokensPartition(t)
	startRPCServer(t, partition, listenAddr)
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1 := createNewTokenWallet(t, "w1", dialAddr)
	require.NotNil(t, w1)
	w1.Shutdown()
	typeId := util.Uint256ToBytes(uint256.NewInt(uint64(0x10)))
	symbol := "ABNFT"
	// create type
	execTokensCmd(t, "w1", fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X", symbol, dialAddr, typeId))
	ensureUnitBytes(t, unitState, typeId)
	// create non-fungible token from using data-file
	nftID := util.Uint256ToBytes(uint256.NewInt(uint64(0x11)))
	data := make([]byte, 1024)
	rand.Read(data)
	tmpfile, err := ioutil.TempFile("", "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	// remember to clean up the file
	defer os.Remove(tmpfile.Name())
	execTokensCmd(t, "w1", fmt.Sprintf("new non-fungible --sync true -u %s --type %X --token-identifier %X --data-file %s", dialAddr, typeId, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID) {
			mintNonFungibleAttr := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(mintNonFungibleAttr))
			require.Equal(t, data, mintNonFungibleAttr.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list non-fungible -u %s", dialAddr)), fmt.Sprintf("ID='%X'", nftID))
}
