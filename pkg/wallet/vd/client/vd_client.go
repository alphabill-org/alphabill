package client

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
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type (
	VDClient struct {
		vdNodeClient    client.ABClient
		moneyNodeClient client.ABClient

		// synchronizes with ledger until the block is found where tx has been added to
		syncToBlock   bool
		timeoutDelta  uint64
		blockCallback func(b *VDBlock)
	}

	VDClientConfig struct {
		// VDNodeURL URL of the vd node to connect
		VDNodeURL string
		// MoneyNodeURL URL of the money node to connect
		MoneyNodeURL string
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

func New(conf *VDClientConfig) (*VDClient, error) {
	moneyNodeClient := client.New(client.AlphabillClientConfig{
		Uri: conf.MoneyNodeURL,
	})
	vdNodeClient := client.New(client.AlphabillClientConfig{
		Uri: conf.VDNodeURL,
	})
	return &VDClient{
		moneyNodeClient: moneyNodeClient,
		vdNodeClient:    vdNodeClient,
		syncToBlock:     conf.WaitBlock,
		timeoutDelta:    conf.BlockTimeout,
		blockCallback:   conf.OnBlockCallback,
	}, nil
}

func (v *VDClient) SystemID() []byte {
	// TODO: return the default "AlphaBill VD System ID" for now
	// but this should come from config (base wallet? AB client?)
	return vd.DefaultSystemIdentifier
}

func (v *VDClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	return v.vdNodeClient.GetRoundNumber(ctx)
}

func (v *VDClient) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
	for _, tx := range txs.Transactions {
		err := v.vdNodeClient.SendTransaction(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to send transaction: %w", err)
		}
	}
	return nil
}

func (v *VDClient) GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	// TODO
	return nil, nil;
}

func (v *VDClient) FetchFeeCreditBill(ctx context.Context, unitID []byte) (*bp.Bill, error) {
	// TODO
	return nil, nil
}

func (v *VDClient) RegisterFileHash(ctx context.Context, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open the file %q: %w", filePath, err)
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to read the file %q: %w", filePath, err)
	}

	hash := hasher.Sum(nil)
	log.Debug("Hash of file '", filePath, "': ", hash)
	return v.registerHashTx(ctx, hash)
}

func (v *VDClient) RegisterHashBytes(ctx context.Context, bytes []byte) error {
	return v.registerHashTx(ctx, bytes)
}

func (v *VDClient) RegisterHash(ctx context.Context, hash string) error {
	b, err := hexStringToBytes(hash)
	if err != nil {
		return err
	}
	return v.registerHashTx(ctx, b)
}

// FetchBlockWithHash is a temporary workaround for verifying registered hash values.
func (v *VDClient) FetchBlockWithHash(ctx context.Context, hash []byte, blockNumber uint64) error {
	return v.sync(ctx, blockNumber-1, blockNumber, hash)
}

// ListAllBlocksWithTx prints all non-empty blocks from genesis up to the latest block
func (v *VDClient) ListAllBlocksWithTx(ctx context.Context) error {
	log.Info("Fetching blocks...")
	maxBlockNumber, err := v.vdNodeClient.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	log.Debug("Max block: #", maxBlockNumber)
	if err := v.sync(ctx, 0, maxBlockNumber, nil); err != nil {
		return err
	}

	log.Info("Done.")
	return nil
}

func (v *VDClient) registerHashTx(ctx context.Context, hash []byte) error {
	if err := validateHash(hash); err != nil {
		return err
	}

	currentRoundNumber, err := v.vdNodeClient.GetRoundNumber(ctx)
	if err != nil {
		return err
	}

	log.Info("Current block #: ", currentRoundNumber)
	timeout := currentRoundNumber + v.timeoutDelta
	tx, err := createRegisterDataTx(v.SystemID(), hash, timeout)
	if err != nil {
		return err
	}

	if err := v.vdNodeClient.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("error while submitting the hash: %w", err)
	}
	log.Info("Hash successfully submitted, timeout block: ", timeout)

	if v.syncToBlock {
		return v.sync(ctx, currentRoundNumber, timeout, hash)
	}
	return nil
}

func (v *VDClient) sync(ctx context.Context, currentBlock uint64, timeout uint64, hash []byte) error {
	wallet := wallet.New().SetABClient(v.vdNodeClient).Build()

	ctx, cancel := context.WithCancel(ctx)
	syncDone := func() {
		cancel()
		wallet.Shutdown()
	}

	wallet.BlockProcessor = v.prepareProcessor(timeout, hash, syncDone)

	return wallet.Sync(ctx, currentBlock)
}

type VDBlockProcessor func(b *block.Block) error

func (p VDBlockProcessor) ProcessBlock(b *block.Block) error {
	return p(b)
}

func (v *VDClient) prepareProcessor(timeout uint64, hash []byte, syncDone func()) VDBlockProcessor {
	return func(b *block.Block) error {
		log.Debug("Fetched block #", b.UnicityCertificate.InputRecord.RoundNumber, ", tx count: ", len(b.GetTransactions()))
		if b.UnicityCertificate.InputRecord.RoundNumber > timeout {
			log.Info("Block timeout reached")
			syncDone()
			return nil
		}
		for _, tx := range b.GetTransactions() {
			log.Debug("Processing block #", b.UnicityCertificate.InputRecord.RoundNumber)
			if hash != nil {
				// if hash is provided, print only the corresponding block
				if bytes.Equal(hash, tx.GetUnitId()) {
					log.Info(fmt.Sprintf("Tx in block #%d, hash: %s", b.UnicityCertificate.InputRecord.RoundNumber, hex.EncodeToString(hash)))
					if v.blockCallback != nil {
						log.Info("Invoking block callback")
						v.blockCallback(&VDBlock{blockNumber: b.UnicityCertificate.InputRecord.RoundNumber})
					}
					syncDone()
					break
				}
			} else {
				log.Info(fmt.Sprintf("Tx in block #%d, hash: %s", b.UnicityCertificate.InputRecord.RoundNumber, hex.EncodeToString(tx.GetUnitId())))
			}
		}
		return nil
	}
}

func (v *VDClient) Close() {
	log.Info("Shutting down")
	err := v.moneyNodeClient.Close()
	if err != nil {
		log.Error(err)
	}
	err = v.vdNodeClient.Close()
	if err != nil {
		log.Error(err)
	}
}

func validateHash(hash []byte) error {
	if len(hash) != sha256.Size {
		return fmt.Errorf("invalid hash length, expected %d bytes, got %d", sha256.Size, len(hash))
	}
	return nil
}

func createRegisterDataTx(systemId, hash []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:         hash,
		SystemId:       systemId,
		ClientMetadata: &txsystem.ClientMetadata{Timeout: timeout},
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
