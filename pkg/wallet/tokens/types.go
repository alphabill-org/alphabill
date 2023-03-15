package tokens

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/protobuf/proto"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
)

const (
	txTimeoutRoundCount        = 100
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
		proto.Message
		SetInvariantPredicateSignatures([][]byte)
	}
)

func ParsePredicates(arguments []string, am account.Manager) ([]*PredicateInput, error) {
	creationInputs := make([]*PredicateInput, 0, len(arguments))
	for _, argument := range arguments {
		input, err := parsePredicate(argument, am)
		if err != nil {
			return nil, err
		}
		creationInputs = append(creationInputs, input)
	}
	return creationInputs, nil
}

// parsePredicate uses the following format:
// empty|true|false|empty produce an empty predicate argument
// ptpkh (key 1) or ptpkh:n (n > 0) produce an argument with the signed transaction by the given key
func parsePredicate(argument string, am account.Manager) (*PredicateInput, error) {
	if len(argument) == 0 || argument == predicateEmpty || argument == predicateTrue || argument == predicateFalse {
		return &PredicateInput{Argument: script.PredicateArgumentEmpty()}, nil
	}
	keyNr := 1
	var err error
	if strings.HasPrefix(argument, predicatePtpkh) {
		if split := strings.Split(argument, ":"); len(split) == 2 {
			keyStr := split[1]
			if strings.HasPrefix(strings.ToLower(keyStr), hexPrefix) {
				return nil, fmt.Errorf("invalid creation input: '%s'", argument)
			} else {
				keyNr, err = strconv.Atoi(keyStr)
				if err != nil {
					return nil, fmt.Errorf("invalid creation input: '%s': %w", argument, err)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, argument)
		}
		_, err := am.GetAccountKey(uint64(keyNr - 1))
		if err != nil {
			return nil, err
		}
		return &PredicateInput{AccountNumber: uint64(keyNr)}, nil

	}
	if strings.HasPrefix(argument, hexPrefix) {
		decoded, err := decodeHexOrEmpty(argument)
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

func ParsePredicateClause(clause string, am account.Manager) ([]byte, error) {
	if len(clause) == 0 || clause == predicateTrue {
		return script.PredicateAlwaysTrue(), nil
	}
	if clause == predicateFalse {
		return script.PredicateAlwaysFalse(), nil
	}

	keyNr := 1
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
				keyNr, err = strconv.Atoi(keyStr)
				if err != nil {
					return nil, fmt.Errorf("invalid predicate clause: '%s': %w", clause, err)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, clause)
		}
		accountKey, err := am.GetAccountKey(uint64(keyNr - 1))
		if err != nil {
			return nil, err
		}
		return script.PredicatePayToPublicKeyHashDefault(accountKey.PubKeyHash.Sha256), nil

	}
	if strings.HasPrefix(clause, hexPrefix) {
		return decodeHexOrEmpty(clause)
	}
	return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
}

func decodeHexOrEmpty(input string) ([]byte, error) {
	if len(input) == 0 || input == predicateEmpty {
		return []byte{}, nil
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(input), hexPrefix))
	if err != nil {
		return nil, err
	}
	return decoded, nil
}