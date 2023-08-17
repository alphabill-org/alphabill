package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
)

const (
	blockSyncBatchSize = 600
	vdStorageFileName  = "vd.db"
)

var (
	ErrNoFeeCredit           = errors.New("no fee credit in vd wallet")
	ErrInsufficientFeeCredit = errors.New("insufficient fee credit for transaction")
)

type (
	VDClient struct {
		vdNodeClient     client.ABClient
		confirmTx        bool
		confirmTxTimeout uint64
		accountKey       *account.AccountKey
		walletHomeDir    string
	}

	VDClientConfig struct {
		// VDNodeURL URL of the vd node to connect
		VDNodeURL string
		// WaitForReady if true, VD node client waits for the node to be ready
		WaitForReady bool
		// ConfirmTx upon hash submission, waits until the transaction is included in a block, or timeout is reached
		ConfirmTx bool
		// ConfirmTxTimeout number of blocks to wait for the confirmation of a transaction
		ConfirmTxTimeout uint64
		// AccountKey Account key used to create FeeProof for register data transactions
		AccountKey *account.AccountKey
		// WalletHomeDir a directory for storage file
		WalletHomeDir string
	}
)

func New(conf *VDClientConfig) (*VDClient, error) {
	vdNodeClient := client.New(client.AlphabillClientConfig{
		Uri:          conf.VDNodeURL,
		WaitForReady: conf.WaitForReady,
	})

	return &VDClient{
		vdNodeClient:     vdNodeClient,
		confirmTx:        conf.ConfirmTx,
		confirmTxTimeout: conf.ConfirmTxTimeout,
		accountKey:       conf.AccountKey,
		walletHomeDir:    conf.WalletHomeDir,
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

func (v *VDClient) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
	for _, tx := range txs.Transactions {
		err := v.vdNodeClient.SendTransaction(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to send transaction: %w", err)
		}
	}
	return nil
}

func (v *VDClient) GetTxProofFromBlock(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash, startRoundNumber, timeout uint64) (*wallet.Proof, error) {
	proofCh := make(chan *wallet.Proof)
	defer close(proofCh)

	ctx, cancelCause := context.WithCancelCause(ctx)

	blockProcessor := NewProofFinder(unitID, txHash, proofCh)
	go func() {
		err := blocksync.Run(ctx, v.vdNodeClient.GetBlocks, startRoundNumber, timeout, blockSyncBatchSize, blockProcessor)
		if context.Cause(ctx) == nil {
			cancelCause(err)
		}
	}()

	select {
	case proof := <-proofCh:
		cancelCause(nil)
		return proof, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func (v *VDClient) GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error) {
	dbFilePath := filepath.Join(v.walletHomeDir, vdStorageFileName)
	db, err := newBoltStore(dbFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open storage: %w", err)
	}
	defer db.Close()

	lastBlockNumber, err := db.GetBlockNumber()
	if err != nil {
		return nil, fmt.Errorf("failed to get last block number from storage: %w", err)
	}

	maxBlockNumber, err := v.vdNodeClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest round number from node: %w", err)
	}

	if lastBlockNumber < maxBlockNumber {
		blockProcessor := NewBlockProcessor(db)
		err = blocksync.Run(ctx, v.vdNodeClient.GetBlocks, lastBlockNumber+1, maxBlockNumber, blockSyncBatchSize, blockProcessor.ProcessBlock)
		if err != nil {
			return nil, err
		}
	}

	fcb, err := db.GetFeeCreditBill(unitID)
	if err != nil {
		return nil, err
	}

	if fcb == nil {
		return nil, nil
	}

	return &wallet.Bill{
		Id:              fcb.Id,
		Value:           fcb.Value,
		TxHash:          fcb.TxHash,
		LastAddFCTxHash: fcb.LastAddFCTxHash,
	}, nil
}

func (v *VDClient) GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error) {
	// TODO impl
	return nil, nil
}

func (v *VDClient) GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	// TODO impl
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

// ListAllBlocksWithTx prints all non-empty blocks from genesis up to the latest block
func (v *VDClient) ListAllBlocksWithTx(ctx context.Context) error {
	log.Info("Fetching blocks...")
	maxBlockNumber, err := v.vdNodeClient.GetRoundNumber(ctx)
	if err != nil {
		return err
	}

	err = blocksync.Run(ctx, v.vdNodeClient.GetBlocks, 1, maxBlockNumber, blockSyncBatchSize, NewTxPrinter())

	if err != nil {
		return err
	}

	log.Info("Done.")
	return nil
}

func (v *VDClient) registerHashTx(ctx context.Context, hash []byte) error {
	if err := validateHash(hash); err != nil {
		return err
	}

	err := v.ensureFeeCredit(ctx, v.accountKey)
	if err != nil {
		return err
	}

	currentRoundNumber, err := v.vdNodeClient.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	log.Info("Current block #: ", currentRoundNumber)
	timeout := currentRoundNumber + v.confirmTxTimeout

	tx, err := v.createRegisterDataTx(hash, timeout)
	if err != nil {
		return err
	}

	if err := v.vdNodeClient.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("error while submitting the hash: %w", err)
	}

	if v.confirmTx {
		proof, err := v.GetTxProofFromBlock(ctx, hash, nil, currentRoundNumber, timeout)
		if proof != nil {
			log.Info(fmt.Sprintf("Tx in block #%d, hash: %s",
				proof.TxProof.UnicityCertificate.GetRoundNumber(), hex.EncodeToString(hash)))
		}
		if err != nil {
			return fmt.Errorf("failed to confirm transaction: %w", err)
		}
	}
	return nil
}

func (v *VDClient) ensureFeeCredit(ctx context.Context, accountKey *account.AccountKey) error {
	fcb, err := v.GetFeeCreditBill(ctx, accountKey.PubKeyHash.Sha256)
	if err != nil {
		return err
	}
	if fcb == nil {
		return ErrNoFeeCredit
	}
	maxFee := tx_builder.MaxFee
	if fcb.GetValue() < maxFee {
		return ErrInsufficientFeeCredit
	}
	return nil
}

func (v *VDClient) createRegisterDataTx(data []byte, timeout uint64) (*types.TransactionOrder, error) {
	payload := &types.Payload{
		SystemID: v.SystemID(),
		Type:     vd.PayloadTypeRegisterData,
		UnitID:   data,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: tx_builder.MaxFee,
			FeeCreditRecordID: v.accountKey.PrivKeyHash,
		},
	}

	return v.signPayload(payload)
}

func (v *VDClient) signPayload(payload *types.Payload) (*types.TransactionOrder, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(v.accountKey.PrivKey)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := payload.Bytes()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(payloadBytes)
	if err != nil {
		return nil, err
	}

	return &types.TransactionOrder{
		Payload:  payload,
		FeeProof: script.PredicateArgumentPayToPublicKeyHashDefault(sig, v.accountKey.PubKey),
	}, nil
}

func (v *VDClient) Close() {
	log.Info("Shutting down")
	err := v.vdNodeClient.Close()
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

func hexStringToBytes(hexString string) ([]byte, error) {
	bs, err := hex.DecodeString(strings.TrimPrefix(hexString, "0x"))
	if err != nil {
		return nil, err
	}
	return bs, nil
}
