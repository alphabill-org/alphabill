package money

import (
	"os"
	"path"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
)

type WalletConfig struct {
	// Directory where default boltdb wallet database is created, only used when Db is not set,
	// if empty then 'home/.alphabill/wallet' directory is used.
	DbPath string

	// Custom database implementation, if set then DbPath is not used,
	// if not set then boltdb is created at DbPath.
	Db Db

	// WalletPass used to encrypt/decrypt sensitive information. If empty then wallet will not be encrypted.
	WalletPass string

	// TrustBase contains a path to the trust base file.
	TrustBaseFile string

	// TrustBase contains a map of root chain validator public keys.
	trustBase map[string]crypto.Verifier

	// Configuration options for connecting to alphabill nodes.
	AlphabillClientConfig client.AlphabillClientConfig
}

// GetWalletDir returns wallet directory,
// if DbPath is set then returns DbPath,
// if DbPath is not set then returns 'home/.alphabill/wallet'.
func (c *WalletConfig) GetWalletDir() (string, error) {
	if c.DbPath != "" {
		return c.DbPath, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return path.Join(homeDir, ".alphabill", "wallet"), nil
}

func (c *WalletConfig) GetTrustBase() (map[string]crypto.Verifier, error) {
	if c.trustBase != nil {
		return c.trustBase, nil
	}
	f := c.TrustBaseFile
	if !path.IsAbs(c.TrustBaseFile) {
		dir, err := c.GetWalletDir()
		if err != nil {
			return nil, err
		}
		f = path.Join(dir, c.TrustBaseFile)
		if c.TrustBaseFile == "" {
			f = path.Join(f, "trust-base.json")
		}
	}
	tb, err := wallet.ReadTrustBaseFile(f)
	if err != nil {
		return nil, err
	}
	c.trustBase = tb
	return c.trustBase, nil
}
