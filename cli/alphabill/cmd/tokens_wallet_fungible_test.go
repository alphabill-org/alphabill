package cmd

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	tokentxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	abclient "github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestFungibleToken_Subtyping_Integration(t *testing.T) {
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	network := NewAlphabillNetwork(t)
	tokensPartition, err := network.abNetwork.GetNodePartition(tokentxs.DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.walletHomedir
	w1key := network.walletKey1
	backendURL := network.tokenBackendURL
	backendClient := network.tokenBackendClient
	ctx := network.ctx

	symbol1 := "AB"
	// test subtyping
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	typeID13 := randomID(t)
	typeID14 := randomID(t)
	//push bool false, equal; to satisfy: 5100
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause 0x53510087", backendURL, symbol1, typeID11))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID11)
	//second type
	//--parent-type without --subtype-input gives error
	execTokensCmdWithError(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X", backendURL, symbol1, typeID12, "ptpkh", typeID11), "missing [subtype-input]")
	//--subtype-input without --parent-type also gives error
	execTokensCmdWithError(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --subtype-input %s", backendURL, symbol1, typeID12, "ptpkh", "0x535100"), "missing [parent-type]")
	//inheriting the first one and setting subtype clause to ptpkh
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", backendURL, symbol1, typeID12, "ptpkh", typeID11, "0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID12)
	//third type needs to satisfy both parents, immediate parent with ptpkh, grandparent with 0x535100
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", backendURL, symbol1, typeID13, "true", typeID12, "ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID13)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID13)
	//4th type
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", backendURL, symbol1, typeID14, "true", typeID13, "empty,ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID14)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID14)
}

func TestFungibleToken_InvariantPredicate_Integration(t *testing.T) {
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	network := NewAlphabillNetwork(t)
	tokensPartition, err := network.abNetwork.GetNodePartition(tokentxs.DefaultTokenTxSystemIdentifier)
	homedirW1 := network.walletHomedir
	w1key := network.walletKey1
	backendUrl := network.tokenBackendURL
	backendClient := network.tokenBackendClient
	ctx := network.ctx

	w2, homedirW2 := createNewTokenWallet(t, backendUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	symbol1 := "AB"
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s  --symbol %s --type %X --decimals 0 --inherit-bearer-clause %s", backendUrl, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID11)
	//second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s  --symbol %s --type %X --decimals 0 --parent-type %X --subtype-input %s", backendUrl, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, backendClient, w1key.PubKey, typeID12)
	//mint
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible -r %s  --type %X --amount %v --mint-input %s,%s", backendUrl, typeID12, 1000, predicatePtpkh, predicatePtpkh))
	ensureTokenIndexed(t, ctx, backendClient, w1key.PubKey, nil)
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='1'000'")
	//send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %X --amount 100 --address 0x%X -k 1 --inherit-bearer-input %s,%s", backendUrl, typeID12, w2key.PubKey, predicateTrue, predicatePtpkh))
	ensureTokenIndexed(t, ctx, backendClient, w2key.PubKey, nil)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='100'")
}

func TestFungibleTokens_Sending_Integration(t *testing.T) {
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	network := NewAlphabillNetwork(t)
	_, err := network.abNetwork.GetNodePartition(defaultABMoneySystemIdentifier)
	require.NoError(t, err)
	moneyBackendURL := network.moneyBackendURL
	tokensPartition, err := network.abNetwork.GetNodePartition(tokentxs.DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.walletHomedir
	w1key := network.walletKey1
	backendUrl := network.tokenBackendURL

	w2, homedirW2 := createNewTokenWallet(t, backendUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	typeID1 := randomID(t)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, homedirW1, "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible  --symbol %s -r %s --type %X --decimals 0", symbol1, backendUrl, typeID1))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types fungible -r %s", backendUrl)), "symbol=AB (fungible)")
	// mint tokens
	crit := func(amount uint64) func(tx *types.TransactionOrder) bool {
		return func(tx *types.TransactionOrder) bool {
			if tx.PayloadType() == tokentxs.PayloadTypeMintFungibleToken {
				attrs := &tokentxs.MintFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				return attrs.Value == amount
			}
			return false
		}
	}
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible  -r %s --type %X --amount 5", backendUrl, typeID1))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible  -r %s --type %X --amount 9", backendUrl, typeID1))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, crit(5)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, crit(9)), test.WaitDuration, test.WaitTick)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl))
	}, "amount='5'", "amount='9'", "Symbol='AB'")
	// check w2 is empty
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible  -r %s", backendUrl)), "No tokens")
	// transfer tokens w1 -> w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %X --amount 6 --address 0x%X -k 1", backendUrl, typeID1, w2key.PubKey)) //split (9=>6+3)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl))
	}, "amount='5'", "amount='3'", "Symbol='AB'")
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %X --amount 6 --address 0x%X -k 1", backendUrl, typeID1, w2key.PubKey)) //transfer (5) + split (3=>2+1)
	//check immediately as tx must be confirmed
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='6'", "amount='5'", "amount='1'", "Symbol='AB'")
	//check what is left in w1
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl))
	}, "amount='2'")

	// send money to w2 to create fee credits
	stdout := execWalletCmd(t, "", homedirW1, fmt.Sprintf("send --amount 100 --address %s -r %s", hexutil.Encode(w2key.PubKey), moneyBackendURL))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// create fee credit on w2
	stdout, err = execFeesCommand(homedirW2, fmt.Sprintf("--partition token add --amount 50 -r %s -m %s", moneyBackendURL, backendUrl))
	require.NoError(t, err)
	verifyStdout(t, stdout, "Successfully created 50 fee credits on token partition.")

	//transfer back w2->w1 (AB-513)
	execTokensCmd(t, homedirW2, fmt.Sprintf("send fungible -r %s --type %X --amount 6 --address 0x%X -k 1", backendUrl, typeID1, w1key.PubKey))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='2'", "amount='6'")
}

