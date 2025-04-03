package evm

import (
	"crypto"
	"fmt"
	"math/big"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
)

const DefaultBlockGasLimit = 15000000
const DefaultGasPrice = 210000000

type (
	Options struct {
		moneyPartitionID     types.PartitionID
		state                *state.State
		executedTransactions map[string]uint64
		hashAlgorithm        crypto.Hash
		trustBase            types.RootTrustBase
		blockGasLimit        uint64
		gasUnitPrice         *big.Int
		blockDB              keyvaluedb.KeyValueDB
		execPredicate        predicates.PredicateExecutor
	}

	Option func(*Options)
)

func defaultOptions(obs Observability) (*Options, error) {
	templEngine, err := templates.New(obs)
	if err != nil {
		return nil, fmt.Errorf("creating predicate templates executor: %w", err)
	}
	predEng, err := predicates.Dispatcher(templEngine)
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}

	return &Options{
		moneyPartitionID: 1,
		hashAlgorithm:    crypto.SHA256,
		trustBase:        nil,
		blockGasLimit:    DefaultBlockGasLimit,
		gasUnitPrice:     big.NewInt(DefaultGasPrice),
		execPredicate:    predEng.Execute,
	}, nil
}

func WithBlockDB(blockDB keyvaluedb.KeyValueDB) Option {
	return func(c *Options) {
		c.blockDB = blockDB
	}
}

func WithState(s *state.State) Option {
	return func(c *Options) {
		c.state = s
	}
}

func WithExecutedTransactions(executedTransactions map[string]uint64) Option {
	return func(c *Options) {
		c.executedTransactions = executedTransactions
	}
}

func WithHashAlgorithm(algorithm crypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}

func WithTrustBase(tb types.RootTrustBase) Option {
	return func(c *Options) {
		c.trustBase = tb
	}
}

func WithMoneyPartitionID(moneyPartitionID types.PartitionID) Option {
	return func(o *Options) {
		o.moneyPartitionID = moneyPartitionID
	}
}

func WithGasPrice(gasPrice uint64) Option {
	return func(o *Options) {
		// todo: conversion problem uint64 -> int64, make sure that argument over int64 max is not provided
		o.gasUnitPrice = big.NewInt(int64(gasPrice))
	}
}

func WithBlockGasLimit(limit uint64) Option {
	return func(o *Options) {
		o.blockGasLimit = limit
	}
}
