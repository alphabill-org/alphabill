package wallet

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/pkg/client"
)

type Option func(w *Wallet)

func WithBlockProcessor(bp BlockProcessor) Option {
	return func(w *Wallet) {
		w.BlockProcessor = bp
	}
}

func WithAlphabillClient(abc client.ABClient) Option {
	return func(w *Wallet) {
		w.AlphabillClient = abc
	}
}

func WithAlphabillClientConfig(abcConf client.AlphabillClientConfig) Option {
	return func(w *Wallet) {
		w.AlphabillClient = client.New(abcConf)
	}
}

func WithTxVerifier(verifier TxVerifier) Option {
	return func(w *Wallet) {
		w.TxVerifier = verifier
	}
}

func WithTrustBase(tb map[string]crypto.Verifier) Option {
	return func(w *Wallet) {
		w.TxVerifier = &DefaultTxVerifier{trustBase: tb}
	}
}