func TestWalletCreateFungibleTokenTypeAndTokenAndSendCmd_IntegrationTest(t *testing.T) {
	const decimals = 3
	// mint tokens
	crit := func(amount uint64) func(tx *types.TransactionOrder) bool {
		return func(tx *types.TransactionOrder) bool {
			if tx.PayloadType() == tokentxs.PayloadTypeMintFungibleToken {
				attrs := &tokentxs.MintFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				return attrs.Value == amount
			}
			return false
		}
	}

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	network := NewAlphabillNetwork(t)
	tokensPart, err := network.abNetwork.GetNodePartition(tokentxs.DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	homedir := network.walletHomedir
	w1key := network.walletKey1
	backendUrl := network.tokenBackendURL
	tokenBackendClient := network.tokenBackendClient
	ctx := network.ctx

	w2, homedirW2 := createNewTokenWallet(t, backendUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()
	typeID := util.Uint256ToBytes(uint256.NewInt(uint64(0x10)))
	symbol := "AB"
	name := "Long name for AB"
	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type fungible  --symbol %s --name %s -r %s --type %X --decimals %v", symbol, name, backendUrl, typeID, decimals))
	ensureTokenTypeIndexed(t, ctx, tokenBackendClient, w1key.PubKey, typeID)
	// non-existing id
	nonExistingTypeId := util.Uint256ToBytes(uint256.NewInt(uint64(0x11)))
	// verify error
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 3", backendUrl, nonExistingTypeId), fmt.Sprintf("failed to load type with id %X", nonExistingTypeId))
	// new token creation fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 0", backendUrl, typeID), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 00.000", backendUrl, typeID), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 00.0.00", backendUrl, typeID), fmt.Sprintf("more than one comma"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount .00", backendUrl, typeID), fmt.Sprintf("missing integer part"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount a.00", backendUrl, typeID), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 0.0a", backendUrl, typeID), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.1111", backendUrl, typeID), fmt.Sprintf("invalid precision"))
	// out of range because decimals = 3 the value is equal to 18446744073709551615000
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 18446744073709551615", backendUrl, typeID), fmt.Sprintf("out of range"))
	// creation succeeds
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 3", backendUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.1", backendUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.11", backendUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.111", backendUrl, typeID))
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(3000)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(1100)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(1110)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(1111)), test.WaitDuration, test.WaitTick)
	// mint tokens from w1 and set the owner to w2
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 2.222 --bearer-clause ptpkh:0x%X", backendUrl, typeID, w2key.PubKeyHash.Sha256))
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(2222)), test.WaitDuration, test.WaitTick)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='2.222'")

	// test send fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 2 --address 0x%X -k 1", backendUrl, nonExistingTypeId, w2key.PubKey), fmt.Sprintf("failed to load type with id %X", nonExistingTypeId))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 0 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 000.000 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 00.0.00 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("more than one comma"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount .00 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("missing integer part"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount a.00 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 1.1111 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("invalid precision"))
}

