package wallet

import "gitdc.ee.guardtime.com/alphabill/alphabill/pkg/client"

// Config configuration options for creating and loading a wallet.
type Config struct {
	// Configuration options for connecting to alphabill nodes.
	AlphabillClientConfig client.AlphabillClientConfig
}
