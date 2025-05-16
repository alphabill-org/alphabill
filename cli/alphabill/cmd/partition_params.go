package cmd

import (
	"fmt"
	"strconv"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

const (
	moneyInitialBillValue          = "initialBillValue"
	moneyInitialBillOwnerPredicate = "initialBillOwnerPredicate"
	moneyDCMoneySupplyValue        = "dcMoneySupplyValue"

	tokensAdminOwnerPredicate = "adminOwnerPredicate"
	tokensFeelessMode         = "feeless-mode"

	orchestrationOwnerPredicate = "ownerPredicate"
)

type MoneyPartitionParams struct {
	InitialBillValue          uint64
	InitialBillOwnerPredicate types.PredicateBytes
	DCMoneySupplyValue        uint64 // The initial value for Dust Collector money supply. Total money supply is initial bill + DC money supply.
}

type OrchestrationPartitionParams struct {
	OwnerPredicate types.PredicateBytes // the Proof-of-Authority owner predicate
}

type TokensPartitionParams struct {
	AdminOwnerPredicate types.PredicateBytes // the admin owner predicate for permissioned mode
	FeelessMode         bool                 // if true then fees are not charged (applies only in permissioned mode)
}

func ParseMoneyPartitionParams(shardConf *types.PartitionDescriptionRecord) (*MoneyPartitionParams, error) {
	var params MoneyPartitionParams
	for key, valueStr := range shardConf.PartitionParams {
		switch key {
		case moneyInitialBillValue:
			parsedValue, err := parseUint64(key, valueStr)
			if err != nil {
				return nil, err
			}
			params.InitialBillValue = parsedValue
		case moneyInitialBillOwnerPredicate:
			value, err := hex.Decode([]byte(valueStr))
			if err != nil {
				return nil, fmt.Errorf("failed to parse param %q value: %w", key, err)
			}
			params.InitialBillOwnerPredicate = value
		case moneyDCMoneySupplyValue:
			parsedValue, err := parseUint64(key, valueStr)
			if err != nil {
				return nil, err
			}
			params.DCMoneySupplyValue = parsedValue
		default:
			return nil, fmt.Errorf("unexpected partition param: %s", key)
		}
	}
	return &params, nil
}

func ParseOrchestrationPartitionParams(shardConf *types.PartitionDescriptionRecord) (*OrchestrationPartitionParams, error) {
	var params OrchestrationPartitionParams
	for key, valueStr := range shardConf.PartitionParams {
		switch key {
		case orchestrationOwnerPredicate:
			value, err := hex.Decode([]byte(valueStr))
			if err != nil {
				return nil, fmt.Errorf("failed to parse param %q value: %w", key, err)
			}
			params.OwnerPredicate = value
		default:
			return nil, fmt.Errorf("unexpected partition param: %s", key)
		}
	}
	return &params, nil
}

func ParseTokensPartitionParams(shardConf *types.PartitionDescriptionRecord) (*TokensPartitionParams, error) {
	var params TokensPartitionParams
	for key, valueStr := range shardConf.PartitionParams {
		switch key {
		case tokensAdminOwnerPredicate:
			{
				value, err := hex.Decode([]byte(valueStr))
				if err != nil {
					return nil, fmt.Errorf("failed to parse param %q value: %w", key, err)
				}
				params.AdminOwnerPredicate = value
			}
		case tokensFeelessMode:
			{
				value, err := strconv.ParseBool(valueStr)
				if err != nil {
					return nil, fmt.Errorf("failed to parse param %q value: %w", key, err)
				}
				params.FeelessMode = value
			}
		default:
			return nil, fmt.Errorf("unexpected partition param: %s", key)
		}
	}
	return &params, nil
}

func parseUint64(key, value string) (uint64, error) {
	ret, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse param %q value: %w", key, err)
	}
	return ret, nil
}
