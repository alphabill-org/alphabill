package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	moneywallet "github.com/alphabill-org/alphabill/pkg/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	vdwallet "github.com/alphabill-org/alphabill/pkg/wallet/vd"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestVD_UseClientForTx(t *testing.T) {
	ctx := context.Background()
	network := NewVDAlphabillNetwork(t, ctx)

	fmt.Println("Sending register data tx")
	err := sendTxWithClient(ctx, network.vdNodeURL, network.walletHomeDir)
	require.NoError(t, err)

	err = sendTxWithClient(ctx, network.vdNodeURL, network.walletHomeDir)

	// failing case, send same stuff once again
	// There are two cases, how second 'register tx' gets rejected:
	if err != nil {
		// first, when both txs end up in the same block, this error is propagated here:
		fmt.Println("second tx rejected from the buffer")
		require.ErrorContains(t, err, "tx already in tx buffer")
	} else {
		// second, if the first tx has been processed, the second tx is rejected,
		// but the error is only printed to the log and not propagated back here (TODO)
		fmt.Println("second tx rejected, but error not propagated")
	}
}

func sendTxWithClient(ctx context.Context, vdNodeURL string, walletHomeDir string) error {
	cmd := New()
	args := "wallet vd register " +
		" --hash 0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD" +
		" -u " + vdNodeURL +
		" -l " + walletHomeDir +
		" --wait"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	return cmd.addAndExecuteCommand(ctx)
}

type VDAlphabillNetwork struct {
	abNetwork          *testpartition.AlphabillNetwork
	moneyBackendClient *moneyclient.MoneyBackendClient
	moneyBackendURL    string
	vdClient           *vdwallet.VDClient
	vdNodeURL          string
	walletHomeDir      string
	accountKey         *account.AccountKey
}

// Starts money partition, money backend, vd partition, vd client.
// Sends initial bill to money wallet.
// Adds fee credit on vd partition.
func NewVDAlphabillNetwork(t *testing.T, ctx context.Context) *VDAlphabillNetwork {
	initialBill := &money.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill)
	vdPartition := createVDPartition(t)
	abNet := startAlphabill(t, []*testpartition.NodePartition{moneyPartition, vdPartition})
	startPartitionRPCServers(t, moneyPartition)
	startPartitionRPCServers(t, vdPartition)

	moneyBackendURL, moneyBackendClient := startMoneyBackend(t, moneyPartition, initialBill)

	walletHomeDir := filepath.Join(t.TempDir(), "wallet")
	am, err := account.NewManager(walletHomeDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	require.NoError(t, am.CreateKeys(""))
	accountKey, err := am.GetAccountKey(0)

	moneyWallet, err := moneywallet.LoadExistingWallet(am, moneyBackendClient)
	require.NoError(t, err)
	t.Cleanup(moneyWallet.Close)

	vdClient, err := vdwallet.New(&vdwallet.VDClientConfig{
		VDNodeURL: vdPartition.Nodes[0].AddrGRPC,
		WaitForReady: true,
		ConfirmTx: true,
		ConfirmTxTimeout: 10,
		AccountKey: accountKey,
		WalletHomeDir: walletHomeDir,
	})
	require.NoError(t, err)
	require.NotNil(t, vdClient)
	t.Cleanup(vdClient.Close)

	vdTxPublisher := vdwallet.NewTxPublisher(vdClient)

	vdFeeManager := fees.NewFeeManager(am, money.DefaultSystemIdentifier, moneyWallet, moneyBackendClient, vd.DefaultSystemIdentifier, vdTxPublisher, vdClient)
	t.Cleanup(vdFeeManager.Close)

	spendInitialBillWithFeeCredits(t, abNet, initialBill, hexutil.Encode(accountKey.PubKey))
	time.Sleep(3 * time.Second) // TODO dynamic sleep

	// Add fee credit for VD partition
	_, err = vdFeeManager.AddFeeCredit(ctx, fees.AddFeeCmd{Amount: 1000})
	require.NoError(t, err)

	return &VDAlphabillNetwork{
		abNetwork:          abNet,
		moneyBackendClient: moneyBackendClient,
		moneyBackendURL:    moneyBackendURL,
		vdClient:           vdClient,
		vdNodeURL:          vdPartition.Nodes[0].AddrGRPC,
		walletHomeDir:      walletHomeDir,
		accountKey:         accountKey,
	}
}

func createVDPartition(t *testing.T) *testpartition.NodePartition {
	vdPartition, err := testpartition.NewPartition(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := vd.NewTxSystem(
			vd.WithSystemIdentifier(vd.DefaultSystemIdentifier),
			vd.WithHashAlgorithm(crypto.SHA256),
			vd.WithTrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, vd.DefaultSystemIdentifier)
	require.NoError(t, err)
	return vdPartition
}
