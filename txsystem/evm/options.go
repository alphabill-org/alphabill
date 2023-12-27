package evm

import (
	gocrypto "crypto"
	"math/big"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/state"
)

const DefaultBlockGasLimit = 15000000
const DefaultGasPrice = 210000000

var DefaultEvmTxSystemIdentifier = []byte{0, 0, 0, 3}

type (
	Options struct {
		moneyTXSystemIdentifier []byte
		state                   *state.State
		hashAlgorithm           gocrypto.Hash
		trustBase               map[string]crypto.Verifier
		blockGasLimit           uint64
		gasUnitPrice            *big.Int
		blockDB                 keyvaluedb.KeyValueDB
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		moneyTXSystemIdentifier: []byte{0, 0, 0, 0},
		hashAlgorithm:           gocrypto.SHA256,
		trustBase:               nil,
		blockGasLimit:           DefaultBlockGasLimit,
		gasUnitPrice:            big.NewInt(DefaultGasPrice),
	}
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

func WithHashAlgorithm(algorithm gocrypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}

func WithTrustBase(tb map[string]crypto.Verifier) Option {
	return func(c *Options) {
		c.trustBase = tb
	}
}

func WithMoneyTXSystemIdentifier(moneyTxSystemID []byte) Option {
	return func(o *Options) {
		o.moneyTXSystemIdentifier = moneyTxSystemID
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
