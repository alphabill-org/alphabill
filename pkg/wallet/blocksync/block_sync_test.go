package blocksync

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/types"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
)

func Test_Run(t *testing.T) {
	t.Parallel()

	t.Run("invalid starting block number", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- Run(ctx,
				func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
					return nil, nil
				},
				0,     // starting block, must be > 0
				0, 10, // max block and batch size
				func(ctx context.Context, b *block.Block) error { return nil })
		}()

		select {
		case err := <-done:
			if err == nil || err.Error() != `invalid sync condition: starting block number must be greater than zero, got 0` {
				t.Errorf("unexpected error returned by Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run didn't finish within timeout")
		}
	})

	t.Run("invalid batch size", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- Run(ctx,
				func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
					return nil, nil
				},
				1, 0, // starting block and max block
				0, // batch size, must be > 0
				func(ctx context.Context, b *block.Block) error { return nil })
		}()

		select {
		case err := <-done:
			if err == nil || err.Error() != `invalid sync condition: batch size must be greater than zero, got 0` {
				t.Errorf("unexpected error returned by Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run didn't finish within timeout")
		}
	})

	t.Run("max block smaller than starting block", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- Run(ctx,
				func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
					return nil, nil
				},
				10, 9, // starting block and max block
				10, // batch size, must be > 0
				func(ctx context.Context, b *block.Block) error { return nil })
		}()

		select {
		case err := <-done:
			if err == nil || err.Error() != `invalid sync condition: starting block number 10 is greater than max block number 9` {
				t.Errorf("unexpected error returned by Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run didn't finish within timeout")
		}
	})

	t.Run("block source returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		expErr := fmt.Errorf("failed to produce block")
		getBlocks := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			// on first call return a block, next call(s) return error
			if blockNumber == 1 {
				return &alphabill.GetBlocksResponse{
					Blocks: []*block.Block{
						{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber}}},
					},
					MaxBlockNumber:      blockNumber + batchSize,
					BatchMaxBlockNumber: blockNumber,
				}, nil
			}
			return nil, expErr
		}

		processor := func(ctx context.Context, b *block.Block) error {
			// just consume everithing
			return nil
		}

		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, getBlocks, 1, 0, 20, processor)
		}()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, expErr) {
				t.Errorf("unexpected error returned by Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run didn't finish within timeout")
		}
	})

	t.Run("processor returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		getBlocks := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			// must have sleep not to starve CPU so that context cancel can take effect
			time.Sleep(time.Millisecond)
			// ignore batchSize and send always single block
			return &alphabill.GetBlocksResponse{
				Blocks: []*block.Block{
					{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber}}},
				},
				MaxBlockNumber:      blockNumber + batchSize, //always signal there is one more batch
				BatchMaxBlockNumber: blockNumber,
			}, nil
		}

		expErr := fmt.Errorf("failed to process block")
		processor := func(ctx context.Context, b *block.Block) error {
			// consume first two blocks, then return error
			if b.UnicityCertificate.InputRecord.RoundNumber < 3 {
				return nil
			}
			return expErr
		}

		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, getBlocks, 1, 0, 20, processor)
		}()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, expErr) {
				t.Errorf("unexpected error returned by Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run didn't finish within timeout")
		}
	})

	t.Run("process blocks until ctx is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var callCnt uint64
		getBlocks := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			n := atomic.AddUint64(&callCnt, 1)
			if n == 4 {
				// there have been 3 iterations, do not generate new blocks
				// signal "no more blocks right now" by setting MaxBlockNumber == blockNumber
				return &alphabill.GetBlocksResponse{MaxBlockNumber: blockNumber, BatchMaxBlockNumber: blockNumber}, nil
			}
			// ignore batchSize and send number of blocks based on which iteration it is
			var b []*block.Block
			for i := 0; i < int(n); i++ {
				b = append(b, &block.Block{
					UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber + uint64(i)}},
				})
			}
			return &alphabill.GetBlocksResponse{
				Blocks:              b,
				MaxBlockNumber:      blockNumber + uint64(len(b)) + batchSize, // signal there is one more batch
				BatchMaxBlockNumber: blockNumber + uint64(len(b)),
			}, nil
		}

		var lastBN uint64
		processor := func(ctx context.Context, b *block.Block) error {
			cbn := atomic.LoadUint64(&lastBN)
			if cbn >= b.UnicityCertificate.InputRecord.RoundNumber {
				return fmt.Errorf("unexpected block order: last %d current %d", cbn, b.UnicityCertificate.InputRecord.RoundNumber)
			}
			atomic.StoreUint64(&lastBN, b.UnicityCertificate.InputRecord.RoundNumber)
			if cbn == 6 {
				// generator did 3 iterations generating 1+2+3 blocks, stop the test
				cancel()
			}
			return nil
		}

		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, getBlocks, 1, 0, 10, processor)
		}()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected error returned by Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run didn't finish within timeout")
		}
	})

	t.Run("process blocks until max block number", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		getBlocks := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			var b []*block.Block
			for i := 0; i < int(batchSize); i++ {
				b = append(b, &block.Block{
					UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber + uint64(i)}},
				})
			}
			return &alphabill.GetBlocksResponse{
				Blocks:              b,
				MaxBlockNumber:      blockNumber + uint64(len(b)) + batchSize, // signal there is one more batch
				BatchMaxBlockNumber: blockNumber + uint64(len(b)),
			}, nil
		}

		var lastBN uint64
		processor := func(ctx context.Context, b *block.Block) error {
			cbn := atomic.LoadUint64(&lastBN)
			if cbn >= b.UnicityCertificate.InputRecord.RoundNumber {
				return fmt.Errorf("unexpected block order: last %d current %d", cbn, b.UnicityCertificate.InputRecord.RoundNumber)
			}
			atomic.StoreUint64(&lastBN, b.UnicityCertificate.InputRecord.RoundNumber)
			// to give context cancellations better chanche to propagate
			time.Sleep(5 * time.Millisecond)
			return nil
		}

		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, getBlocks, 1, 5, 3, processor)
		}()

		select {
		case err := <-done:
			// when fetching until max block Run is expected to return nil when reaching to it
			if err != nil {
				t.Errorf("unexpected error returned by Run: %v", err)
			}
			if lbn := atomic.LoadUint64(&lastBN); lbn != 5 {
				t.Errorf("expected last block processed to be 5, got %d", lbn)
			}
		case <-time.After(time.Second):
			t.Error("Run didn't finish within timeout")
		}
	})
}

