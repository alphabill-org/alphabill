package cmd

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill/internal/partition/event"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestNFTs_Integration(t *testing.T) {
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	network := NewAlphabillNetwork(t)
	_, err := network.abNetwork.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	moneyBackendURL := network.moneyBackendURL
	tokenPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.walletHomedir
	//w1key := network.walletKey1
	w1key2 := network.walletKey2
	backendURL := network.tokenBackendURL
	backendClient := network.tokenBackendClient
	ctx := network.ctx

	w2, homedirW2 := createNewTokenWallet(t, backendURL)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	// send money to w1k2 to create fee credits
	stdout := execWalletCmd(t, homedirW1, fmt.Sprintf("send --amount 100 --address %s -r %s", hexutil.Encode(w1key2.PubKey), moneyBackendURL))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// create fee credit on w1k2
	stdout, err = execFeesCommand(homedirW1, fmt.Sprintf("--partition tokens add -k 2 --amount 50 -r %s -m %s", moneyBackendURL, backendURL))
	require.NoError(t, err)
	verifyStdout(t, stdout, "Successfully created 50 fee credits on tokens partition.")

	// non-fungible token types
	typeID := randomID(t)
	typeID2 := randomID(t)
	nftID := randomID(t)
	symbol := "ABNFT"
	execTokensCmdWithError(t, homedirW1, "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -k 2 --symbol %s -r %s --type %s --subtype-clause ptpkh", symbol, backendURL, typeID))
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key2.PubKey, typeID)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -k 2 --symbol %s -r %s --type %s --parent-type %s --subtype-input ptpkh", symbol+"2", backendURL, typeID2, typeID))
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key2.PubKey, typeID2)
	// mint NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -k 2 -r %s --type %s --token-identifier %s", backendURL, typeID, nftID))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return tx.PayloadType() == tokens.PayloadTypeMintNFT && bytes.Equal(tx.UnitID(), nftID)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenIndexed(t, ctx, backendClient, w1key2.PubKey, nftID)
	// transfer NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible -k 2 -r %s --token-identifier %s --address 0x%X", backendURL, nftID, w2key.PubKey))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return tx.PayloadType() == tokens.PayloadTypeTransferNFT && bytes.Equal(tx.UnitID(), nftID)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenIndexed(t, ctx, backendClient, w2key.PubKey, nftID)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendURL)), fmt.Sprintf("ID='%s'", nftID))
	//check what is left in w1, nothing, that is
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -k 2 -r %s", backendURL)), "No tokens")
	// list token types
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types -r %s", backendURL)), "symbol=ABNFT (nft)")
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types non-fungible -r %s", backendURL)), "symbol=ABNFT (nft)")

	// send money to w2 to create fee credits
	stdout = execWalletCmd(t, homedirW1, fmt.Sprintf("send --amount 100 --address %s -r %s", hexutil.Encode(w2key.PubKey), moneyBackendURL))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// create fee credit on w2
	stdout, err = execFeesCommand(homedirW2, fmt.Sprintf("--partition tokens add --amount 50 -r %s -m %s", moneyBackendURL, backendURL))
	require.NoError(t, err)
	verifyStdout(t, stdout, "Successfully created 50 fee credits on tokens partition.")

	// transfer back
	execTokensCmd(t, homedirW2, fmt.Sprintf("send non-fungible -r %s --token-identifier %s --address 0x%X -k 1", backendURL, nftID, w1key2.PubKey))
	ensureTokenIndexed(t, ctx, backendClient, w1key2.PubKey, nftID)
	// mint nft from w1 and set the owner to w2
	nftID2 := randomID(t)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendURL)), "No tokens")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -k 2 -r %s --type %s --bearer-clause ptpkh:0x%X --token-identifier %s", backendURL, typeID, w2key.PubKeyHash.Sha256, nftID2))
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendURL)), fmt.Sprintf("ID='%s'", nftID2))
}

