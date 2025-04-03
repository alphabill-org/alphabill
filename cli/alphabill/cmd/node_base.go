package cmd

import (
	"net/http"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/rpc"
)

type (
	p2pFlags struct {
		Address            string
		AnnounceAddresses  []string
		BootstrapAddresses []string // bootstrap addresses (libp2p multiaddress format)
	}

	rpcFlags struct {
		rpc.ServerConfiguration
		StateRpcRateLimit int
	}
)

func (f *p2pFlags) addP2PFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.Address, "address", "a", "/ip4/127.0.0.1/tcp/26652", "listen address for p2p connections (libp2p multiaddress format)")
	cmd.Flags().StringSliceVarP(&f.AnnounceAddresses, "announce-addresses", "", nil, "announced listen addresses (libp2p multiaddress format)")
	cmd.Flags().StringSliceVar(&f.BootstrapAddresses, "bootnodes", nil, "addresses of bootstrap nodes (libp2p multiaddress format)")
}

func (f *rpcFlags) addRPCFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.Address, "rpc-server-address", "",
		"Specifies the TCP address for the RPC server to listen on, in the form \"host:port\". RPC server isn't initialised if Address is empty. (default \"\")")
	cmd.Flags().DurationVar(&f.ReadTimeout, "rpc-server-read-timeout", 0,
		"The maximum duration for reading the entire request, including the body. A zero or negative value means there will be no timeout. (default 0)")
	cmd.Flags().DurationVar(&f.ReadHeaderTimeout, "rpc-server-read-header-timeout", 0,
		"The amount of time allowed to read request headers. If rpc-server-read-header-timeout is zero, the value of rpc-server-read-timeout is used. If both are zero, there is no timeout. (default 0)")
	cmd.Flags().DurationVar(&f.WriteTimeout, "rpc-server-write-timeout", 0,
		"The maximum duration before timing out writes of the response. A zero or negative value means there will be no timeout. (default 0)")
	cmd.Flags().DurationVar(&f.IdleTimeout, "rpc-server-idle-timeout", 0,
		"The maximum amount of time to wait for the next request when keep-alives are enabled. If rpc-server-idle-timeout is zero, the value of rpc-server-read-timeout is used. If both are zero, there is no timeout. (default 0)")
	cmd.Flags().IntVar(&f.MaxHeaderBytes, "rpc-server-max-header", http.DefaultMaxHeaderBytes,
		"Controls the maximum number of bytes the server will read parsing the request header's keys and values, including the request line.")
	cmd.Flags().Int64Var(&f.MaxBodyBytes, "rpc-server-max-body", rpc.DefaultMaxBodyBytes,
		"Controls the maximum number of bytes the server will read parsing the request body.")
	cmd.Flags().IntVar(&f.BatchItemLimit, "rpc-server-batch-item-limit", rpc.DefaultBatchItemLimit,
		"The maximum number of requests in a batch.")
	cmd.Flags().IntVar(&f.BatchResponseSizeLimit, "rpc-server-batch-response-size-limit", rpc.DefaultBatchResponseSizeLimit,
		"The maximum number of response bytes across all requests in a batch.")
	cmd.Flags().IntVar(&f.StateRpcRateLimit, "state-rpc-rate-limit", 20, "number of costliest state rpc requests allowed in a second")

	hideFlags(cmd,
		"rpc-server-read-timeout",
		"rpc-server-read-header-timeout",
		"rpc-server-write-timeout",
		"rpc-server-idle-timeout",
		"rpc-server-max-header",
		"rpc-server-max-body",
		"rpc-server-batch-item-limit",
		"rpc-server-batch-response-size-limit",
	)
}

func hideFlags(cmd *cobra.Command, flags ...string) {
	for _, flag := range flags {
		if err := cmd.Flags().MarkHidden(flag); err != nil {
			panic(err)
		}
	}
}
