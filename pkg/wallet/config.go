package wallet

// Config configuration options for creating and loading a wallet.
type Config struct {
	WalletPass string

	// Configuration options for connecting to alphabill nodes.
	AlphabillClientConfig AlphabillClientConfig
}

// AlphabillClientConfig configuration options for connecting to alphabill nodes.
type AlphabillClientConfig struct {
	Uri string

	// RequestTimeoutMs timeout for RPC request, if not set then requests never expire
	RequestTimeoutMs uint64
}
