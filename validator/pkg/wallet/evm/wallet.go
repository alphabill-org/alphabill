package evm

import (
	"context"
	"fmt"
	"math/big"
	"net/url"
	"strings"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/account"
	evmclient "github.com/alphabill-org/alphabill/validator/pkg/wallet/evm/client"
	"github.com/fxamacker/cbor/v2"
)

const txTimeoutBlockCount = 10

type (
	evmClient interface {
		Client
		Call(ctx context.Context, callAttr *evmclient.CallAttributes) (*evmclient.ProcessingDetails, error)
		GetTransactionCount(ctx context.Context, ethAddr []byte) (uint64, error)
		GetBalance(ctx context.Context, ethAddr []byte) (string, []byte, error)
	}
	Wallet struct {
		systemID []byte
		am       account.Manager
		restCli  evmClient
	}
)

func ConvertBalanceToAlpha(eth *big.Int) uint64 {
	return evmclient.WeiToAlpha(eth)
}

func New(systemID []byte, restUrl string, am account.Manager) (*Wallet, error) {
	if systemID == nil {
		return nil, fmt.Errorf("system id is nil")
	}
	if len(restUrl) == 0 {
		return nil, fmt.Errorf("rest url is empty")
	}
	if am == nil {
		return nil, fmt.Errorf("account manager is nil")
	}
	if !strings.HasPrefix(restUrl, "http://") && !strings.HasPrefix(restUrl, "https://") {
		restUrl = "http://" + restUrl
	}
	addr, err := url.Parse(restUrl)
	if err != nil {
		return nil, err
	}
	return &Wallet{
		systemID: systemID,
		am:       am,
		restCli:  evmclient.New(*addr),
	}, nil
}

func (w *Wallet) Shutdown() {
	w.am.Close()
}

func (w *Wallet) SendEvmTx(ctx context.Context, accNr uint64, attrs *evmclient.TxAttributes) (*evmclient.Result, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, fmt.Errorf("account key read failed: %w", err)
	}
	from, err := generateAddress(acc.PubKey)
	if err != nil {
		return nil, fmt.Errorf("from address generation failed: %w", err)
	}
	roundNumber, err := w.restCli.GetRoundNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("evm current round number read failed: %w", err)
	}
	// verify account exists and get transaction count
	nonce, err := w.restCli.GetTransactionCount(ctx, from.Bytes())
	if err != nil {
		return nil, fmt.Errorf("account %x transaction count read failed: %w", from.Bytes(), err)
	}
	attrs.From = from.Bytes()
	attrs.Nonce = nonce
	if attrs.Value == nil {
		attrs.Value = big.NewInt(0)
	}
	payload, err := newTxPayload(w.systemID, "evm", from.Bytes(), roundNumber+txTimeoutBlockCount, attrs)
	if err != nil {
		return nil, fmt.Errorf("evm transaction payload error: %w", err)
	}
	txo, err := signPayload(payload, acc)
	if err != nil {
		return nil, fmt.Errorf("transaction sign failed: %w", err)
	}
	// send transaction and wait for response or timeout
	txPub := NewTxPublisher(w.restCli)
	proof, err := txPub.SendTx(ctx, txo, nil)
	if err != nil {
		return nil, fmt.Errorf("evm transaction failed or account does not have enough fee credit: %w", err)
	}
	if proof == nil || proof.TxRecord == nil {
		return nil, fmt.Errorf("unexpected result")
	}
	var details evmclient.ProcessingDetails
	if err = proof.TxRecord.UnmarshalProcessingDetails(&details); err != nil {
		return nil, fmt.Errorf("failed to de-serialize evm execution result: %w", err)
	}
	return &evmclient.Result{
		Success:   proof.TxRecord.ServerMetadata.SuccessIndicator == types.TxStatusSuccessful,
		ActualFee: proof.TxRecord.ServerMetadata.GetActualFee(),
		Details:   &details,
	}, nil
}

func (w *Wallet) EvmCall(ctx context.Context, accNr uint64, attrs *evmclient.CallAttributes) (*evmclient.Result, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, fmt.Errorf("account key read failed: %w", err)
	}
	from, err := generateAddress(acc.PubKey)
	if err != nil {
		return nil, fmt.Errorf("generating address: %w", err)
	}
	attrs.From = from.Bytes()
	details, err := w.restCli.Call(ctx, attrs)
	if err != nil {
		return nil, err
	}
	return &evmclient.Result{
		Success:   len(details.ErrorDetails) == 0,
		ActualFee: 0,
		Details:   details,
	}, nil
}

func (w *Wallet) GetBalance(ctx context.Context, accNr uint64) (*big.Int, error) {
	if accNr < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accNr)
	}
	acc, err := w.am.GetAccountKey(accNr - 1)
	if err != nil {
		return nil, fmt.Errorf("account key read failed: %w", err)
	}
	from, err := generateAddress(acc.PubKey)
	if err != nil {
		return nil, fmt.Errorf("generating address: %w", err)
	}
	balanceStr, _, err := w.restCli.GetBalance(ctx, from.Bytes())
	balance, ok := new(big.Int).SetString(balanceStr, 10)
	if !ok {
		return nil, fmt.Errorf("balance string %s to base 10 conversion failed: %w", balanceStr, err)
	}
	return balance, nil
}

func newTxPayload(systemID []byte, txType string, unitID []byte, timeout uint64, attr interface{}) (*types.Payload, error) {
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	return &types.Payload{
		SystemID:   systemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout: timeout,
		},
	}, nil
}

func signPayload(payload *types.Payload, ac *account.AccountKey) (*types.TransactionOrder, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := payload.Bytes()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(payloadBytes)
	if err != nil {
		return nil, err
	}
	return &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: templates.NewP2pkh256SignatureBytes(sig, ac.PubKey),
	}, nil
}