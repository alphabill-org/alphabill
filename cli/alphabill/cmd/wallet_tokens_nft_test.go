package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/partition/event"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
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
	tmpfile, err := ioutil.TempFile(t.TempDir(), "test")
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
	tmpfile, err := ioutil.TempFile(t.TempDir(), "test")
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
			wantErrStr: "Either of ['--data', '--data-file'] flags must be specified",
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

func TestNFTs_Integration(t *testing.T) {
	partition, unitState := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr)
	_, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, dialAddr)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	// non-fungible token types
	typeID := randomID(t)
	typeID2 := randomID(t)
	nftID := randomID(t)
	symbol := "ABNFT"
	execTokensCmdWithError(t, homedirW1, "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X --subtype-clause ptpkh", symbol, dialAddr, typeID))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X --parent-type %X --subtype-input ptpkh", symbol+"2", dialAddr, typeID2, typeID))
	ensureUnitBytes(t, unitState, typeID)
	// mint NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible --sync true -u %s --type %X --token-identifier %X", dialAddr, typeID, nftID))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	// transfer NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible --sync false -u %s --token-identifier %X --address 0x%X -k 1", dialAddr, nftID, w2key.PubKey))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.TransferNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -u %s", dialAddr)), fmt.Sprintf("ID='%X'", nftID))
	//check what is left in w1, nothing, that is
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -u %s", dialAddr)), "No tokens")
	// list token types
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types")), "symbol=ABNFT (type,non-fungible)")
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types non-fungible")), "symbol=ABNFT (type,non-fungible)")
}

func TestNFTDataUpdateCmd_Integration(t *testing.T) {
	partition, unitState := startTokensPartition(t)
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedir := createNewTokenWallet(t, dialAddr)
	require.NotNil(t, w1)
	w1.Shutdown()
	typeId := randomID(t)
	symbol := "ABNFT"
	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X", symbol, dialAddr, typeId))
	ensureUnitBytes(t, unitState, typeId)
	// create non-fungible token from using data-file
	nftID := randomID(t)
	data := make([]byte, 1024)
	rand.Read(data)
	tmpfile, err := ioutil.TempFile(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible --sync true -u %s --type %X --token-identifier %X --data-file %s", dialAddr, typeId, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID) {
			mintNonFungibleAttr := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(mintNonFungibleAttr))
			require.Equal(t, data, mintNonFungibleAttr.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list non-fungible -u %s", dialAddr)), fmt.Sprintf("ID='%X'", nftID))
	require.Equal(t, data, reflect.ValueOf(ensureUnitBytes(t, unitState, nftID).Data).Elem().FieldByName("data").Bytes())
	// generate new data
	data2 := make([]byte, 1024)
	rand.Read(data2)
	require.NotEqual(t, data, data2)
	tmpfile, err = ioutil.TempFile(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data2)
	require.NoError(t, err)
	// update data
	execTokensCmd(t, homedir, fmt.Sprintf("update -u %s --token-identifier %X --data-file %s --data-update-input true,true", dialAddr, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.UpdateNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID) {
			dataUpdateAttrs := &tokens.UpdateNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(dataUpdateAttrs))
			require.Equal(t, data2, dataUpdateAttrs.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	require.Equal(t, data2, reflect.ValueOf(ensureUnitBytes(t, unitState, nftID).Data).Elem().FieldByName("data").Bytes())

	// create non-updatable nft
	nftID2 := randomID(t)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible --sync true -u %s --type %X --token-identifier %X --data 01 --data-update-clause false", dialAddr, typeId, nftID2))
	require.Equal(t, []byte{0x01}, reflect.ValueOf(ensureUnitBytes(t, unitState, nftID2).Data).Elem().FieldByName("data").Bytes())
	//try to update and observe failure
	execTokensCmd(t, homedir, fmt.Sprintf("update -u %s --sync false --token-identifier %X --data 02 --data-update-input false,true", dialAddr, nftID2))
	testevent.ContainsEvent(t, partition.EventHandler, event.TransactionFailed)
}

func TestNFT_InvariantPredicate_Integration(t *testing.T) {
	partition, unitState := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr)
	_, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, dialAddr)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	symbol1 := "ABNFT"
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -u %s --sync true --symbol %s --type %X --inherit-bearer-clause %s", dialAddr, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID11)
	//second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -u %s --sync true --symbol %s --type %X --parent-type %X --subtype-input %s", dialAddr, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID12)
	//mint
	id := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -u %s --sync true --type %X --token-identifier %X --mint-input %s,%s", dialAddr, typeID12, id, predicatePtpkh, predicatePtpkh))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -u %s", dialAddr)), "Symbol='ABNFT'")
	//send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible -u %s --token-identifier %X --address 0x%X -k 1 --inherit-bearer-input %s,%s", dialAddr, id, w2key.PubKey, predicateTrue, predicatePtpkh))
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -u %s", dialAddr)), "Symbol='ABNFT'")
}
