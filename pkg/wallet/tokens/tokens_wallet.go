package tokens

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"strings"
)

type (
	PublicKey []byte
	TokenKind uint

	TokenId     []byte
	TokenTypeId []byte
)

const (
	Any TokenKind = 1 << iota
	TokenType
	Token
	Fungible
	NonFungible
)

const (
	txTimeoutBlockCount     = 100
	AllAccounts         int = -1
)

var (
	ErrInvalidBlockSystemID = errors.New("invalid system identifier")
)

type (
	TokensWallet struct {
		mw            *money.Wallet
		db            *tokensDb
		txs           block.TxConverter
		blockListener wallet.BlockProcessor
	}

	BlockListener func(b *block.Block) error
)

func (l BlockListener) ProcessBlock(b *block.Block) error {
	return l(b)
}

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
	w := &TokensWallet{mw, db, txs, nil}
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
				for idx, accPubKey := range pubKeys {
					err = w.readTx(txc, tx, uint64(idx), accPubKey)
					if err != nil {
						return err
					}
				}
				log.Info(fmt.Sprintf("tx with UnitID=%X", tx.UnitId))
			}
		}

		lst := w.blockListener
		if lst != nil {
			go func() {
				err := lst.ProcessBlock(b)
				if err != nil {
					log.Info(fmt.Sprintf("Failed to process a block #%v with blockListener", b.BlockNumber))
				}
			}()
		}

		return txc.SetBlockNumber(b.BlockNumber)
	})
}

func (w *TokensWallet) readTx(txc TokenTxContext, tx *txsystem.Transaction, accIdx uint64, key PublicKey) error {
	gtx, err := w.txs.ConvertTx(tx)
	if err != nil {
		return err
	}
	log.Info("Converted tx: ", gtx)
	id := util.Uint256ToBytes(gtx.UnitID())
	switch ctx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		log.Info("Token tx: CreateFungibleTokenType")
		err := txc.SetToken(accIdx, &token{
			Id:     id,
			Kind:   TokenType | Fungible,
			Symbol: ctx.Symbol(),
		})
		if err != nil {
			return err
		}
	case tokens.MintFungibleToken:
		log.Info("Token tx: MintFungibleToken")
		err := txc.SetToken(accIdx, &token{
			Id:     id,
			Kind:   Token | Fungible,
			TypeId: ctx.TypeId(),
			Amount: ctx.Value(),
		})
		if err != nil {
			return err
		}
	case tokens.TransferFungibleToken:
		log.Info("Token tx: TransferFungibleToken")
		// TODO remove token if bearer is someone else
	case tokens.SplitFungibleToken:
		log.Info("Token tx: SplitFungibleToken")
		// TODO
	case tokens.BurnFungibleToken:
		log.Info("Token tx: BurnFungibleToken")
		// TODO
	case tokens.JoinFungibleToken:
		log.Info("Token tx: JoinFungibleToken")
		// TODO
	case tokens.CreateNonFungibleTokenType:
		log.Info("Token tx: CreateNonFungibleTokenType")
		// TODO
	case tokens.MintNonFungibleToken:
		log.Info("Token tx: MintNonFungibleToken")
		// TODO
	case tokens.TransferNonFungibleToken:
		log.Info("Token tx: TransferNonFungibleToken")
		// TODO
	case tokens.UpdateNonFungibleToken:
		log.Info("Token tx: UpdateNonFungibleToken")
		// TODO
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
	return w.mw.Wallet.SyncToMaxBlockNumber(ctx, latestBlockNumber)
}

func (w *TokensWallet) SyncUntilCanceled(ctx context.Context) error {
	latestBlockNumber, err := w.db.Do().GetBlockNumber()
	if err != nil {
		return err
	}
	log.Info("Synchronizing tokens from block #", latestBlockNumber)
	return w.mw.Wallet.Sync(ctx, latestBlockNumber)
}

func (w *TokensWallet) NewFungibleType(ctx context.Context, attrs *tokens.CreateFungibleTokenTypeAttributes) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new fungible token type"))
	id, err := w.sendTx(attrs)
	if err != nil {
		return nil, err
	}

	return id, w.syncToUnit(ctx, id)
}

func (w *TokensWallet) NewFungibleToken(ctx context.Context, accIdx uint64, attrs *tokens.MintFungibleTokenAttributes) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new fungible token"))
	key, err := w.mw.GetAccountKey(accIdx)
	if err != nil {
		return nil, err
	}
	attrs.Bearer = script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256)
	id, err := w.sendTx(attrs)
	if err != nil {
		return nil, err
	}

	return id, w.syncToUnit(ctx, id)
}

func (w *TokensWallet) syncToUnit(ctx context.Context, id TokenId) error {
	ctx, cancel := context.WithCancel(ctx)

	log.Info(fmt.Sprintf("Request sent, waiting the tx to be finalized"))
	var bl BlockListener = func(b *block.Block) error {
		log.Info(fmt.Sprintf("Listener has got the block #%v", b.BlockNumber))
		for _, tx := range b.Transactions {
			if bytes.Equal(tx.UnitId, id) {
				log.Info(fmt.Sprintf("Tx with UnitID=%X is in the block #%v", id, b.BlockNumber))
				cancel()
			}
		}
		return nil
	}
	w.blockListener = bl

	defer func() {
		w.blockListener = nil
		cancel()
	}()

	return w.SyncUntilCanceled(ctx)
}

func randomId() (TokenId, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (w *TokensWallet) sendTx(attrs proto.Message) (TokenId, error) {
	id, err := randomId()
	if err != nil {
		return nil, err
	}

	blockNumber, err := w.mw.GetMaxBlockNumber()
	if err != nil {
		return nil, err
	}
	log.Info(fmt.Sprintf("New UnitID=%X", id))
	gtx := createGenericTx(id, blockNumber+txTimeoutBlockCount)
	err = anypb.MarshalFrom(gtx.TransactionAttributes, attrs, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	res, err := w.mw.SendTransaction(gtx)
	if err != nil {
		return nil, err
	}
	if !res.Ok {
		return nil, errors.New("tx submission returned error code: " + res.Message)
	}
	return id, nil
}

func (w *TokensWallet) ListTokens(ctx context.Context, kind TokenKind, accountIdx int) error {

	err := w.Sync(ctx)
	if err != nil {
		return err
	}

	var pubKeys [][]byte
	if accountIdx > AllAccounts {
		pubKeys[0], err = w.mw.GetPublicKey(uint64(accountIdx))
		if err != nil {
			return err
		}
	} else {
		pubKeys, err = w.mw.GetPublicKeys()
		if err != nil {
			return err
		}
	}

	for idx, key := range pubKeys {
		tokens, err := w.db.Do().GetTokens(uint64(idx))
		if err != nil {
			return err
		}
		if len(tokens) > 0 {
			log.Info(fmt.Sprintf("Account #%v (key '%X') tokens: ", idx+1, key))
			for _, token := range tokens {
				log.Info(fmt.Sprintf("Id=%X, kind: %s", token.Id, token.Kind.pretty()))
			}
		}
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

func (k *TokenKind) pretty() string {
	if *k&Any > 0 {
		return "[any]"
	}
	res := make([]string, 0)
	if *k&TokenType > 0 {
		res = append(res, "type")
	} else {
		res = append(res, "token")
	}
	if *k&Fungible > 0 {
		res = append(res, "fungible")
	} else {
		res = append(res, "non-fungible")
	}
	return "[" + strings.Join(res, ",") + "]"
}
