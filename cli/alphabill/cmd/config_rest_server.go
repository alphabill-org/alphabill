package cmd

import (
	"net/http"
	"strings"
	"time"

	"github.com/alphabill-org/alphabill/validator/pkg/rpc"
	"github.com/spf13/cobra"
)

const (
	MaxBodyBytes int64 = 4194304 // 4MB
)

type (
	// restServerConfiguration is a common configuration for REST servers.
	restServerConfiguration struct {
		// Address specifies the TCP address for the server to listen on, in the form "host:port".
		// REST server isn't initialised if Address is empty.
		Address string

		// ReadTimeout is the maximum duration for reading the entire request, including the body. A zero or negative
		// value means there will be no timeout.
		ReadTimeout time.Duration

		// ReadHeaderTimeout is the amount of time allowed to read request headers. If ReadHeaderTimeout is zero, the
		// value of ReadTimeout is used. If both are zero, there is no timeout.
		ReadHeaderTimeout time.Duration

		// WriteTimeout is the maximum duration before timing out writes of the response. A zero or negative value means
		// there will be no timeout.
		WriteTimeout time.Duration

		// IdleTimeout is the maximum amount of time to wait for the next request when keep-alives are enabled. If
		// IdleTimeout is zero, the value of ReadTimeout is used. If both are zero, there is no timeout.
		IdleTimeout time.Duration

		// MaxHeaderBytes controls the maximum number of bytes the server will read parsing the request header's keys
		// and values, including the request line. It does not limit the size of the request body. If zero,
		// http.DefaultMaxHeaderBytes is used.
		MaxHeaderBytes int

		// MaxHeaderBytes controls the maximum number of bytes the server will read parsing the request body. If zero,
		// MaxBodyBytes is used.
		MaxBodyBytes int64

		router rpc.Registrar
	}
)

func (c *restServerConfiguration) IsAddressEmpty() bool {
	return strings.TrimSpace(c.Address) == ""
}

func (c *restServerConfiguration) addConfigurationFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&c.Address, "rest-server-address", "", "Specifies the TCP address for the REST server to listen on, in the form \"host:port\". REST server isn't initialised if Address is empty. (default \"\")")
	cmd.Flags().DurationVar(&c.ReadTimeout, "rest-server-read-timeout", 0, "The maximum duration for reading the entire request, including the body. A zero or negative value means there will be no timeout. (default 0)")
	cmd.Flags().DurationVar(&c.ReadHeaderTimeout, "rest-server-read-header-timeout", 0, "The amount of time allowed to read request headers. If rest-server-read-header-timeout is zero, the value of rest-server-read-timeout is used. If both are zero, there is no timeout. (default 0)")
	cmd.Flags().DurationVar(&c.WriteTimeout, "rest-server-write-timeout", 0, "The maximum duration before timing out writes of the response. A zero or negative value means there will be no timeout. (default 0)")
	cmd.Flags().DurationVar(&c.IdleTimeout, "rest-server-idle-timeout", 0, "The maximum amount of time to wait for the next request when keep-alives are enabled. If rest-server-idle-timeout is zero, the value of rest-server-read-timeout is used. If both are zero, there is no timeout. (default 0)")
	cmd.Flags().IntVar(&c.MaxHeaderBytes, "rest-server-max-header", http.DefaultMaxHeaderBytes, "Controls the maximum number of bytes the server will read parsing the request header's keys and values, including the request line.")
	cmd.Flags().Int64Var(&c.MaxBodyBytes, "rest-server-max-body", MaxBodyBytes, "Controls the maximum number of bytes the server will read parsing the request body.")
}
