package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/rootchain"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/trustbase"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
)

const (
	rootStoreFileName          = "rootchain.db"
	trustBaseStoreFileName     = "trustbase.db"
	orchestrationStoreFileName = "orchestration.db"
	defaultNetworkTimeout      = 300 * time.Millisecond
)

type (
	rootNodeRunFlags struct {
		*baseFlags
		keyConfFlags
		trustBaseFlags
		p2pFlags

		RootStoreFile          string   // path to Bolt storage file
		TrustBaseStoreFile     string
		OrchestrationStoreFile string
		ShardConfFiles         []string // paths to shard conf files

		BlockRate        uint32
		MaxRequests      uint   // validator partition certification request channel capacity
		RPCServerAddress string // address on which http server is exposed with metrics endpoint
	}
)

func newRootNodeCmd(baseFlags *baseFlags) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "root-node",
		Short: "Tools to run a root node",
	}

	cmd.AddCommand(rootNodeInitCmd(baseFlags))
	cmd.AddCommand(rootNodeRunCmd(baseFlags))
	return cmd
}

func rootNodeInitCmd(baseFlags *baseFlags) *cobra.Command {
	// Currently root node init is the same as shard node init
	flags := &shardNodeInitFlags{baseFlags: baseFlags}

	var cmd = &cobra.Command{
		Use:   "init",
		Short: "Generate node keys and a public node info file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return shardNodeInit(cmd.Context(), flags)
		},
	}

	flags.addKeyConfFlags(cmd, true)
	return cmd
}

func rootNodeRunCmd(baseFlags *baseFlags) *cobra.Command {
	flags := &rootNodeRunFlags{baseFlags: baseFlags}
	var cmd = &cobra.Command{
		Use:   "run",
		Short: "Run a node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootNodeRun(cmd.Context(), flags)
		},
	}

	flags.addKeyConfFlags(cmd, false)
	flags.addTrustBaseFlags(cmd)
	flags.addP2PFlags(cmd)

	cmd.Flags().UintVar(&flags.MaxRequests, "max-requests", 1000, "request buffer capacity")
	cmd.Flags().StringVar(&flags.RPCServerAddress, "rpc-server-address", "",
		`Specifies the TCP address for the RPC server to listen on, in the form "host:port". RPC server isn't initialised if address is empty.`)

	cmd.Flags().StringVar(&flags.RootStoreFile, "root-db", "",
		fmt.Sprintf("path to the root database (default: %s)", filepath.Join("$AB_HOME", rootStoreFileName)))
	cmd.Flags().StringVar(&flags.TrustBaseStoreFile, "trust-base-db", "",
		fmt.Sprintf("path to the trust base database (default: %s)", filepath.Join("$AB_HOME", trustBaseStoreFileName)))
	cmd.Flags().StringVar(&flags.OrchestrationStoreFile, "orchestration-db", "",
		fmt.Sprintf("path to the orchestration database (default: %s)", filepath.Join("$AB_HOME", orchestrationStoreFileName)))

	cmd.Flags().StringSliceVarP(&flags.ShardConfFiles, "shard-conf", "", []string{}, "path to shard conf files")
	cmd.Flags().Uint32Var(&flags.BlockRate, "block-rate", consensus.BlockRate, "block rate (consensus parameter)")

	hideFlags(cmd, "block-rate")
	return cmd
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

