package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
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

func TestNFTs_Integration(t *testing.T) {
	partition, nodeDialUrl := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, client, ctx := startTokensBackend(t, nodeDialUrl)

	w1, homedirW1 := createNewTokenWallet(t, backendUrl)
	_, w1key2, err := w1.GetAccountManager().AddAccount()
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, backendUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	// non-fungible token types
	typeID := randomID(t)
	typeID2 := randomID(t)
	nftID := randomID(t)
	symbol := "ABNFT"
	execTokensCmdWithError(t, homedirW1, "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -k 2 --symbol %s -r %s --type %X --subtype-clause ptpkh", symbol, backendUrl, typeID))
	ensureTokenTypeIndexed(t, ctx, client, w1key2, typeID)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -k 2 --symbol %s -r %s --type %X --parent-type %X --subtype-input ptpkh", symbol+"2", backendUrl, typeID2, typeID))
	ensureTokenTypeIndexed(t, ctx, client, w1key2, typeID2)
	// mint NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -k 2 -r %s --type %X --token-identifier %X", backendUrl, typeID, nftID))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenIndexed(t, ctx, client, w1key2, nftID)
	// transfer NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible -k 2 -r %s --token-identifier %X --address 0x%X", backendUrl, nftID, w2key.PubKey))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.TransferNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenIndexed(t, ctx, client, w2key.PubKey, nftID)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendUrl)), fmt.Sprintf("ID='%X'", nftID))
	//check what is left in w1, nothing, that is
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -k 2 -r %s", backendUrl)), "No tokens")
	// list token types
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types -r %s", backendUrl)), "symbol=ABNFT (nft)")
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types non-fungible -r %s", backendUrl)), "symbol=ABNFT (nft)")
	// transfer back
	execTokensCmd(t, homedirW2, fmt.Sprintf("send non-fungible -r %s --token-identifier %X --address 0x%X -k 1", backendUrl, nftID, w1key2))
	ensureTokenIndexed(t, ctx, client, w1key2, nftID)
	// mint nft from w1 and set the owner to w2
	nftID2 := randomID(t)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendUrl)), "No tokens")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -k 2 -r %s --type %X --bearer-clause ptpkh:0x%X --token-identifier %X", backendUrl, typeID, w2key.PubKeyHash.Sha256, nftID2))
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendUrl)), fmt.Sprintf("ID='%X'", nftID2))
}

func TestNFTDataUpdateCmd_Integration(t *testing.T) {
	partition, nodeDialUrl := startTokensPartition(t)
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, client, ctx := startTokensBackend(t, nodeDialUrl)

	w1, homedir := createNewTokenWallet(t, backendUrl)
	require.NotNil(t, w1)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()
	typeID := randomID(t)
	symbol := "ABNFT"
	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type non-fungible --symbol %s -r %s --type %X", symbol, backendUrl, typeID))
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID)
	// create non-fungible token from using data-file
	nftID := randomID(t)
	data := make([]byte, 1024)
	rand.Read(data)
	tmpfile, err := ioutil.TempFile(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible -r %s --type %X --token-identifier %X --data-file %s", backendUrl, typeID, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID) {
			mintNonFungibleAttr := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(mintNonFungibleAttr))
			require.Equal(t, data, mintNonFungibleAttr.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	nft := ensureTokenIndexed(t, ctx, client, w1key.PubKey, nftID)
	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list non-fungible -r %s", backendUrl)), fmt.Sprintf("ID='%X'", nftID))
	require.Equal(t, data, nft.NftData)
	// generate new data
	data2 := make([]byte, 1024)
	rand.Read(data2)
	require.NotEqual(t, data, data2)
	tmpfile, err = ioutil.TempFile(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data2)
	require.NoError(t, err)
	// update data, assumes default [--data-update-input true,true]
	execTokensCmd(t, homedir, fmt.Sprintf("update -r %s --token-identifier %X --data-file %s", backendUrl, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.UpdateNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID) {
			dataUpdateAttrs := &tokens.UpdateNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(dataUpdateAttrs))
			require.Equal(t, data2, dataUpdateAttrs.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	// check that data was updated on the backend
	require.Eventually(t, func() bool {
		return bytes.Equal(data2, ensureTokenIndexed(t, ctx, client, w1key.PubKey, nftID).NftData)
	}, 2*test.WaitDuration, test.WaitTick)

	// create non-updatable nft
	nftID2 := randomID(t)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible -r %s --type %X --token-identifier %X --data 01 --data-update-clause false", backendUrl, typeID, nftID2))
	nft2 := ensureTokenIndexed(t, ctx, client, w1key.PubKey, nftID2)
	require.Equal(t, []byte{0x01}, nft2.NftData)
	//try to update and observe failure
	execTokensCmd(t, homedir, fmt.Sprintf("update -r %s --token-identifier %X --data 02 --data-update-input false,true -w false", backendUrl, nftID2))
	testevent.ContainsEvent(t, partition.EventHandler, event.TransactionFailed)
}

func TestNFT_InvariantPredicate_Integration(t *testing.T) {
	partition, nodeDialUrl := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, client, ctx := startTokensBackend(t, nodeDialUrl)

	w1, homedirW1 := createNewTokenWallet(t, backendUrl)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, backendUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	symbol1 := "ABNFT"
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -r %s --symbol %s --type %X --inherit-bearer-clause %s", backendUrl, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID11)
	//second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -r %s --symbol %s --type %X --parent-type %X --subtype-input %s", backendUrl, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID12)
	//mint
	id := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -r %s --type %X --token-identifier %X --mint-input %s,%s", backendUrl, typeID12, id, predicatePtpkh, predicatePtpkh))
	ensureTokenIndexed(t, ctx, client, w1key.PubKey, id)
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -r %s", backendUrl)), "Symbol='ABNFT'")
	//send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible -r %s --token-identifier %X --address 0x%X -k 1 --inherit-bearer-input %s,%s", backendUrl, id, w2key.PubKey, predicateTrue, predicatePtpkh))
	ensureTokenIndexed(t, ctx, client, w2key.PubKey, id)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendUrl)), "Symbol='ABNFT'")
}
