package tokens

import (
	"context"
	goerrors "errors"
	"fmt"
	"net/url"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
)

const (
	uriMaxSize  = 4 * 1024
	dataMaxSize = 64 * 1024
)

var (
	ErrInvalidBlockSystemID = goerrors.New("invalid system identifier")
	ErrAttributesMissing    = goerrors.New("attributes missing")
	ErrInvalidURILength     = fmt.Errorf("URI exceeds the maximum allowed size of %v bytes", uriMaxSize)
	ErrInvalidDataLength    = fmt.Errorf("data exceeds the maximum allowed size of %v bytes", dataMaxSize)
)

type (
	Wallet struct {
		txs     block.TxConverter
		am      account.Manager
		backend *client.TokenBackend
	}
)

func Load(backendUrl string, am account.Manager) (*Wallet, error) {
	addr, err := url.Parse(backendUrl)
	if err != nil {
		return nil, err
	}

	txs, err := tokens.New()
	if err != nil {
		return nil, err
	}
	w := &Wallet{am: am, txs: txs, backend: client.New(*addr)}

	return w, nil
}

func (w *Wallet) Shutdown() {
	w.am.Close()
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) NewFungibleType(ctx context.Context, attrs *tokens.CreateFungibleTokenTypeAttributes, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new fungible token type")
	panic("not implemented")
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, attrs *tokens.CreateNonFungibleTokenTypeAttributes, typeId twb.TokenTypeID, subtypePredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new NFT type")
	panic("not implemented")
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, attrs *tokens.MintFungibleTokenAttributes, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new fungible token")
	panic("not implemented")
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs *tokens.MintNonFungibleTokenAttributes, tokenId twb.TokenID, mintPredicateArgs []*PredicateInput) (twb.TokenID, error) {
	log.Info("Creating new NFT")
	if attrs == nil {
		return nil, ErrAttributesMissing
	}
	if len(attrs.Uri) > uriMaxSize {
		return nil, ErrInvalidURILength
	}
	if attrs.Uri != "" && !util.IsValidURI(attrs.Uri) {
		return nil, fmt.Errorf("URI '%s' is invalid", attrs.Uri)
	}
	if len(attrs.Data) > dataMaxSize {
		return nil, ErrInvalidDataLength
	}
	panic("not implemented")
}

func (w *Wallet) ListTokenTypes(ctx context.Context, kind twb.Kind) ([]*twb.TokenUnitType, error) {
	pubKeys, err := w.am.GetPublicKeys()
	if err != nil {
		return nil, err
	}
	allTokenTypes := make([]*twb.TokenUnitType, 0)
	fetchForPubKey := func(pubKey []byte) ([]*twb.TokenUnitType, error) {
		allTokenTypesForKey := make([]*twb.TokenUnitType, 0)
		var types []twb.TokenUnitType
		offsetKey := ""
		var err error
		for {
			types, offsetKey, err = w.backend.GetTokenTypes(ctx, kind, pubKey, offsetKey, 0)
			if err != nil {
				return nil, err
			}
			for _, t := range types {
				allTokenTypesForKey = append(allTokenTypesForKey, &t)
			}
			if offsetKey == "" {
				break
			}
		}
		return allTokenTypesForKey, nil
	}

	for _, pubKey := range pubKeys {
		types, err := fetchForPubKey(pubKey)
		if err != nil {
			return nil, err
		}
		allTokenTypes = append(allTokenTypes, types...)
	}

	return allTokenTypes, nil
}

// GetTokenType returns non-nil TokenUnitType or error if not found or other issues
func (w *Wallet) GetTokenType(ctx context.Context, typeId twb.TokenTypeID) (*twb.TokenUnitType, error) {
	panic("not implemented")
}

// ListTokens specify accountNumber=0 to list tokens from all accounts
func (w *Wallet) ListTokens(ctx context.Context, kind twb.Kind, accountNumber uint64) (map[uint64][]*twb.TokenUnit, error) {
	var pubKeys [][]byte
	var err error
	singleKey := accountNumber > AllAccounts
	accountIdx := accountNumber - 1
	if singleKey {
		key, err := w.am.GetPublicKey(accountIdx)
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, key)
	} else {
		pubKeys, err = w.am.GetPublicKeys()
		if err != nil {
			return nil, err
		}
	}

	// account number -> list of its tokens
	allTokensByAccountNumber := make(map[uint64][]*twb.TokenUnit, 0)
	fetchForPubKey := func(pubKey []byte) ([]*twb.TokenUnit, error) {
		allTokensForKey := make([]*twb.TokenUnit, 0)
		var ts []twb.TokenUnit
		offsetKey := ""
		var err error
		for {
			ts, offsetKey, err = w.backend.GetTokens(ctx, kind, pubKey, offsetKey, 0)
			if err != nil {
				return nil, err
			}
			for _, t := range ts {
				allTokensForKey = append(allTokensForKey, &t)
			}
			if offsetKey == "" {
				break
			}
		}
		return allTokensForKey, nil
	}

	if singleKey {
		ts, err := fetchForPubKey(pubKeys[0])
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[accountNumber] = ts
		return allTokensByAccountNumber, nil
	}

	for idx, pubKey := range pubKeys {
		ts, err := fetchForPubKey(pubKey)
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[uint64(idx)+1] = ts
	}
	return allTokensByAccountNumber, nil
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId twb.TokenID, receiverPubKey twb.PubKey, invariantPredicateArgs []*PredicateInput) error {
	panic("not implemented")
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId twb.TokenTypeID, targetAmount uint64, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) error {
	panic("not implemented")
}

func (w *Wallet) UpdateNFTData(ctx context.Context, accountNumber uint64, tokenId []byte, data []byte, updatePredicateArgs []*PredicateInput) error {
	panic("not implemented")
}
