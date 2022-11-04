package tokens

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"reflect"
	"sort"
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
	FungibleToken    = Token | Fungible
	NonFungibleToken = Token | NonFungible
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
			accounts, err := w.mw.GetAccountKeys()
			if err != nil {
				return err
			}
			log.Info(fmt.Sprintf("pub keys: %v", len(accounts)))
			for _, tx := range b.Transactions {
				for n := 0; n <= len(accounts); n++ {
					var keyHashes *wallet.KeyHashes = nil
					if n > 0 {
						keyHashes = accounts[n-1].PubKeyHash
					}
					err = w.readTx(txc, tx, uint64(n), keyHashes)
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

func (w *TokensWallet) readTx(txc TokenTxContext, tx *txsystem.Transaction, accNr uint64, key *wallet.KeyHashes) error {
	gtx, err := w.txs.ConvertTx(tx)
	if err != nil {
		return err
	}
	id := util.Uint256ToBytes(gtx.UnitID())
	txHash := gtx.Hash(crypto.SHA256)
	log.Info(fmt.Sprintf("Converted tx: UnitId=%X, TxId=%X", id, txHash))

	switch ctx := gtx.(type) {
	case tokens.CreateFungibleTokenType:
		log.Info("CreateFungibleTokenType tx")
		err := txc.AddTokenType(&tokenType{
			Id:            id,
			Kind:          TokenType | Fungible,
			Symbol:        ctx.Symbol(),
			ParentTypeId:  ctx.ParentTypeId(),
			DecimalPlaces: ctx.DecimalPlaces(),
		})
		if err != nil {
			return err
		}
	case tokens.MintFungibleToken:
		log.Info("MintFungibleToken tx")
		if checkOwner(accNr, key, ctx.Bearer()) {
			tType, err := txc.GetTokenType(ctx.TypeId())
			if err != nil {
				return err
			}
			err = txc.SetToken(accNr, &token{
				Id:       id,
				Kind:     FungibleToken,
				TypeId:   ctx.TypeId(),
				Amount:   ctx.Value(),
				Backlink: make([]byte, crypto.SHA256.Size()), //zerohash
				Symbol:   tType.Symbol,
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.TransferFungibleToken:
		log.Info("TransferFungibleToken tx")
		if checkOwner(accNr, key, ctx.NewBearer()) {
			err := txc.SetToken(accNr, &token{
				Id:       id,
				Kind:     FungibleToken,
				Amount:   ctx.Value(),
				Backlink: txHash,
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.SplitFungibleToken:
		log.Info("SplitFungibleToken tx")
		tok, found, err := txc.GetToken(accNr, id)
		if err != nil {
			return err
		}
		var tokenInfo TokenTypeInfo
		if found {
			tokenInfo = tok
			log.Info("SplitFungibleToken updating existing unit")
			err := txc.SetToken(accNr, &token{
				Id:       id,
				Symbol:   tok.Symbol,
				TypeId:   tok.TypeId,
				Kind:     tok.Kind,
				Amount:   tok.Amount - ctx.TargetValue(),
				Backlink: txHash,
			})
			if err != nil {
				return err
			}
		} else {
			tokenInfo = &token{}
		}

		if checkOwner(accNr, key, ctx.NewBearer()) {
			newId := txutil.SameShardIdBytes(ctx.UnitID(), ctx.HashForIdCalculation(crypto.SHA256))
			log.Info(fmt.Sprintf("SplitFungibleToken: adding new unit from split, new UnitId=%X", newId))
			err := txc.SetToken(accNr, &token{
				Id:       newId,
				Symbol:   tokenInfo.GetSymbol(),
				TypeId:   tokenInfo.GetTypeId(),
				Kind:     FungibleToken,
				Amount:   ctx.TargetValue(),
				Backlink: txHash,
			})
			if err != nil {
				return err
			}
		}
	case tokens.BurnFungibleToken:
		log.Info("Token tx: BurnFungibleToken")
		panic("not implemented") // TODO
	case tokens.JoinFungibleToken:
		log.Info("Token tx: JoinFungibleToken")
		panic("not implemented") // TODO
	case tokens.CreateNonFungibleTokenType:
		log.Info("Token tx: CreateNonFungibleTokenType")
		err := txc.AddTokenType(&tokenType{
			Id:           id,
			Kind:         TokenType | NonFungible,
			Symbol:       ctx.Symbol(),
			ParentTypeId: ctx.ParentTypeId(),
		})
		if err != nil {
			return err
		}
	case tokens.MintNonFungibleToken:
		log.Info("Token tx: MintNonFungibleToken")
		if checkOwner(accNr, key, ctx.Bearer()) {
			tType, err := txc.GetTokenType(ctx.NFTTypeId())
			if err != nil {
				return err
			}
			err = txc.SetToken(accNr, &token{
				Id:       id,
				Kind:     NonFungibleToken,
				TypeId:   tType.Id,
				Uri:      ctx.URI(),
				Backlink: make([]byte, crypto.SHA256.Size()), //zerohash
				Symbol:   tType.Symbol,
				//ctx.Data() // TODO
				//ctx.DataUpdatePredicate()
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.TransferNonFungibleToken:
		log.Info("Token tx: TransferNonFungibleToken")
		if checkOwner(accNr, key, ctx.NewBearer()) {
			err := txc.SetToken(accNr, &token{
				Id:       id,
				Kind:     NonFungibleToken,
				Backlink: txHash,
			})
			if err != nil {
				return err
			}
		} else {
			err := txc.RemoveToken(accNr, id)
			if err != nil {
				return err
			}
		}
	case tokens.UpdateNonFungibleToken:
		log.Info("Token tx: UpdateNonFungibleToken")
		panic("not implemented") // TODO
	default:
		log.Warning(fmt.Sprintf("received unknown token transaction type, skipped processing: %s", ctx))
		return nil
	}
	return nil
}

func checkOwner(accNr uint64, pubkeyHashes *wallet.KeyHashes, bearerPredicate []byte) bool {
	if accNr == alwaysTrueTokensAccountNumber {
		return bytes.Equal(script.PredicateAlwaysTrue(), bearerPredicate)
	} else {
		return wallet.VerifyP2PKHOwner(pubkeyHashes, bearerPredicate)
	}
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

func (w *TokensWallet) NewFungibleType(ctx context.Context, attrs *tokens.CreateFungibleTokenTypeAttributes, typeId TokenTypeId) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new fungible token type"))
	return w.newType(ctx, attrs, typeId)
}

func (w *TokensWallet) NewNonFungibleType(ctx context.Context, attrs *tokens.CreateNonFungibleTokenTypeAttributes, typeId TokenTypeId) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new NFT type"))
	return w.newType(ctx, attrs, typeId)
}

func (w *TokensWallet) newType(ctx context.Context, attrs proto.Message, typeId TokenTypeId) (TokenId, error) {
	sub, err := w.sendTx(TokenId(typeId), attrs, nil)
	if err != nil {
		return nil, err
	}
	return sub.id, w.syncToUnit(ctx, sub.id, sub.timeout)
}

func (w *TokensWallet) NewFungibleToken(ctx context.Context, accNr uint64, attrs *tokens.MintFungibleTokenAttributes) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new fungible token"))
	return w.newToken(ctx, accNr, attrs, nil)
}

func (w *TokensWallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId TokenId) (TokenId, error) {
	log.Info(fmt.Sprintf("Creating new NFT"))
	return w.newToken(ctx, accNr, attrs, tokenId)
}

func (w *TokensWallet) newToken(ctx context.Context, accNr uint64, attrs tokens.AttrWithBearer, tokenId TokenId) (TokenId, error) {
	accIdx := accNr - 1
	key, err := w.mw.GetAccountKey(accIdx)
	if err != nil {
		return nil, err
	}
	attrs.SetBearer(script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256))
	sub, err := w.sendTx(tokenId, attrs, nil)
	if err != nil {
		return nil, err
	}

	return sub.id, w.syncToUnit(ctx, sub.id, sub.timeout)
}

func (w *TokensWallet) syncToUnit(ctx context.Context, id TokenId, timeout uint64) error {
	submissions := make(map[string]*submittedTx, 1)
	submissions[id.string()] = &submittedTx{id, timeout}
	return w.syncToUnits(ctx, submissions, timeout)
}

func (w *TokensWallet) syncToUnits(ctx context.Context, subs map[string]*submittedTx, maxTimeout uint64) error {
	ctx, cancel := context.WithCancel(ctx)

	log.Info(fmt.Sprintf("Waiting the transactions to be finalized"))
	var bl BlockListener = func(b *block.Block) error {
		log.Debug(fmt.Sprintf("Listener has got the block #%v", b.BlockNumber))
		if b.BlockNumber > maxTimeout {
			log.Info(fmt.Sprintf("Sync timeout is reached, block (#%v)", b.BlockNumber))
			for _, sub := range subs {
				log.Info(fmt.Sprintf("Tx not found for UnitID=%X", sub.id))
			}
			cancel()
		}
		for _, tx := range b.Transactions {
			id := TokenId(tx.UnitId).string()
			if sub, found := subs[id]; found {
				log.Info(fmt.Sprintf("Tx with UnitID=%X is in the block #%v", sub.id, b.BlockNumber))
				delete(subs, id)
			}
			if len(subs) == 0 {
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

type submittedTx struct {
	id      TokenId
	timeout uint64
}

func (w *TokensWallet) sendTx(unitId TokenId, attrs proto.Message, ac *wallet.AccountKey) (*submittedTx, error) {
	txSub := &submittedTx{id: unitId}
	if unitId == nil {
		id, err := randomId()
		if err != nil {
			return txSub, err
		}
		txSub.id = id
	}
	log.Info(fmt.Sprintf("Sending token tx, UnitID=%X, attributes: %v", unitId, reflect.TypeOf(attrs)))

	blockNumber, err := w.mw.GetMaxBlockNumber()
	if err != nil {
		return txSub, err
	}
	tx := createTx(txSub.id, blockNumber+txTimeoutBlockCount)
	err = anypb.MarshalFrom(tx.TransactionAttributes, attrs, proto.MarshalOptions{})
	if err != nil {
		return txSub, err
	}
	err = signTx(tx, ac)
	if err != nil {
		return txSub, err
	}
	res, err := w.mw.SendTransaction(tx)
	if err != nil {
		return txSub, err
	}
	if !res.Ok {
		return txSub, errors.New("tx submission returned error code: " + res.Message)
	}
	txSub.timeout = tx.Timeout
	return txSub, nil
}

func signTx(tx *txsystem.Transaction, ac *wallet.AccountKey) error {
	gtx, err := tokens.NewGenericTx(tx)
	if err != nil {
		return err
	}
	if ac != nil {
		signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
		if err != nil {
			return err
		}
		sig, err := signer.SignBytes(gtx.SigBytes())
		if err != nil {
			return err
		}
		tx.OwnerProof = script.PredicateArgumentPayToPublicKeyHashDefault(sig, ac.PubKey)
	} else {
		tx.OwnerProof = script.PredicateArgumentEmpty()
	}
	return nil
}

func (w *TokensWallet) ListTokenTypes(ctx context.Context) ([]string, error) {
	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}

	types, err := w.db.Do().GetTokenTypes()
	if err != nil {
		return nil, err
	}
	res := make([]string, len(types))
	for _, t := range types {
		m := fmt.Sprintf("Id=%X, symbol=%s, kind: %s", t.Id, t.Symbol, t.Kind.pretty())
		log.Info(m)
		res = append(res, m)
	}
	return res, nil
}

func (w *TokensWallet) ListTokens(ctx context.Context, kind TokenKind, accountNumber int) ([]string, error) {

	err := w.Sync(ctx)
	if err != nil {
		return nil, err
	}

	var pubKeys [][]byte
	if accountNumber > AllAccounts+1 {
		pubKeys[0], err = w.mw.GetPublicKey(uint64(accountNumber - 1))
		if err != nil {
			return nil, err
		}
	} else {
		pubKeys, err = w.mw.GetPublicKeys()
		if err != nil {
			return nil, err
		}
	}

	res := make([]string, len(pubKeys)+1)
	for n := 0; n <= len(pubKeys); n++ {
		tokens, err := w.db.Do().GetTokens(uint64(n))
		if err != nil {
			return nil, err
		}
		if len(tokens) > 0 {
			// TODO filter by kind
			var m string
			if n == alwaysTrueTokensAccountNumber {
				m = fmt.Sprintf("Tokens spendable by anyone: ")
			} else {
				m = fmt.Sprintf("Account #%v (key '%X') tokens: ", n, pubKeys[n-1])
			}
			log.Info(m)
			res = append(res, m)
			for _, token := range tokens {
				m = fmt.Sprintf("Id=%X, symbol=%s, value=%v, kind: %s", token.Id, token.Symbol, token.Amount, token.Kind.pretty())
				log.Info(m)
				res = append(res, m)
			}
		}
	}
	return res, nil
}

func (w *TokensWallet) Transfer(ctx context.Context, accountNumber uint64, tokenId TokenId, receiverPubKey []byte) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	t, found, err := w.db.Do().GetToken(accountNumber, tokenId)
	if err != nil {
		return err
	}
	if !found {
		return errors.New(fmt.Sprintf("token with id=%X not found under account #%v", tokenId, accountNumber))
	}
	return w.transfer(ctx, acc, t, receiverPubKey)
}

func newFungibleTransferTxAttrs(token *token, receiverPubKey []byte) *tokens.TransferFungibleTokenAttributes {
	var bearer []byte
	if receiverPubKey != nil {
		bearer = script.PredicatePayToPublicKeyHashDefault(hash.Sum256(receiverPubKey))
	} else {
		bearer = script.PredicateAlwaysTrue()
	}

	log.Info(fmt.Sprintf("Creating transfer with bl=%X", token.Backlink))

	return &tokens.TransferFungibleTokenAttributes{
		NewBearer:                   bearer,
		Value:                       token.Amount,
		Backlink:                    token.Backlink,
		InvariantPredicateSignature: script.PredicateArgumentEmpty(),
	}
}

func (w *TokensWallet) transfer(ctx context.Context, ac *wallet.AccountKey, token *token, receiverPubKey []byte) error {
	sub, err := w.sendTx(token.Id, newFungibleTransferTxAttrs(token, receiverPubKey), ac)
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, token.Id, sub.timeout)
}

func newNonFungibleTransferTxAttrs(token *token, receiverPubKey []byte) *tokens.TransferNonFungibleTokenAttributes {
	var bearer []byte
	if receiverPubKey != nil {
		bearer = script.PredicatePayToPublicKeyHashDefault(hash.Sum256(receiverPubKey))
	} else {
		bearer = script.PredicateAlwaysTrue()
	}

	log.Info(fmt.Sprintf("Creating NFT transfer with bl=%X", token.Backlink))

	return &tokens.TransferNonFungibleTokenAttributes{
		NewBearer:                   bearer,
		Backlink:                    token.Backlink,
		InvariantPredicateSignature: script.PredicateArgumentEmpty(),
	}
}

func (w *TokensWallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId TokenId, receiverPubKey []byte) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	t, found, err := w.db.Do().GetToken(accountNumber, tokenId)
	if err != nil {
		return err
	}
	if !found {
		return errors.New(fmt.Sprintf("token with id=%X not found under account #%v", tokenId, accountNumber))
	}

	sub, err := w.sendTx(tokenId, newNonFungibleTransferTxAttrs(t, receiverPubKey), acc)
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, tokenId, sub.timeout)
}

func newSplitTxAttrs(token *token, amount uint64, receiverPubKey []byte) *tokens.SplitFungibleTokenAttributes {
	var bearer []byte
	if receiverPubKey != nil {
		bearer = script.PredicatePayToPublicKeyHashDefault(hash.Sum256(receiverPubKey))
	} else {
		bearer = script.PredicateAlwaysTrue()
	}

	log.Info(fmt.Sprintf("Creating split with bl=%X, new value=%v", token.Backlink, amount))

	return &tokens.SplitFungibleTokenAttributes{
		NewBearer:                   bearer,
		TargetValue:                 amount,
		Backlink:                    token.Backlink,
		InvariantPredicateSignature: script.PredicateArgumentEmpty(),
	}
}

func (w *TokensWallet) split(ctx context.Context, ac *wallet.AccountKey, token *token, amount uint64, receiverPubKey []byte) error {
	if amount >= token.Amount {
		return errors.New(fmt.Sprintf("invalid target value for split: %v, token value=%v, UnitId=%X", amount, token.Amount, token.Id))
	}

	sub, err := w.sendTx(token.Id, newSplitTxAttrs(token, amount, receiverPubKey), ac)
	if err != nil {
		return err
	}

	return w.syncToUnit(ctx, token.Id, sub.timeout)
}

func (w *TokensWallet) SendFungible(ctx context.Context, accountNumber uint64, typeId TokenTypeId, targetAmount uint64, receiverPubKey []byte) error {
	acc, err := w.getAccountKey(accountNumber)
	if err != nil {
		return err
	}
	tokens, err := w.db.Do().GetTokens(accountNumber)
	if err != nil {
		return err
	}
	fungibleTokens := make([]*token, 0)
	var totalBalance uint64 = 0
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *token = nil
	for _, token := range tokens {
		if token.isFungible() && typeId.equal(token.TypeId) {
			fungibleTokens = append(fungibleTokens, token)
			totalBalance += token.Amount
			if closestMatch == nil {
				closestMatch = token
			} else {
				prevDiff := closestMatch.Amount - targetAmount
				currDiff := token.Amount - targetAmount
				// this should work with overflow nicely
				if prevDiff > currDiff {
					closestMatch = token
				}
			}
		}
	}
	if targetAmount > totalBalance {
		return errors.New(fmt.Sprintf("insufficient value: got %v, need %v", totalBalance, targetAmount))
	}
	var submissions map[string]*submittedTx
	var maxTimeout uint64
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		var sub *submittedTx
		sub, err = w.sendSplitOrTransferTx(acc, targetAmount, closestMatch, receiverPubKey)
		submissions = make(map[string]*submittedTx, 1)
		submissions[sub.id.string()] = sub
		maxTimeout = sub.timeout
	} else {
		submissions, maxTimeout, err = w.doSendMultiple(targetAmount, fungibleTokens, acc, receiverPubKey)
	}

	// error might have happened, but some submissions could have succeeded
	syncErr := w.syncToUnits(ctx, submissions, maxTimeout)

	if err != nil {
		return err
	}
	return syncErr
}

func (w *TokensWallet) getAccountKey(accountNumber uint64) (*wallet.AccountKey, error) {
	if accountNumber > 0 {
		return w.mw.GetAccountKey(accountNumber - 1)
	}
	return nil, nil
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *TokensWallet) doSendMultiple(amount uint64, tokens []*token, acc *wallet.AccountKey, receiverPubKey []byte) (map[string]*submittedTx, uint64, error) {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})
	var maxTimeout uint64 = 0
	submissions := make(map[string]*submittedTx, 2)
	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.sendSplitOrTransferTx(acc, remainingAmount, t, receiverPubKey)
		if sub.timeout > maxTimeout {
			maxTimeout = sub.timeout
		}
		if err != nil {
			return submissions, maxTimeout, err
		}
		submissions[sub.id.string()] = sub
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	return submissions, maxTimeout, nil
}

func (w *TokensWallet) sendSplitOrTransferTx(acc *wallet.AccountKey, amount uint64, token *token, receiverPubKey []byte) (*submittedTx, error) {
	var attrs proto.Message
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
	}
	sub, err := w.sendTx(token.Id, attrs, acc)
	if err != nil {
		return sub, err
	}
	return sub, nil
}

func createTx(unitId []byte, timeout uint64) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              tokens.DefaultTokenTxSystemIdentifier,
		UnitId:                unitId,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		// OwnerProof is added after whole transaction is built
	}
}

func (t *token) isFungible() bool {
	return t.Kind&FungibleToken == FungibleToken
}

func (k *TokenKind) pretty() string {
	if *k&Any != 0 {
		return "[any]"
	}
	res := make([]string, 0)
	if *k&TokenType != 0 {
		res = append(res, "type")
	} else {
		res = append(res, "token")
	}
	if *k&Fungible != 0 {
		res = append(res, "fungible")
	} else {
		res = append(res, "non-fungible")
	}
	return "[" + strings.Join(res, ",") + "]"
}

func (t TokenTypeId) equal(to TokenTypeId) bool {
	return bytes.Equal(t, to)
}

type TokenTypeInfo interface {
	GetSymbol() string
	GetTypeId() TokenTypeId
}

func (tp *tokenType) GetSymbol() string {
	return tp.Symbol
}

func (tp *tokenType) GetTypeId() TokenTypeId {
	return TokenTypeId(tp.Id)
}

func (t *token) GetSymbol() string {
	return t.Symbol
}

func (t *token) GetTypeId() TokenTypeId {
	return t.TypeId
}

func (id TokenId) string() string {
	return string(id)
}
