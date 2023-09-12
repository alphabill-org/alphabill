package wallet

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

const (
	maxTxFailedTries = 3
)

type (
	// Shutdown needs to be called to release resources used by wallet.
	Wallet struct {
		AlphabillClient client.ABClient
	}

	Builder struct {
		abcConf client.AlphabillClientConfig
		// overrides abcConf
		abc client.ABClient
	}

	SendOpts struct {
		// RetryOnFullTxBuffer retries to send transaction when tx buffer is full
		RetryOnFullTxBuffer bool
	}
)

func New() *Builder {
	return &Builder{}
}

func (b *Builder) SetABClientConf(abcConf client.AlphabillClientConfig) *Builder {
	b.abcConf = abcConf
	return b
}

func (b *Builder) SetABClient(abc client.ABClient) *Builder {
	b.abc = abc
	return b
}

func (b *Builder) Build() *Wallet {
	return &Wallet{
		AlphabillClient: b.getOrCreateABClient(),
	}
}

func (b *Builder) getOrCreateABClient() client.ABClient {
	if b.abc != nil {
		return b.abc
	}
	return client.New(b.abcConf)
}

// GetRoundNumber queries the node for latest round number
func (w *Wallet) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.AlphabillClient.GetRoundNumber(ctx)
}

// SendTransaction broadcasts transaction to configured node.
// Returns nil if transaction was successfully accepted by node, otherwise returns error.
func (w *Wallet) SendTransaction(ctx context.Context, tx *types.TransactionOrder, opts *SendOpts) error {
	if opts == nil || !opts.RetryOnFullTxBuffer {
		return w.AlphabillClient.SendTransactionWithRetry(ctx, tx, 1)
	}
	return w.AlphabillClient.SendTransactionWithRetry(ctx, tx, maxTxFailedTries)
}

// Shutdown terminates connection to alphabill node and cancels any background goroutines.
func (w *Wallet) Shutdown() {
	log.Debug("shutting down wallet")

	if w.AlphabillClient != nil {
		err := w.AlphabillClient.Close()
		if err != nil {
			log.Error("error shutting down wallet: ", err)
		}
	}
}