func Test_fetchBlocks(t *testing.T) {
	t.Parallel()

	t.Run("cancelling context stops the func", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		blocks := make(chan *block.Block)
		done := make(chan error, 1)
		go func() {
			done <- fetchBlocks(ctx, func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
				return nil, nil
			}, 1, blocks)
		}()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected error returned by fetchBlocks: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("fetchBlocks didn't finish within timeout")
		}
	})

	t.Run("getBlocks callback returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		expErr := fmt.Errorf("getBlocks failed")
		getBlocks := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			return nil, expErr
		}

		blocks := make(chan *block.Block)
		done := make(chan error, 1)
		go func() {
			done <- fetchBlocks(ctx, getBlocks, 1, blocks)
		}()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, expErr) {
				t.Errorf("unexpected error returned by fetchBlocks: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("fetchBlocks didn't finish within timeout")
		}
	})

	t.Run("no new blocks from the source", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var callCnt int32
		getBlocks := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			if blockNumber != 4 {
				return nil, fmt.Errorf("expected that block 4 is requested, got %d", blockNumber)
			}
			if atomic.AddInt32(&callCnt, 1) == 3 {
				// there have been 3 iterations, cancel the ctx, this should stop the test
				cancel()
			}
			// always respond that the latest block is 3
			return &alphabill.GetBlocksResponse{MaxBlockNumber: 3, BatchMaxBlockNumber: 3}, nil
		}

		blocks := make(chan *block.Block)
		done := make(chan error, 1)
		go func() {
			done <- fetchBlocks(ctx, getBlocks, 4, blocks)
		}()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected error returned by fetchBlocks: %v", err)
			}
		case <-time.After(2500 * time.Millisecond):
			// we're supposed to exit on third iteration, one (full) iteration could sleep up to 1s so 2.5s should do
			t.Error("fetchBlocks didn't finish within timeout")
		}
	})

	t.Run("next blocks are fetched from source", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var callCnt uint64
		getBlocks := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			n := atomic.AddUint64(&callCnt, 1)
			if n == 3 {
				// there have been 3 iterations, cancel the ctx, this should stop the test
				cancel()
			}
			if blockNumber != n {
				return nil, fmt.Errorf("expected that block %d is requested, got %d", n, blockNumber)
			}
			// ignore batchSize and send only one block
			return &alphabill.GetBlocksResponse{
				Blocks: []*block.Block{
					{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber}}},
				},
				MaxBlockNumber:      4,
				BatchMaxBlockNumber: blockNumber,
			}, nil
		}

		blocks := make(chan *block.Block, 4) // buf must be big enough that all blocks can be sent!
		done := make(chan error, 1)
		go func() {
			done <- fetchBlocks(ctx, getBlocks, 1, blocks)
		}()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected error returned by fetchBlocks: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			// we report that there is 4 blocks but return then one-by-one
			// so expect no delays between getBlocks calls
			t.Error("fetchBlocks didn't finish within timeout")
		}
	})
}