func TestFungibleTokens_CollectDust_Integration(t *testing.T) {
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))
	network := NewAlphabillNetwork(t)
	homedir := network.walletHomedir
	backendUrl := network.tokenBackendURL

	typeID1 := randomID(t)
	symbol1 := "AB"
	execTokensCmd(t, homedir, fmt.Sprintf("new-type fungible --symbol %s -r %s --type %X --decimals 0", symbol1, backendUrl, typeID1))
	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list-types fungible -r %s", backendUrl)), "symbol=AB (fungible)")
	// mint tokens (without confirming, for speed)
	mintIterations := 110
	expectedAmounts := make([]string, 0, mintIterations)
	expectedTotal := 0
	for i := 1; i <= mintIterations; i++ {
		execTokensCmd(t, homedir, fmt.Sprintf("new fungible -r %s --type %X --amount %v -w false", backendUrl, typeID1, i))
		expectedAmounts = append(expectedAmounts, fmt.Sprintf("amount='%v'", i))
		expectedTotal += i
	}
	//check w1
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execTokensCmd(t, homedir, fmt.Sprintf("list fungible -r %s", backendUrl))
	}, expectedAmounts...)
	// DC
	execTokensCmd(t, homedir, fmt.Sprintf("collect-dust -r %s", backendUrl))

	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list fungible -r %s", backendUrl)), fmt.Sprintf("amount='%v'", insertSeparator(fmt.Sprint(expectedTotal), false)))
}

type AlphabillNetwork struct {
	abNetwork          *testpartition.AlphabillNetwork
	moneyBackendClient *moneyclient.MoneyBackendClient
	moneyBackendURL    string

	tokenBackendClient *client.TokenBackend
	tokenBackendURL    string

	walletHomedir string
	walletKey1    *account.AccountKey
	walletKey2    *account.AccountKey
	ctx           context.Context
}

// starts money partition, money backend, ut partition, ut backend
// sends initial bill to money wallet
// creates fee credit on money wallet and token wallet
func NewAlphabillNetwork(t *testing.T) *AlphabillNetwork {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill)
	tokensPartition := createTokensPartition(t)
	abNet := startAlphabill(t, []*testpartition.NodePartition{moneyPartition, tokensPartition})
	startPartitionRPCServers(t, moneyPartition)
	startPartitionRPCServers(t, tokensPartition)

	moneyBackendURL, moneyBackendClient := startMoneyBackend(t, moneyPartition, initialBill)

	tokenBackendURL, tokenBackendClient, ctx := startTokensBackend(t, tokensPartition.Nodes[0].AddrGRPC)

	homedirW1 := t.TempDir()
	walletDir := filepath.Join(homedirW1, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))

	moneyWallet, err := money.LoadExistingWallet(abclient.AlphabillClientConfig{Uri: moneyPartition.Nodes[0].AddrGRPC}, am, moneyBackendClient)
	require.NoError(t, err)
	t.Cleanup(moneyWallet.Shutdown)

	tokenTxPublisher := tokens.NewTxPublisher(tokenBackendClient)
	tokenFeeManager := fees.NewFeeManager(am, defaultABMoneySystemIdentifier, moneyWallet, moneyBackendClient, tokentxs.DefaultTokenTxSystemIdentifier, tokenTxPublisher, tokenBackendClient)

	w1, err := tokens.New(tokentxs.DefaultTokenTxSystemIdentifier, tokenBackendURL, am, true, tokenFeeManager)
	require.NoError(t, err)
	require.NotNil(t, w1)
	t.Cleanup(w1.Shutdown)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	_, _, err = am.AddAccount()
	require.NoError(t, err)
	w1key2, err := w1.GetAccountManager().GetAccountKey(1)

	spendInitialBillWithFeeCredits(t, abNet, initialBill, hexutil.Encode(w1key.PubKey))
	time.Sleep(2 * time.Second) // TODO dynamic sleep

	// create fees on money partition
	_, err = moneyWallet.AddFeeCredit(ctx, fees.AddFeeCmd{Amount: 1000})
	require.NoError(t, err)

	// create fees on token partition
	_, err = w1.AddFeeCredit(ctx, fees.AddFeeCmd{Amount: 1000})
	require.NoError(t, err)
	w1.Shutdown()

	return &AlphabillNetwork{
		abNetwork:          abNet,
		moneyBackendClient: moneyBackendClient,
		moneyBackendURL:    moneyBackendURL,
		tokenBackendClient: tokenBackendClient,
		tokenBackendURL:    tokenBackendURL,
		walletHomedir:      homedirW1,
		walletKey1:         w1key,
		walletKey2:         w1key2,
		ctx:                ctx,
	}
}
