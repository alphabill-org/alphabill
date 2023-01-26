// package twb implements token wallet backend
package twb

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/pkg/client"
	wtokens "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
)

type Storage interface {
	Close() error
	GetBlockNumber() (uint64, error)
	SetBlockNumber(blockNumber uint64) error

	SaveTokenType(data *wtokens.TokenUnitType) error
	GetTokenType(id []byte) (*wtokens.TokenUnitType, error)

	SaveTokenUnit(data *wtokens.TokenUnit) error
}

type Configuration interface {
	Client() client.ABClient
	Storage() (Storage, error)
	BatchSize() int
}

/*
Run starts the token wallet backend - syncing blocks to local storage and
launching HTTP server to query it.
Run blocks until ctx is cancelled or some unrecoverable error happens, it
always returns non-nil error.
*/
func Run(ctx context.Context, cfg Configuration) error {
	db, err := cfg.Storage()
	if err != nil {
		return fmt.Errorf("failed to get storage: %w", err)
	}
	defer db.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return syncBlocks(ctx, db, cfg.Client(), cfg.BatchSize())
	})

	g.Go(func() error {
		// TODO: launch http server for REST API
		<-ctx.Done()
		return ctx.Err()
	})

	return g.Wait()
}

type cfg struct {
	abc    client.AlphabillClientConfig
	boltDB string
}

/*
abURL - AlphaBill backend from where to sync blocks.
boltDB - filename (with full path) of the bolt db to use as storage
*/
func NewConfig(abURL, boltDB string) Configuration {
	return &cfg{abc: client.AlphabillClientConfig{Uri: abURL}}
}

func (c *cfg) Client() client.ABClient   { return client.New(c.abc) }
func (c *cfg) Storage() (Storage, error) { return newBoltStore(c.boltDB) }
func (c *cfg) BatchSize() int            { return 100 }
