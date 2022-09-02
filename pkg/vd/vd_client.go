package verifiable_data

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/pkg/errors"
)

type (
	VDClient struct {
		abClient client.ABClient
		wallet   *wallet.Wallet
		// synchronizes with ledger until the block is found where tx has been added to
		syncToBlock   bool
		timeoutDelta  uint64
		ctx           context.Context
		blockCallback func(b *VDBlock)
	}

	VDClientConfig struct {
		// AbConf configuration parameters of Alphabill client
		AbConf *client.AlphabillClientConfig
		// WaitBlock upon hash submission, waits for a block to contain a transaction with the given hash, stops when found or timeout is reached
		WaitBlock bool
		// BlockTimeout relative timeout of a transaction (e.g. if latest block # is 100, given block timeout is 50, transaction's timeout is 150)
		BlockTimeout uint64
		// OnBlockCallback is called if WaitBlock is set to true and block containing a transaction with the given hash is found
		OnBlockCallback func(b *VDBlock)
	}

	VDBlock struct {
		blockNumber uint64
	}
)

func (block *VDBlock) GetBlockNumber() uint64 {
	return block.blockNumber
}

func New(ctx context.Context, conf *VDClientConfig) (*VDClient, error) {
	return &VDClient{
		ctx:           ctx,
		abClient:      client.New(*conf.AbConf),
		syncToBlock:   conf.WaitBlock,
		timeoutDelta:  conf.BlockTimeout,
		blockCallback: conf.OnBlockCallback,
	}, nil
}

func (v *VDClient) RegisterFileHash(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open the file %s", filePath)
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return errors.Wrapf(err, "failed to read the file %s", filePath)
	}

	hash := hasher.Sum(nil)
	log.Debug("Hash of file '", filePath, "': ", hash)
	return v.registerHashTx(hash)
}

func (v *VDClient) RegisterHashBytes(bytes []byte) error {
	return v.registerHashTx(bytes)
}

func (v *VDClient) RegisterHash(hash string) error {
	b, err := hexStringToBytes(hash)
	if err != nil {
		return err
	}
	return v.registerHashTx(b)
}

// FetchBlockWithHash is a temporary workaround for verifying registered hash values.
func (v *VDClient) FetchBlockWithHash(hash []byte, blockNumber uint64) error {
	return v.sync(blockNumber-1, blockNumber, hash)
}

// ListAllBlocksWithTx prints all non-empty blocks from genesis up to the latest block
func (v *VDClient) ListAllBlocksWithTx() error {
	defer v.shutdown()
	log.Info("Fetching blocks...")
	maxBlockNumber, err := v.abClient.GetMaxBlockNumber()
	if err != nil {
		return err
	}
	log.Debug("Max block: #", maxBlockNumber)
	if err := v.sync(0, maxBlockNumber, nil); err != nil {
		return err
	}

	log.Info("Done.")
	return nil
}

func (v *VDClient) registerHashTx(hash []byte) error {
	defer v.shutdown()

	if err := validateHash(hash); err != nil {
		return err
	}

	currentBlockNumber, err := v.abClient.GetMaxBlockNumber()
	if err != nil {
		return err
	}

	log.Info("Current block #: ", currentBlockNumber)
	timeout := currentBlockNumber + v.timeoutDelta
	tx, err := createRegisterDataTx(hash, timeout)
	if err != nil {
		return err
	}
	resp, err := v.abClient.SendTransaction(tx)
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return errors.New(fmt.Sprintf("error while submitting the hash: %s", resp.GetMessage()))
	}
	log.Info("Hash successfully submitted, timeout block: ", timeout)

	if v.syncToBlock {
		return v.sync(currentBlockNumber, timeout, hash)
	}
	return nil
}

func (v *VDClient) sync(currentBlock uint64, timeout uint64, hash []byte) error {
	v.wallet = wallet.New().
		SetBlockProcessor(v.prepareProcessor(timeout, hash)).
		SetABClient(v.abClient).
		Build()
	return v.wallet.Sync(v.ctx, currentBlock)
}

type VDBlockProcessor func(b *block.Block) error

func (p VDBlockProcessor) ProcessBlock(b *block.Block) error {
	return p(b)
}

func (v *VDClient) prepareProcessor(timeout uint64, hash []byte) VDBlockProcessor {
	return func(b *block.Block) error {
		log.Debug("Fetched block #", b.GetBlockNumber(), ", tx count: ", len(b.GetTransactions()))
		if b.GetBlockNumber() > timeout {
			log.Info("Block timeout reached")
			v.shutdown()
			return nil
		}
		for _, tx := range b.GetTransactions() {
			log.Debug("Processing block #", b.GetBlockNumber())
			if hash != nil {
				// if hash is provided, print only the corresponding block
				if bytes.Equal(hash, tx.GetUnitId()) {
					log.Info(fmt.Sprintf("Tx in block #%d, hash: %s", b.GetBlockNumber(), hex.EncodeToString(hash)))
					if v.blockCallback != nil {
						log.Info("Invoking block callback")
						v.blockCallback(&VDBlock{blockNumber: b.BlockNumber})
					}
					v.shutdown()
					break
				} else if hex.EncodeToString(hash) == hex.EncodeToString(tx.GetUnitId()) {
					log.Info("WTF?")
				}
			} else {
				log.Info(fmt.Sprintf("Tx in block #%d, hash: %s", b.GetBlockNumber(), hex.EncodeToString(tx.GetUnitId())))
			}
		}
		return nil
	}
}

func (v *VDClient) shutdown() {
	log.Info("Shutting down")
	if v.wallet != nil {
		v.wallet.Shutdown()
	}
	err := v.abClient.Shutdown()
	if err != nil {
		log.Error(err)
	}
}

func (v *VDClient) IsShutdown() bool {
	if v.abClient != nil {
		return v.abClient.IsShutdown()
	}
	return true
}

func validateHash(hash []byte) error {
	if len(hash) != sha256.Size {
		return errors.New(fmt.Sprintf("invalid hash length, expected %d bytes, got %d", sha256.Size, len(hash)))
	}
	return nil
}

func createRegisterDataTx(hash []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:   hash,
		SystemId: []byte{0, 0, 0, 1},
		Timeout:  timeout,
	}
	return tx, nil
}

func hexStringToBytes(hexString string) ([]byte, error) {
	bs, err := hex.DecodeString(strings.TrimPrefix(hexString, "0x"))
	if err != nil {
		return nil, err
	}
	return bs, nil
}