func TestNFTDataUpdateCmd_Integration(t *testing.T) {
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	network := NewAlphabillNetwork(t)
	tokenPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedir := network.walletHomedir
	w1key := network.walletKey1
	backendURL := network.tokenBackendURL
	backendClient := network.tokenBackendClient
	ctx := network.ctx

	typeID := randomID(t)
	symbol := "ABNFT"
	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type non-fungible --symbol %s -r %s --type %s", symbol, backendURL, typeID))
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID)
	// create non-fungible token from using data-file
	nftID := randomID(t)
	data := make([]byte, 1024)
	n, err := rand.Read(data)
	require.NoError(t, err)
	require.EqualValues(t, n, len(data))
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible -r %s --type %s --token-identifier %s --data-file %s", backendURL, typeID, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		if tx.PayloadType() == tokens.PayloadTypeMintNFT && bytes.Equal(tx.UnitID(), nftID) {
			mintNonFungibleAttr := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.UnmarshalAttributes(mintNonFungibleAttr))
			require.Equal(t, data, mintNonFungibleAttr.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	nft := ensureTokenIndexed(t, ctx, backendClient, w1key.PubKey, nftID)
	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list non-fungible -r %s", backendURL)), fmt.Sprintf("ID='%s'", nftID))
	require.Equal(t, data, nft.NftData)
	// generate new data
	data2 := make([]byte, 1024)
	n, err = rand.Read(data2)
	require.NoError(t, err)
	require.EqualValues(t, n, len(data2))
	require.False(t, bytes.Equal(data, data2))
	tmpfile, err = os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data2)
	require.NoError(t, err)
	// update data, assumes default [--data-update-input true,true]
	execTokensCmd(t, homedir, fmt.Sprintf("update -r %s --token-identifier %s --data-file %s", backendURL, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		if tx.PayloadType() == tokens.PayloadTypeUpdateNFT && bytes.Equal(tx.UnitID(), nftID) {
			dataUpdateAttrs := &tokens.UpdateNonFungibleTokenAttributes{}
			require.NoError(t, tx.UnmarshalAttributes(dataUpdateAttrs))
			require.Equal(t, data2, dataUpdateAttrs.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	// check that data was updated on the backend
	require.Eventually(t, func() bool {
		return bytes.Equal(data2, ensureTokenIndexed(t, ctx, backendClient, w1key.PubKey, nftID).NftData)
	}, 2*test.WaitDuration, test.WaitTick)

	// create non-updatable nft
	nftID2 := randomID(t)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible -r %s --type %s --token-identifier %s --data 01 --data-update-clause false", backendURL, typeID, nftID2))
	nft2 := ensureTokenIndexed(t, ctx, backendClient, w1key.PubKey, nftID2)
	require.Equal(t, []byte{0x01}, nft2.NftData)
	//try to update and observe failure
	execTokensCmd(t, homedir, fmt.Sprintf("update -r %s --token-identifier %s --data 02 --data-update-input false,true -w false", backendURL, nftID2))
	testevent.ContainsEvent(t, tokenPartition.Nodes[0].EventHandler, event.TransactionFailed)
}

func TestNFT_InvariantPredicate_Integration(t *testing.T) {
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	network := NewAlphabillNetwork(t)
	tokenPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.walletHomedir
	w1key := network.walletKey1
	backendURL := network.tokenBackendURL
	backendClient := network.tokenBackendClient
	ctx := network.ctx

	w2, homedirW2 := createNewTokenWallet(t, backendURL)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	symbol1 := "ABNFT"
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -r %s --symbol %s --type %s --inherit-bearer-clause %s", backendURL, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID11)
	//second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -r %s --symbol %s --type %s --parent-type %s --subtype-input %s", backendURL, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID12)
	//mint
	id := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -r %s --type %s --token-identifier %s --mint-input %s,%s", backendURL, typeID12, id, predicatePtpkh, predicatePtpkh))
	ensureTokenIndexed(t, ctx, backendClient, w1key.PubKey, id)
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -r %s", backendURL)), "symbol='ABNFT'")
	//send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible -r %s --token-identifier %s --address 0x%X -k 1 --inherit-bearer-input %s,%s", backendURL, id, w2key.PubKey, predicateTrue, predicatePtpkh))
	ensureTokenIndexed(t, ctx, backendClient, w2key.PubKey, id)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", backendURL)), "symbol='ABNFT'")
}
