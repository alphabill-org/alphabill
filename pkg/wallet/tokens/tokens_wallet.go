package tokens

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	PublicKey []byte
	TokenKind uint
)

const (
	Any TokenKind = 1 << iota
	TokenType
	Token
	Fungible
	NonFungible
)

const (
	txTimeoutBlockCount = 100
)

var (
	ErrInvalidBlockSystemID = errors.New("invalid system identifier")
)

type (
	TokensWallet struct {
		mw  *money.Wallet
		db  *tokensDb
		txs block.TxConverter
	}
)

func Load(mw *money.Wallet) (*TokensWallet, error) {
	config := mw.GetConfig()
	walletDir, err := config.GetWalletDir()
	if err != nil {
		return nil, err
	}

	db, err := openTokensDb(walletDir)
	if err != nil {
		return nil, err
	}
	txs, err := tokens.New()
	if err != nil {
		return nil, err
	}
	w := &TokensWallet{mw, db, txs}
	w.mw.Wallet = wallet.New().
		SetBlockProcessor(w).
		SetABClient(mw.AlphabillClient).
		Build()
	return w, nil
}

func (w *TokensWallet) Shutdown() {
	w.mw.Shutdown()
}

func (w *TokensWallet) ProcessBlock(b *block.Block) error {
	if !bytes.Equal(tokens.DefaultTokenTxSystemIdentifier, b.GetSystemIdentifier()) {
		return ErrInvalidBlockSystemID
	}
	return w.db.WithTransaction(func(txc TokenTxContext) error {
		blockNumber := b.BlockNumber
		lastBlockNumber, err := txc.GetBlockNumber()
		if err != nil {
			return nil
		}
		if blockNumber != lastBlockNumber+1 {
			return errors.New(fmt.Sprintf("Invalid block height. Received blockNumber %d current wallet blockNumber %d", blockNumber, lastBlockNumber))
		}

		if len(b.Transactions) != 0 {
			log.Info("processing non-empty block: ", b.BlockNumber)

			// lists tokens for all keys and with 'always true' predicate
			pubKeys, err := w.mw.GetPublicKeys()
			if err != nil {
				return err
			}
			log.Info(fmt.Sprintf("pub keys: %v", len(pubKeys)))
			for _, tx := range b.Transactions {
				//for _, accPubKey := range pubKeys {
				err = w.fetchTokens(txc, tx, 0, nil)
				if err != nil {
					return err
				}
				//}
				log.Info(fmt.Sprintf("tx with UnitID=%X", tx.UnitId))
			}
		}

		return txc.SetBlockNumber(b.BlockNumber)
	})
}

func (w *TokensWallet) fetchTokens(txc TokenTxContext, tx *txsystem.Transaction, accIdx uint64, key PublicKey) error {
	gtx, err := w.txs.ConvertTx(tx)
	if err != nil {
		return err
	}
	log.Info("Converted tx: ", gtx)
	id := util.Uint256ToBytes(gtx.UnitID())
	switch ctx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		log.Info("Token tx: CreateFungibleTokenType")
		txc.SetToken(accIdx, &token{
			Id:   id,
			Kind: TokenType | Fungible,
		})
	case tokens.MintFungibleToken:
		log.Info("Token tx: MintFungibleToken")
	case tokens.TransferFungibleToken:
		log.Info("Token tx: TransferFungibleToken")
	case tokens.SplitFungibleToken:
		log.Info("Token tx: SplitFungibleToken")
	case tokens.BurnFungibleToken:
		log.Info("Token tx: BurnFungibleToken")
	case tokens.JoinFungibleToken:
		log.Info("Token tx: JoinFungibleToken")
	case tokens.CreateNonFungibleTokenType:
		log.Info("Token tx: CreateNonFungibleTokenType")
	case tokens.MintNonFungibleToken:
		log.Info("Token tx: MintNonFungibleToken")
	case tokens.TransferNonFungibleToken:
		log.Info("Token tx: TransferNonFungibleToken")
	case tokens.UpdateNonFungibleToken:
		log.Info("Token tx: UpdateNonFungibleToken")
	default:
		log.Warning(fmt.Sprintf("received unknown token transaction type, skipped processing: %s", ctx))
		return nil
	}
	return nil
}

func (w *TokensWallet) Sync(ctx context.Context) error {
	latestBlockNumber, err := w.db.Do().GetBlockNumber()
	if err != nil {
		return err
	}
	log.Info("Synchronizing tokens from block #", latestBlockNumber)
	err = w.mw.Wallet.SyncToMaxBlockNumber(ctx, latestBlockNumber)
	log.Info("SyncToMaxBlockNumber returned")
	return err
}

func (w *TokensWallet) NewFungibleType(attrs *tokens.CreateFungibleTokenTypeAttributes) error {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return err
	}

	blockNumber, err := w.mw.GetMaxBlockNumber()
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Creating new type with UnitID=%X", id))
	gtx := createGenericTx(id, blockNumber+txTimeoutBlockCount)
	err = anypb.MarshalFrom(gtx.TransactionAttributes, attrs, proto.MarshalOptions{})
	if err != nil {
		return err
	}
	res, err := w.mw.SendTransaction(gtx)
	if err != nil {
		return err
	}
	if !res.Ok {
		return errors.New("tx submission returned error code: " + res.Message)
	}
	return nil
}

func (w *TokensWallet) ListTokens(ctx context.Context, kind TokenKind) error {

	err := w.Sync(ctx)
	if err != nil {
		return err
	}
	if kind&(Token|Fungible) != 0 {
		// TODO
	}
	return nil
}

func createGenericTx(unitId []byte, timeout uint64) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              tokens.DefaultTokenTxSystemIdentifier,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		// OwnerProof is added after whole transaction is built
	}
}
