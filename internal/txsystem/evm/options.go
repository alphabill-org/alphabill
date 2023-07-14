package evm

import (
	gocrypto "crypto"
	"math/big"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
)

const DefaultBlockGasLimit = 15000000
const DefaultGasPrice = 210000000

var DefaultEvmTxSystemIdentifier = []byte{0, 0, 0, 3}

type (
	Options struct {
		moneyTXSystemIdentifier []byte
		state                   *rma.Tree
		hashAlgorithm           gocrypto.Hash
		trustBase               map[string]crypto.Verifier
		initialAccountAddress   []byte
		initialAccountBalance   *big.Int
		blockGasLimit           uint64
		gasUnitPrice            *big.Int
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		moneyTXSystemIdentifier: []byte{0, 0, 0, 0},
		state:                   rma.NewWithSHA256(),
		hashAlgorithm:           gocrypto.SHA256,
		trustBase:               nil,
		initialAccountAddress:   make([]byte, 20),
		initialAccountBalance:   big.NewInt(1000000000000000000), //1-ETH
		blockGasLimit:           DefaultBlockGasLimit,
		gasUnitPrice:            big.NewInt(DefaultGasPrice),
	}
}

func WithState(state *rma.Tree) Option {
	return func(c *Options) {
		c.state = state
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

func WithInitialAddressAndBalance(address []byte, balance *big.Int) Option {
	return func(o *Options) {
		o.initialAccountAddress = address
		o.initialAccountBalance = balance
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