func Test_processBlocks(t *testing.T) {
	t.Parallel()

	t.Run("closing data chan returns nil", func(t *testing.T) {
		blocks := make(chan *block.Block)
		done := make(chan error, 1)
		go func() {
			done <- processBlocks(context.Background(), blocks, func(ctx context.Context, b *block.Block) error { return nil })
		}()

		close(blocks)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("unexpected error returned by processBlocks: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("processBlocks didn't finish within timeout")
		}
	})

	t.Run("sending to chan calls processBlocks", func(t *testing.T) {
		expErr := fmt.Errorf("failed to process block")
		blocks := make(chan *block.Block)
		done := make(chan error, 1)
		go func() {
			done <- processBlocks(context.Background(), blocks, func(ctx context.Context, b *block.Block) error { return expErr })
		}()

		go func() { blocks <- &block.Block{} }()

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, expErr) {
				t.Errorf("unexpected error returned by processBlocks: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("processBlocks didn't finish within timeout")
		}
	})
}

func Test_loadUntilBlockNumber(t *testing.T) {
	t.Parallel()

	t.Run("invalid input: blockNumber > maxBN", func(t *testing.T) {
		loader := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
			err := fmt.Errorf("unexpected call (%d, %d)", blockNumber, batchSize)
			t.Error(err.Error())
			return nil, err
		}
		f := loadUntilBlockNumber(3, loader)
		abr, err := f(context.Background(), 4, 100)
		if err == nil {
			t.Error("expected non-nil error")
		} else if !errors.Is(err, errMaxBlockReached) {
			t.Errorf("unexpected error: %v", err)
		}
		if abr != nil {
			t.Errorf("unexpectedly got non-nil response: %v", abr)
		}
	})

	t.Run("correct block number and batch size is sent to the loader", func(t *testing.T) {
		cases := []struct {
			blockNo   uint64 // input: block number
			batchSize uint64 // input: batch size to load
			maxBNo    uint64 // input for the limiter: max block number to load
			queryBS   uint64 // batch size we expect to see in the loader
		}{
			// increase starting block number while max block and batch size stay
			// the same - the batch size that the query gets should decrease so that
			// loader never asks for a block past the maximum block
			{blockNo: 1, batchSize: 10, maxBNo: 10, queryBS: 10},
			{blockNo: 2, batchSize: 10, maxBNo: 10, queryBS: 9},
			{blockNo: 5, batchSize: 10, maxBNo: 10, queryBS: 6},
			{blockNo: 10, batchSize: 10, maxBNo: 10, queryBS: 1},
			// ask for the max block with increasing batch size - query should ask for one block only anyway
			{blockNo: 10, batchSize: 1, maxBNo: 10, queryBS: 1},
			{blockNo: 10, batchSize: 2, maxBNo: 10, queryBS: 1},
			{blockNo: 10, batchSize: 10, maxBNo: 10, queryBS: 1},
			// when max block is bigger than the starting block + batch size query should always ask for batch size blocks
			{blockNo: 10, batchSize: 1, maxBNo: 70, queryBS: 1},
			{blockNo: 11, batchSize: 10, maxBNo: 70, queryBS: 10},
			{blockNo: 12, batchSize: 18, maxBNo: 70, queryBS: 18},
			{blockNo: 30, batchSize: 40, maxBNo: 70, queryBS: 40},
		}

		for n, tc := range cases {
			loader := func(ctx context.Context, blockNumber, batchSize uint64) (*alphabill.GetBlocksResponse, error) {
				if blockNumber != tc.blockNo || batchSize != tc.queryBS {
					return nil, fmt.Errorf("unexpected parameters block(%d vs %d) batch(%d vs %d)", blockNumber, tc.blockNo, batchSize, tc.queryBS)
				}
				return nil, nil
			}
			f := loadUntilBlockNumber(tc.maxBNo, loader)
			_, err := f(context.Background(), tc.blockNo, tc.batchSize)
			if err != nil {
				t.Errorf("test case [%d] unexpected error: %v", n, err)
			}
		}
	})
}
