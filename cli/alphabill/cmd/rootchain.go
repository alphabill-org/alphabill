package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ainvaltin/httpsrv"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/trustbase"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
)

const (
	boltRootChainStoreFileName = "rootchain.db"
	boltTrustBaseStoreFileName = "trustbase.db"
	rootBootStrapNodesCmdFlag  = "bootnodes"
	defaultNetworkTimeout      = 300 * time.Millisecond
)

type rootNodeConfig struct {
	Base               *baseConfiguration
	KeyFile            string   // path to rootchain chain key file
	GenesisFile        string   // path to rootchain-genesis.json file
	TrustBaseFile      string   // path to root-trust-base.json file
	Address            string   // node address (libp2p multiaddress format)
	AnnounceAddrs      []string // node public ip addresses (libp2p multiaddress format)
	StoragePath        string   // path to Bolt storage file
	MaxRequests        uint     // validator partition certification request channel capacity
	BootStrapAddresses []string // boot strap addresses (libp2p multiaddress format)
	RPCServerAddress   string   // address on which http server is exposed with metrics endpoint
}

// newRootNodeCmd creates a new cobra command for root chain node
func newRootNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &rootNodeConfig{
		Base: baseConfig,
	}
	var cmd = &cobra.Command{
		Use:   "root",
		Short: "Starts a root chain node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRootNode(cmd.Context(), config)
		},
	}

	cmd.Flags().StringVarP(&config.KeyFile, keyFileCmdFlag, "k", "", "path to node validator key file  (default $AB_HOME/rootchain/"+defaultKeysFileName+")")
	cmd.Flags().StringVar(&config.GenesisFile, "genesis-file", "", "path to root-genesis.json file (default $AB_HOME/rootchain/"+rootGenesisFileName+")")
	cmd.Flags().StringVar(&config.TrustBaseFile, "trust-base-file", "", "path to root-trust-base.json file (default $AB_HOME/"+rootTrustBaseFileName+")")
	cmd.Flags().StringVar(&config.StoragePath, "db", "", "persistent store path (default: $AB_HOME/rootchain/)")
	cmd.Flags().StringVar(&config.Address, "address", "/ip4/127.0.0.1/tcp/26662", "validator address in libp2p multiaddress-format")
	cmd.Flags().StringSliceVar(&config.AnnounceAddrs, "announce-addresses", nil, "validator public ip addresses in libp2p multiaddress-format, if specified overwrites any and all default listen addresses")
	cmd.Flags().UintVar(&config.MaxRequests, "max-requests", 1000, "request buffer capacity")
	cmd.Flags().StringSliceVar(&config.BootStrapAddresses, rootBootStrapNodesCmdFlag, nil, "list of bootstrap root node addresses in libp2p multiaddress format")
	cmd.Flags().StringVar(&config.RPCServerAddress, "rpc-server-address", "", `Specifies the TCP address for the RPC server to listen on, in the form "host:port". RPC server isn't initialised if address is empty.`)
	return cmd
}

// getGenesisFilePath returns genesis file path if provided, otherwise $AB_HOME/rootchain/root-genesis.json
// Must be called after $AB_HOME is initialized in base command PersistentPreRunE function.
func (c *rootNodeConfig) getGenesisFilePath() string {
	if c.GenesisFile != "" {
		return c.GenesisFile
	}
	return filepath.Join(c.Base.defaultRootchainDir(), rootGenesisFileName)
}

// getRootTrustBaseFilePath returns root trust base file path if provided, otherwise $AB_HOME/root-trust-base.json
// Must be called after $AB_HOME is initialized in base command PersistentPreRunE function.
func (c *rootNodeConfig) getRootTrustBaseFilePath() string {
	if c.TrustBaseFile != "" {
		return c.TrustBaseFile
	}
	return filepath.Join(c.Base.HomeDir, rootTrustBaseFileName)
}

func (c *rootNodeConfig) getStorageDir() string {
	if c.StoragePath != "" {
		return c.StoragePath
	}
	return c.Base.defaultRootchainDir()
}

func (c *rootNodeConfig) getKeyFilePath() string {
	if c.KeyFile != "" {
		return c.KeyFile
	}
	return filepath.Join(c.Base.defaultRootchainDir(), defaultKeysFileName)
}

