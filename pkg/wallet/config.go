package wallet

import "github.com/alphabill-org/alphabill/pkg/client"

// Config configuration options for creating and loading a wallet.
type Config struct {
	// Configuration options for connecting to alphabill nodes.
	AlphabillClientConfig client.AlphabillClientConfig
}
