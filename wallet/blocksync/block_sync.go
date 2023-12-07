package blocksync

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/sync/errgroup"
)

type BlocksLoaderFunc func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error)
type BlockProcessorFunc func(context.Context, *types.Block) error

/*
Run loads blocks using "getBlocks" and processes them using "processor" until:
  - ctx is cancelled;
  - maxBlockNumber param is not zero and block with that number has been processed;
  - unrecoverable error is encountered.

Other parameters:
  - startingBlockNumber is the first block number to ask for (using getBlocks) must be > 0;
  - maxBlockNumber: when zero Run loads new blocks until ctx is cancelled, when not zero
    blocks are loaded until block with given number has been processed;
  - batchSize how big batches to use (used for getBlocks parameter);

Run returns non-nil error unless maxBlockNumber param is not zero and that block is
loaded and processed successfully.
*/
func Run(ctx context.Context, getBlocks BlocksLoaderFunc, startingBlockNumber, maxBlockNumber uint64, batchSize int, processor BlockProcessorFunc) error {
	if startingBlockNumber <= 0 {
		return fmt.Errorf("invalid sync condition: starting block number must be greater than zero, got %d", startingBlockNumber)
	}
	if batchSize <= 0 {
		return fmt.Errorf("invalid sync condition: batch size must be greater than zero, got %d", batchSize)
	}
	if maxBlockNumber != 0 {
		if maxBlockNumber < startingBlockNumber {
			return fmt.Errorf("invalid sync condition: starting block number %d is greater than max block number %d", startingBlockNumber, maxBlockNumber)
		}
		getBlocks = loadUntilBlockNumber(maxBlockNumber, getBlocks)
	}

	blocks := make(chan *types.Block, batchSize)
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(blocks)
		err := fetchBlocks(ctx, getBlocks, startingBlockNumber, blocks)
		if err != nil && errors.Is(err, errMaxBlockReached) {
			return nil
		}
		return err
	})

	g.Go(func() error {
		return processBlocks(ctx, blocks, processor)
	})

	return g.Wait()
}

func fetchBlocks(ctx context.Context, getBlocks BlocksLoaderFunc, blockNumber uint64, out chan<- *types.Block) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rsp, err := getBlocks(ctx, blockNumber, uint64(cap(out)))
		if err != nil {
			return fmt.Errorf("failed to fetch blocks [%d...]: %w", blockNumber, err)
		}
		for _, b := range rsp.Blocks {
			block := &types.Block{}
			if err := cbor.Unmarshal(b, block); err != nil {
				return fmt.Errorf("failed to unmarshal block: %w", err)
			}
			out <- block
		}
		blockNumber = rsp.BatchMaxBlockNumber + 1
		if rsp.MaxRoundNumber < blockNumber {
			// we have reached to the last block the source currently has - wait a bit before asking for more
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(rand.Int31n(500)+500) * time.Millisecond):
			}
		}
	}
}

func processBlocks(ctx context.Context, blocks <-chan *types.Block, processor BlockProcessorFunc) error {
	for b := range blocks {
		if err := processor(ctx, b); err != nil {
			return fmt.Errorf("failed to process block {%x : %d}: %w", b.SystemID(), b.GetRoundNumber(), err)
		}
	}
	return nil
}

func loadUntilBlockNumber(maxBN uint64, f BlocksLoaderFunc) BlocksLoaderFunc {
	return func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
		if blockNumber > maxBN {
			return nil, errMaxBlockReached
		}
		if blockNumber+batchSize > maxBN {
			batchSize = (maxBN - blockNumber) + 1
		}
		return f(ctx, blockNumber, batchSize)
	}
}

var errMaxBlockReached = fmt.Errorf("max block number has been reached")