func getBootStrapNodes(bootNodesStr []string) ([]peer.AddrInfo, error) {
	bootNodes := make([]peer.AddrInfo, len(bootNodesStr))
	for i, str := range bootNodesStr {
		addrInfo, err := peer.AddrInfoFromString(str)
		if err != nil {
			return nil, fmt.Errorf("invalid bootstrap node parameter: %w", err)
		}
		bootNodes[i] = *addrInfo
	}
	return bootNodes, nil
}

func initRootStore(dbPath string) (*boltdb.BoltDB, error) {
	if dbPath != "" {
		return boltdb.New(filepath.Join(dbPath, boltRootChainStoreFileName))
	}
	return nil, fmt.Errorf("persistent storage path not set")
}

func initTrustBaseStore(dbPath string) (*boltdb.BoltDB, error) {
	if dbPath != "" {
		return boltdb.New(filepath.Join(dbPath, boltTrustBaseStoreFileName))
	}
	return nil, fmt.Errorf("persistent storage path not set")
}

func runRootNode(ctx context.Context, config *rootNodeConfig) error {
	gf, err := os.Open(config.getGenesisFilePath())
	if err != nil {
		return fmt.Errorf("opening root genesis file: %w", err)
	}
	rootGenesis, err := parseRootGenesis(gf)
	if err != nil {
		return fmt.Errorf("reading root genesis file: %w", err)
	}

	keys, err := LoadKeys(config.getKeyFilePath(), false, false)
	if err != nil {
		return fmt.Errorf("loading keys from %s: %w", config.KeyFile, err)
	}
	host, err := createHost(ctx, keys, config)
	if err != nil {
		return fmt.Errorf("creating partition host: %w", err)
	}
	log := config.Base.observe.Logger().With(logger.NodeID(host.ID()))
	obs := observability.WithLogger(config.Base.observe, log)
	partitionNet, err := network.NewLibP2PRootChainNetwork(host, config.MaxRequests, defaultNetworkTimeout, obs)
	if err != nil {
		return fmt.Errorf("partition network initialization failed: %w", err)
	}
	ver, err := keys.Signer.Verifier()
	if err != nil {
		return fmt.Errorf("invalid root node signing key: %w", err)
	}
	if err = verifyKeyPresentInGenesis(host.ID(), rootGenesis.Root, ver); err != nil {
		return fmt.Errorf("root node key not found in genesis: %w", err)
	}
	// Initiate root storage
	rootStore, err := initRootStore(config.getStorageDir())
	if err != nil {
		return fmt.Errorf("root store init failed: %w", err)
	}
	// init trust base store
	trustBaseStore, err := initTrustBaseStore(config.getStorageDir())
	if err != nil {
		return fmt.Errorf("trust base store init failed: %w", err)
	}
	// load or create trust base
	trustBase, err := initTrustBase(trustBaseStore, config.getRootTrustBaseFilePath())
	if err != nil {
		return fmt.Errorf("root trust base init failed: %w", err)
	}

	rootNet, err := network.NewLibP2RootConsensusNetwork(host, config.MaxRequests, defaultNetworkTimeout, obs)
	if err != nil {
		return fmt.Errorf("failed initiate root network, %w", err)
	}

	orchestration, err := partitions.NewOrchestration(rootGenesis, filepath.Join(config.getStorageDir(), "orchestration.db"))
	if err != nil {
		return fmt.Errorf("creating orchestration: %w", err)
	}
	cm, err := consensus.NewConsensusManager(
		host.ID(),
		rootGenesis,
		trustBase,
		orchestration,
		rootNet,
		keys.Signer,
		obs,
		consensus.WithStorage(rootStore),
	)
	if err != nil {
		return fmt.Errorf("failed initiate distributed consensus manager: %w", err)
	}
	if err = host.BootstrapConnect(ctx, log); err != nil {
		return err
	}
	node, err := rootchain.New(
		host,
		partitionNet,
		cm,
		obs,
	)
	if err != nil {
		return fmt.Errorf("failed initiate root node: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error { return node.Run(ctx) })

	g.Go(func() error {
		if config.RPCServerAddress == "" {
			return nil // do not kill the group!
		}

		mux := http.NewServeMux()
		if pr := config.Base.observe.PrometheusRegisterer(); pr != nil {
			mux.Handle("/api/v1/metrics", promhttp.HandlerFor(pr.(prometheus.Gatherer), promhttp.HandlerOpts{MaxRequestsInFlight: 1}))
		}
		mux.HandleFunc("PUT /api/v1/configurations", putShardConfigHandler(orchestration.AddShardConfig))
		return httpsrv.Run(ctx,
			&http.Server{
				Addr:              config.RPCServerAddress,
				Handler:           mux,
				ReadTimeout:       3 * time.Second,
				ReadHeaderTimeout: time.Second,
				WriteTimeout:      5 * time.Second,
				IdleTimeout:       30 * time.Second,
			})
	})

	return g.Wait()
}

func createHost(ctx context.Context, keys *Keys, cfg *rootNodeConfig) (*network.Peer, error) {
	bootNodes, err := getBootStrapNodes(cfg.BootStrapAddresses)
	if err != nil {
		return nil, fmt.Errorf("boot nodes parameter error: %w", err)
	}
	keyPair, err := keys.getAuthKeyPair()
	if err != nil {
		return nil, fmt.Errorf("get key pair failed: %w", err)
	}
	peerConf, err := network.NewPeerConfiguration(cfg.Address, cfg.AnnounceAddrs, keyPair, bootNodes)
	if err != nil {
		return nil, err
	}
	return network.NewPeer(ctx, peerConf, cfg.Base.observe.Logger(), cfg.Base.observe.PrometheusRegisterer())
}

func verifyKeyPresentInGenesis(nodeID peer.ID, rg *genesis.GenesisRootRecord, ver abcrypto.Verifier) error {
	nodeInfo := rg.FindRootValidatorByNodeID(nodeID.String())
	if nodeInfo == nil {
		return fmt.Errorf("node id/encode key not found in genesis")
	}
	sigKey, err := ver.MarshalPublicKey()
	if err != nil {
		return fmt.Errorf("invalid root node signing key: %w", err)
	}
	// verify that the same public key is present in the genesis file
	if !bytes.Equal(sigKey, nodeInfo.SigKey) {
		return fmt.Errorf("signing key not found in genesis file")
	}
	return nil
}

// initTrustBase returns the stored trust base if it exists, if it does not exist then
// verifies that the genesis trust base file is provided, stores it, and returns it.
func initTrustBase(store keyvaluedb.KeyValueDB, trustBaseFile string) (types.RootTrustBase, error) {
	trustBaseStore, err := trustbase.NewStore(store)
	if err != nil {
		return nil, fmt.Errorf("consensus trust base storage init failed: %w", err)
	}
	// TODO latest epoch number must be provided externally or stored internally
	trustBase, err := trustBaseStore.LoadTrustBase(0)
	if err != nil {
		return nil, fmt.Errorf("failed to load trust base: %w", err)
	}
	if trustBase != nil {
		return trustBase, nil
	}
	trustBase, err = types.NewTrustBaseFromFile(trustBaseFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create trust base: %w", err)
	}
	if err := trustBaseStore.StoreTrustBase(0, trustBase); err != nil {
		return nil, fmt.Errorf("failed to store trust base: %w", err)
	}
	return trustBase, nil
}

func putShardConfigHandler(addVarFn func(rec *partitions.ValidatorAssignmentRecord) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := parseVAR(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "parsing var request body: %v", err)
			return
		}

		if err := addVarFn(rec); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "registering configurations: %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func parseRootGenesis(r io.ReadCloser) (*genesis.RootGenesis, error) {
	defer r.Close()
	rg := &genesis.RootGenesis{Version: 1}
	if err := json.NewDecoder(r).Decode(&rg); err != nil {
		return nil, fmt.Errorf("decoding root genesis: %w", err)
	}
	if err := rg.Verify(); err != nil {
		return nil, fmt.Errorf("invalid root genesis: %w", err)
	}
	return rg, nil
}

func parseVAR(r io.ReadCloser) (*partitions.ValidatorAssignmentRecord, error) {
	defer r.Close()
	var rec *partitions.ValidatorAssignmentRecord
	if err := json.NewDecoder(r).Decode(&rec); err != nil {
		return nil, fmt.Errorf("decoding var json: %w", err)
	}
	return rec, nil
}
