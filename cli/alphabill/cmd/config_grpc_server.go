package cmd

import (
	"time"

	"github.com/spf13/cobra"
	grpckeepalive "google.golang.org/grpc/keepalive"
)

type (
	// grpcServerConfiguration is a common configuration for gRPC servers.
	grpcServerConfiguration struct {
		// Listen address together with port.
		Address string `validate:"empty=false"`

		// Maximum number of bytes the incoming message may be.
		MaxRecvMsgSize int `validate:"gte=0" default:"256000"`

		// MaxConnectionAgeMs is a duration for the maximum amount of time a
		// connection may exist before it will be closed by sending a GoAway. A
		// random jitter of +/-10% will be added to MaxConnectionAgeMs to spread out
		// connection storms.
		MaxConnectionAgeMs int64

		// MaxConnectionAgeGraceMs is an additive period after MaxConnectionAgeMs after
		// which the connection will be forcibly closed.
		MaxConnectionAgeGraceMs int64
	}
)

const (
	defaultServerAddr     = ":9543"
	defaultMaxRecvMsgSize = 256000
)

func (c *grpcServerConfiguration) addConfigurationFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&c.Address, "server-address", defaultServerAddr, "The gRPC server listen address with port.")
	cmd.Flags().IntVar(&c.MaxRecvMsgSize, "server-max-recv-msg-size", defaultMaxRecvMsgSize, "Maximum number of bytes the incoming message may be.")
	cmd.Flags().Int64Var(&c.MaxConnectionAgeMs, "server-max-connection-age-ms", 0, "a duration for the maximum amount of time a connection may exist before it will be closed by sending a GoAway in milliseconds. 0 means forever.")
	cmd.Flags().Int64Var(&c.MaxConnectionAgeGraceMs, "server-max-connection-age-grace-ms", 0, "is an additive period after MaxConnectionAgeMs after which the connection will be forcibly closed in milliseconds. 0 means no grace period.")
	err := cmd.Flags().MarkHidden("server-max-recv-msg-size")
	if err != nil {
		panic(err)
	}
	err = cmd.Flags().MarkHidden("server-max-connection-age-ms")
	if err != nil {
		panic(err)
	}
	err = cmd.Flags().MarkHidden("server-max-connection-age-grace-ms")
	if err != nil {
		panic(err)
	}
}

func (c *grpcServerConfiguration) GrpcKeepAliveServerParameters() grpckeepalive.ServerParameters {
	p := grpckeepalive.ServerParameters{}
	if c.MaxConnectionAgeMs != 0 {
		p.MaxConnectionAge = time.Duration(c.MaxConnectionAgeMs) * time.Millisecond
		log.Debug("Server grpc client connection max connection age: %dms", c.MaxConnectionAgeMs)
	}
	if c.MaxConnectionAgeGraceMs != 0 {
		p.MaxConnectionAgeGrace = time.Duration(c.MaxConnectionAgeGraceMs) * time.Millisecond
		log.Debug("Server grpc client connection max connection age grace: %dms", c.MaxConnectionAgeGraceMs)
	}
	return p
}
