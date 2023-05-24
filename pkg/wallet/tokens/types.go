package tokens

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/alphabill-org/alphabill/pkg/wallet"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/protobuf/proto"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
)

const (
	txTimeoutRoundCount        = 10
	AllAccounts         uint64 = 0

	predicateEmpty = "empty"
	predicateTrue  = "true"
	predicateFalse = "false"
	predicatePtpkh = "ptpkh"
	hexPrefix      = "0x"
)

type (
	PredicateInput struct {
		// first priority
		Argument tokens.Predicate
		// if Argument empty, check AccountNumber
		AccountNumber uint64
	}

	CreateFungibleTokenTypeAttributes struct {
		Symbol                   string
		Name                     string
		Icon                     *Icon
		DecimalPlaces            uint32
		ParentTypeId             twb.TokenTypeID
		SubTypeCreationPredicate wallet.Predicate
		TokenCreationPredicate   wallet.Predicate
		InvariantPredicate       wallet.Predicate
	}

	Icon struct {
		Type string
		Data []byte
	}

	CreateNonFungibleTokenTypeAttributes struct {
		Symbol                   string
		Name                     string
		Icon                     *Icon
		ParentTypeId             twb.TokenTypeID
		SubTypeCreationPredicate wallet.Predicate
		TokenCreationPredicate   wallet.Predicate
		InvariantPredicate       wallet.Predicate
		DataUpdatePredicate      wallet.Predicate
	}

	MintNonFungibleTokenAttributes struct {
		Name                string
		NftType             twb.TokenTypeID
		Uri                 string
		Data                []byte
		Bearer              wallet.Predicate
		DataUpdatePredicate wallet.Predicate
	}

	MintAttr interface {
		proto.Message
		SetBearer([]byte)
		SetTokenCreationPredicateSignatures([][]byte)
	}

	AttrWithSubTypeCreationInputs interface {
		proto.Message
		SetSubTypeCreationPredicateSignatures([][]byte)
	}

	AttrWithInvariantPredicateInputs interface {
		SetInvariantPredicateSignatures([][]byte)
	}
)

func ParsePredicates(arguments []string, keyNr uint64, am account.Manager) ([]*PredicateInput, error) {
	creationInputs := make([]*PredicateInput, 0, len(arguments))
	for _, argument := range arguments {
		input, err := parsePredicate(argument, keyNr, am)
		if err != nil {
			return nil, err
		}
		creationInputs = append(creationInputs, input)
	}
	return creationInputs, nil
}

// parsePredicate uses the following format:
// empty|true|false|empty produce an empty predicate argument
// ptpkh (provided key #) or ptpkh:n (n > 0) produce an argument with the signed transaction by the given key
func parsePredicate(argument string, keyNr uint64, am account.Manager) (*PredicateInput, error) {
	if len(argument) == 0 || argument == predicateEmpty || argument == predicateTrue || argument == predicateFalse {
		return &PredicateInput{Argument: script.PredicateArgumentEmpty()}, nil
	}
	var err error
	if strings.HasPrefix(argument, predicatePtpkh) {
		if split := strings.Split(argument, ":"); len(split) == 2 {
			keyStr := split[1]
			if strings.HasPrefix(strings.ToLower(keyStr), hexPrefix) {
				return nil, fmt.Errorf("invalid creation input: '%s'", argument)
			} else {
				keyNr, err = strconv.ParseUint(keyStr, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid creation input: '%s': %w", argument, err)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, argument)
		}
		_, err := am.GetAccountKey(keyNr - 1)
		if err != nil {
			return nil, err
		}
		return &PredicateInput{AccountNumber: keyNr}, nil

	}
	if strings.HasPrefix(argument, hexPrefix) {
		decoded, err := DecodeHexOrEmpty(argument)
		if err != nil {
			return nil, err
		}
		if len(decoded) == 0 {
			decoded = script.PredicateArgumentEmpty()
		}
		return &PredicateInput{Argument: decoded}, nil
	}
	return nil, fmt.Errorf("invalid creation input: '%s'", argument)
}

func ParsePredicateClause(clause string, keyNr uint64, am account.Manager) ([]byte, error) {
	if len(clause) == 0 || clause == predicateTrue {
		return script.PredicateAlwaysTrue(), nil
	}
	if clause == predicateFalse {
		return script.PredicateAlwaysFalse(), nil
	}

	var err error
	if strings.HasPrefix(clause, predicatePtpkh) {
		if split := strings.Split(clause, ":"); len(split) == 2 {
			keyStr := split[1]
			if strings.HasPrefix(strings.ToLower(keyStr), hexPrefix) {
				if len(keyStr) < 3 {
					return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
				}
				keyHash, err := hexutil.Decode(keyStr)
				if err != nil {
					return nil, err
				}
				return script.PredicatePayToPublicKeyHashDefault(keyHash), nil
			} else {
				keyNr, err = strconv.ParseUint(keyStr, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid predicate clause: '%s': %w", clause, err)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, clause)
		}
		accountKey, err := am.GetAccountKey(keyNr - 1)
		if err != nil {
			return nil, err
		}
		return script.PredicatePayToPublicKeyHashDefault(accountKey.PubKeyHash.Sha256), nil

	}
	if strings.HasPrefix(clause, hexPrefix) {
		return DecodeHexOrEmpty(clause)
	}
	return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
}

func (c *CreateFungibleTokenTypeAttributes) toProtobuf() *tokens.CreateFungibleTokenTypeAttributes {
	var icon *tokens.Icon
	if c.Icon != nil {
		icon = &tokens.Icon{Type: c.Icon.Type, Data: c.Icon.Data}
	}
	return &tokens.CreateFungibleTokenTypeAttributes{
		Name:                     c.Name,
		Icon:                     icon,
		Symbol:                   c.Symbol,
		DecimalPlaces:            c.DecimalPlaces,
		ParentTypeId:             c.ParentTypeId,
		SubTypeCreationPredicate: c.SubTypeCreationPredicate,
		TokenCreationPredicate:   c.TokenCreationPredicate,
		InvariantPredicate:       c.InvariantPredicate,
	}
}

func (c *CreateNonFungibleTokenTypeAttributes) toProtobuf() *tokens.CreateNonFungibleTokenTypeAttributes {
	var icon *tokens.Icon
	if c.Icon != nil {
		icon = &tokens.Icon{Type: c.Icon.Type, Data: c.Icon.Data}
	}
	return &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                   c.Symbol,
		Name:                     c.Name,
		Icon:                     icon,
		ParentTypeId:             c.ParentTypeId,
		SubTypeCreationPredicate: c.SubTypeCreationPredicate,
		TokenCreationPredicate:   c.TokenCreationPredicate,
		InvariantPredicate:       c.InvariantPredicate,
		DataUpdatePredicate:      c.DataUpdatePredicate,
	}
}

func (a *MintNonFungibleTokenAttributes) toProtobuf() *tokens.MintNonFungibleTokenAttributes {
	return &tokens.MintNonFungibleTokenAttributes{
		Name:                a.Name,
		NftType:             a.NftType,
		Uri:                 a.Uri,
		Data:                a.Data,
		Bearer:              a.Bearer,
		DataUpdatePredicate: a.DataUpdatePredicate,
	}
}

func DecodeHexOrEmpty(input string) ([]byte, error) {
	if len(input) == 0 || input == predicateEmpty {
		return []byte{}, nil
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(input), hexPrefix))
	if err != nil {
		return nil, err
	}
	return decoded, nil
}