func rootNodeRun(ctx context.Context, flags *rootNodeRunFlags) error {
	keyConf, err := flags.loadKeyConf(flags.baseFlags, false)
	if err != nil {
		return err
	}

	nodeID, err := keyConf.NodeID()
	if err != nil {
		return fmt.Errorf("failed to calculate nodeID: %w", err)
	}
	log := flags.observe.Logger().With(logger.NodeID(nodeID))
	obs := observability.WithLogger(flags.observe, log)

	host, err := createHost(ctx, keyConf, flags, obs)
	if err != nil {
		return fmt.Errorf("creating partition host: %w", err)
	}
	partitionNet, err := network.NewLibP2PRootChainNetwork(host, flags.MaxRequests, defaultNetworkTimeout, obs)
	if err != nil {
		return fmt.Errorf("partition network initialization failed: %w", err)
	}

	rootStore, err := flags.initStore(flags.RootStoreFile, rootStoreFileName)
	if err != nil {
		return err
	}
	trustBaseStore, err := flags.initStore(flags.TrustBaseStoreFile, trustBaseStoreFileName)
	if err != nil {
		return err
	}

	// load trust base
	trustBase, err := loadTrustBase(trustBaseStore, flags)
	if err != nil {
		return fmt.Errorf("root trust base init failed: %w", err)
	}

	signer, err := keyConf.Signer()
	if err != nil {
		return err
	}
	ver, err := signer.Verifier()
	if err != nil {
		return fmt.Errorf("invalid root node signing key: %w", err)
	}
	if err = verifyKeyPresentInTrustBase(host.ID(), trustBase, ver); err != nil {
		return fmt.Errorf("root node key not found in trust base: %w", err)
	}

	rootNet, err := network.NewLibP2RootConsensusNetwork(host, flags.MaxRequests, defaultNetworkTimeout, obs)
	if err != nil {
		return fmt.Errorf("failed initiate root network, %w", err)
	}

	orchestrationStorePath := flags.pathWithDefault(flags.OrchestrationStoreFile, orchestrationStoreFileName)
	orchestration, err := partitions.NewOrchestration(trustBase.GetNetworkID(), orchestrationStorePath, log)
	if err != nil {
		return fmt.Errorf("creating orchestration: %w", err)
	}

	if err := loadShardConfFiles(flags.ShardConfFiles, orchestration); err != nil {
		return fmt.Errorf("failed to load shard conf files: %w", err)
	}

	consensusParams := consensus.NewConsensusParams()
	consensusParams.BlockRate = time.Duration(flags.BlockRate) * time.Millisecond

	cm, err := consensus.NewConsensusManager(
		host.ID(),
		trustBase,
		orchestration,
		rootNet,
		signer,
		obs,
		consensus.WithStorage(rootStore),
		consensus.WithConsensusParams(*consensusParams),
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
		if flags.RPCServerAddress == "" {
			return nil // do not kill the group!
		}

		mux := http.NewServeMux()
		if pr := flags.observe.PrometheusRegisterer(); pr != nil {
			mux.Handle("/api/v1/metrics", promhttp.HandlerFor(pr.(prometheus.Gatherer), promhttp.HandlerOpts{MaxRequestsInFlight: 1}))
		}
		mux.HandleFunc("PUT /api/v1/configurations", putShardConfigHandler(orchestration.AddShardConfig))
		mux.HandleFunc("GET /api/v1/roundInfo", getRoundInfoHandler(cm.GetState, obs))
		return httpsrv.Run(ctx,
			&http.Server{
				Addr:              flags.RPCServerAddress,
				Handler:           mux,
				ReadTimeout:       3 * time.Second,
				ReadHeaderTimeout: time.Second,
				WriteTimeout:      5 * time.Second,
				IdleTimeout:       30 * time.Second,
			})
	})

	return g.Wait()
}

func createHost(ctx context.Context, keyConf *partition.KeyConf, flags *rootNodeRunFlags, obs Observability) (*network.Peer, error) {
	bootNodes, err := getBootStrapNodes(flags.BootstrapAddresses)
	if err != nil {
		return nil, fmt.Errorf("boot nodes parameter error: %w", err)
	}
	authKeyPair, err := keyConf.AuthKeyPair()
	if err != nil {
		return nil, fmt.Errorf("invalid authentication key: %w", err)
	}
	peerConf, err := network.NewPeerConfiguration(flags.Address, flags.AnnounceAddresses, authKeyPair, bootNodes)
	if err != nil {
		return nil, err
	}
	return network.NewPeer(ctx, peerConf, obs.Logger(), obs.PrometheusRegisterer())
}

