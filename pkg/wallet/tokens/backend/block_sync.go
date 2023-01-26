package twb

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/client"
)

/*
syncBlocks loads blocks from backend to local storage.
*/
func syncBlocks(ctx context.Context, db Storage, abc client.ABClient, batchSize int) error {
	blocks := make(chan *block.Block, batchSize)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(blocks)
		blockNumber, err := db.GetBlockNumber()
		if err != nil {
			return fmt.Errorf("failed to read current block number for a sync starting point: %w", err)
		}
		// on bootstrap storage returns 0 as current block and as block
		// numbering starts from 1 we start with first block.
		return fetchBlocks(ctx, abc, blockNumber+1, blocks)
	})

	g.Go(func() error {
		txs, err := tokens.New()
		if err != nil {
			return fmt.Errorf("failed to create token tx system: %w", err)
		}
		bp := &blockProcessor{store: db, txs: txs}
		return processBlocks(ctx, blocks, bp.ProcessBlock)
	})

	return g.Wait()
}

func fetchBlocks(ctx context.Context, abc client.ABClient, blockNumber uint64, out chan<- *block.Block) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rsp, err := abc.GetBlocks(blockNumber, uint64(cap(out)))
		if err != nil {
			return fmt.Errorf("failed to fetch blocks [%d...]: %w", blockNumber, err)
		}
		for _, b := range rsp.Blocks {
			out <- b
		}
		if n := len(rsp.Blocks); n > 0 {
			blockNumber = rsp.Blocks[n-1].BlockNumber + 1
		}
		if rsp.MaxBlockNumber <= blockNumber {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(rand.Int31n(1000)) * time.Millisecond):
			}
		}
	}
}

func processBlocks(ctx context.Context, blocks <-chan *block.Block, processor func(context.Context, *block.Block) error) error {
	for b := range blocks {
		if err := processor(ctx, b); err != nil {
			return fmt.Errorf("failed to procces block {%x : %d}: %w", b.SystemIdentifier, b.BlockNumber, err)
		}
	}
	return nil
}