func verifyKeyPresentInTrustBase(nodeID peer.ID, trustBase types.RootTrustBase, ver abcrypto.Verifier) error {
	nodeIDStr := nodeID.String()
	for _, node := range trustBase.GetRootNodes() {
		if nodeIDStr != node.NodeID {
			continue
		}

		sigKey, err := ver.MarshalPublicKey()
		if err != nil {
			return fmt.Errorf("invalid root node signing key: %w", err)
		}
		// verify that the same public key is present in the genesis file
		if !bytes.Equal(sigKey, node.SigKey) {
			return fmt.Errorf("different signing key in trust base")
		}
		return nil
	}

	return fmt.Errorf("node not part of trust base")
}

// loadTrustBase returns the stored trust base if it exists, otherwise
// loads and the stores the trust base from given file.
func loadTrustBase(store keyvaluedb.KeyValueDB, flags *rootNodeRunFlags) (types.RootTrustBase, error) {
	trustBaseStore, err := trustbase.NewStore(store)
	if err != nil {
		return nil, fmt.Errorf("consensus trust base storage init failed: %w", err)
	}
	trustBase, err := trustBaseStore.LoadTrustBase(0)
	if err != nil {
		return nil, fmt.Errorf("failed to load trust base: %w", err)
	}
	if trustBase != nil {
		return trustBase, nil
	}

	trustBase, err = flags.loadTrustBase(flags.baseFlags)
	if err != nil {
		return nil, fmt.Errorf("failed to create trust base: %w", err)
	}

	if err := trustBaseStore.StoreTrustBase(0, trustBase); err != nil {
		return nil, fmt.Errorf("failed to store trust base: %w", err)
	}
	return trustBase, nil
}

func putShardConfigHandler(addShardConfFn func(shardConf *types.PartitionDescriptionRecord) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shardConf, err := parseShardConf(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "parsing request body: %v", err)
			return
		}

		if err := addShardConfFn(shardConf); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "registering shard conf: %v", err)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func parseShardConf(r io.ReadCloser) (*types.PartitionDescriptionRecord, error) {
	defer r.Close()
	var shardConf *types.PartitionDescriptionRecord
	if err := json.NewDecoder(r).Decode(&shardConf); err != nil {
		return nil, fmt.Errorf("decoding shard conf json: %w", err)
	}
	return shardConf, nil
}

type (
	shardInfo struct {
		PartitionID types.PartitionID `json:"partitionId"`
		ShardID     types.ShardID     `json:"shardId"`
		RoundNumber uint64            `json:"roundNumber,string"`
		EpochNumber uint64            `json:"epochNumber,string"`
	}
	roundInfoResponse struct {
		RoundNumber     uint64      `json:"roundNumber,string"`
		EpochNumber     uint64      `json:"epochNumber,string"`
		PartitionShards []shardInfo `json:"partitionShards"`
	}
)

func loadShardConfFiles(paths []string, orchestration *partitions.Orchestration) error {
	for _, p := range paths {
		shardConf, err := util.ReadJsonFile(p, &types.PartitionDescriptionRecord{})
		if err != nil {
			return fmt.Errorf("failed to read shard conf from %q: %w", p, err)
		}
		if err := orchestration.AddShardConfig(shardConf); err != nil {
			return fmt.Errorf("failed to add shard conf from %q: %w", p, err)
		}
	}
	return nil
}

func getRoundInfoHandler(getState func() (*abdrc.StateMsg, error), obs Observability) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := getState()
		if err != nil {
			obs.Logger().Warn(fmt.Sprintf("GET roundInfo request: failed to load state: %v", err))
			http.Error(w, "failed to load state", http.StatusInternalServerError)
			return
		}

		partitionShards := make([]shardInfo, 0, len(state.CommittedHead.ShardInfo))
		for _, si := range state.CommittedHead.ShardInfo {
			partitionShards = append(partitionShards, shardInfo{
				PartitionID: si.Partition,
				ShardID:     si.Shard,
				RoundNumber: si.IR.RoundNumber,
				EpochNumber: si.IR.Epoch,
			})
		}
		response := roundInfoResponse{
			RoundNumber:     state.CommittedHead.Block.Round,
			EpochNumber:     state.CommittedHead.Block.Epoch,
			PartitionShards: partitionShards,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			obs.Logger().Warn(fmt.Sprintf("GET roundInfo request: failed to write response: %v", err))
		}
	}
}
