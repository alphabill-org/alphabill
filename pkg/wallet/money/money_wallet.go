package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	abclient "github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	backendmoney "github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/sync/errgroup"
)

const (
	dcTimeoutBlockCount       = 10
	swapTimeoutBlockCount     = 60
	txTimeoutBlockCount       = 10
	maxBillsForDustCollection = 100
	dustBillDeletionTimeout   = 65536
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance for transaction")
	ErrInvalidPubKey       = errors.New("invalid public key, public key must be in compressed secp256k1 format")
	ErrTxFailedToConfirm   = errors.New("transaction(s) failed to confirm")

	ErrNoFeeCredit                  = errors.New("no fee credit in money wallet")
	ErrInsufficientFeeCredit        = errors.New("insufficient fee credit balance for transaction(s)")
	ErrInvalidCreateFeeCreditAmount = errors.New("fee credit amount must be positive")
)

var (
	txBufferFullErrMsg = "tx buffer is full"
	maxTxFailedTries   = 3
)

type (
	Wallet struct {
		*wallet.Wallet

		dcWg        *dcWaitGroup
		am          account.Manager
		backend     BackendAPI
		feeManager  *fees.FeeManager
		TxPublisher *TxPublisher
	}

	BackendAPI interface {
		GetBalance(pubKey []byte, includeDCBills bool) (uint64, error)
		ListBills(pubKey []byte, includeDCBills bool) (*backendmoney.ListBillsResponse, error)
		GetBills(pubKey []byte) ([]*wallet.Bill, error)
		GetProof(billId []byte) (*wallet.Bills, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		FetchFeeCreditBill(ctx context.Context, unitID []byte) (*wallet.Bill, error)
	}

	SendCmd struct {
		ReceiverPubKey      []byte
		Amount              uint64
		WaitForConfirmation bool
		AccountIndex        uint64
	}

	GetBalanceCmd struct {
		AccountIndex uint64
		CountDCBills bool
	}

	AddFeeCmd struct {
		Amount       uint64
		AccountIndex uint64
	}

	ReclaimFeeCmd struct {
		AccountIndex uint64
	}
)

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
// If mnemonic seed is empty then new mnemonic will ge generated, otherwise wallet is restored using given mnemonic.
func CreateNewWallet(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

func LoadExistingWallet(config abclient.AlphabillClientConfig, am account.Manager, backend BackendAPI) (*Wallet, error) {
	genericWallet := wallet.New().
		SetABClientConf(config).
		Build()
	moneySystemID := []byte{0, 0, 0, 0}
	moneyTxPublisher := NewTxPublisher(genericWallet, backend)
	feeManager := fees.NewFeeManager(am, moneySystemID, moneyTxPublisher, backend, moneySystemID, moneyTxPublisher, backend)
	return &Wallet{
		Wallet:      genericWallet,
		am:          am,
		backend:     backend,
		dcWg:        newDcWaitGroup(),
		TxPublisher: moneyTxPublisher,
		feeManager:  feeManager,
	}, nil
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) SystemID() []byte {
	// TODO: return the default "AlphaBill Money System ID" for now
	// but this should come from config (base wallet? AB client?)
	return []byte{0, 0, 0, 0}
}

// Shutdown terminates connection to alphabill node, closes account manager and cancels any background goroutines.
func (w *Wallet) Shutdown() {
	w.Wallet.Shutdown()
	w.am.Close()
	if w.dcWg != nil {
		w.dcWg.ResetWaitGroup()
	}
}

// CollectDust starts the dust collector process for all accounts in the wallet.
// Wallet needs to be synchronizing using Sync or SyncToMaxBlockNumber in order to receive transactions and finish the process.
// The function blocks until dust collector process is finished or timed out. Skips account if the account already has only one or no bills.
func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64) error {
	errgrp, ctx := errgroup.WithContext(ctx)
	if accountNumber == 0 {
		for _, acc := range w.am.GetAll() {
			accIndex := acc.AccountIndex // copy value for closure
			errgrp.Go(func() error {
				return w.collectDust(ctx, true, accIndex)
			})
		}
	} else {
		errgrp.Go(func() error {
			return w.collectDust(ctx, true, accountNumber-1)
		})
	}
	return errgrp.Wait()
}

// GetBalance returns sum value of all bills currently owned by the wallet, for given account.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalance(cmd GetBalanceCmd) (uint64, error) {
	pubKey, err := w.am.GetPublicKey(cmd.AccountIndex)
	if err != nil {
		return 0, err
	}
	return w.backend.GetBalance(pubKey, cmd.CountDCBills)
}

// GetBalances returns sum value of all bills currently owned by the wallet, for all accounts.
// The value returned is the smallest denomination of alphabills.
func (w *Wallet) GetBalances(cmd GetBalanceCmd) ([]uint64, uint64, error) {
	pubKeys, err := w.am.GetPublicKeys()
	totals := make([]uint64, len(pubKeys))
	sum := uint64(0)
	for accountIndex, pubKey := range pubKeys {
		balance, err := w.backend.GetBalance(pubKey, cmd.CountDCBills)
		if err != nil {
			return nil, 0, err
		}
		sum += balance
		totals[accountIndex] = balance
	}
	return totals, sum, err
}

// Send creates, signs and broadcasts transactions, in total for the given amount,
// to the given public key, the public key must be in compressed secp256k1 format.
// Sends one transaction per bill, prioritizing larger bills.
// Waits for initial response from the node, returns error if any transaction was not accepted to the mempool.
// Returns list of bills including transaction and proof data, if waitForConfirmation=true, otherwise nil.
func (w *Wallet) Send(ctx context.Context, cmd SendCmd) ([]*Bill, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	pubKey, _ := w.am.GetPublicKey(cmd.AccountIndex)
	balance, err := w.backend.GetBalance(pubKey, true)
	if err != nil {
		return nil, err
	}
	if cmd.Amount > balance {
		return nil, ErrInsufficientBalance
	}

	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	fcb, err := w.GetFeeCreditBill(ctx, fees.GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}
	if fcb == nil {
		return nil, ErrNoFeeCredit
	}

	bills, err := w.backend.GetBills(pubKey)
	if err != nil {
		return nil, err
	}

	timeout := roundNumber + txTimeoutBlockCount
	apiWrapper := &backendAPIWrapper{wallet: w}
	batch := txsubmitter.NewBatch(k.PubKey, apiWrapper)

	txs, err := tx_builder.CreateTransactions(cmd.ReceiverPubKey, cmd.Amount, w.SystemID(), bills, k, timeout, fcb.Id)
	if err != nil {
		return nil, err
	}
	for _, tx := range txs {
		// TODO should not rely on server metadata
		// gtx.SetServerMetadata(&txsystem.ServerMetadata{Fee: 1})
		batch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}

	txsCost := tx_builder.MaxFee * uint64(len(batch.Submissions()))
	if fcb.Value < txsCost {
		return nil, ErrInsufficientFeeCredit
	}

	if err = batch.SendTx(ctx, cmd.WaitForConfirmation); err != nil {
		return nil, err
	}
	return apiWrapper.txProofs, nil
}

type backendAPIWrapper struct {
	wallet   *Wallet
	txProofs []*Bill
}

func (b *backendAPIWrapper) GetRoundNumber(ctx context.Context) (uint64, error) {
	return b.wallet.backend.GetRoundNumber(ctx)
}

func (b *backendAPIWrapper) PostTransactions(ctx context.Context, _ wallet.PubKey, txs *wallet.Transactions) error {
	for _, tx := range txs.Transactions {
		err := b.wallet.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *backendAPIWrapper) GetTxProof(_ context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	resp, err := b.wallet.backend.GetProof(unitID)
	if err != nil {
		return nil, err
	}
	if len(resp.Bills) != 1 {
		return nil, errors.New(fmt.Sprintf("unexpected number of proofs: %d, bill ID: %X", len(resp.Bills), unitID))
	}
	bill := resp.Bills[0]
	if !bytes.Equal(bill.TxHash, txHash) {
		// confirmation expects nil (not error) if there's no proof for the given tx hash (yet)
		return nil, nil
	}
	b.txProofs = append(b.txProofs, convertBill(bill))
	return bill.TxProof, nil
}

// AddFeeCredit creates fee credit for the given amount.
// Wallet must have a bill large enough for the required amount plus fees.
// Returns transferFC and addFC transaction proofs.
func (w *Wallet) AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) ([]*types.TxProof, error) {
	return w.feeManager.AddFeeCredit(ctx, cmd)
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns closeFC and reclaimFC transaction proofs.
func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) ([]*types.TxProof, error) {
	return w.feeManager.ReclaimFeeCredit(ctx, cmd)
}

// GetFeeCreditBill returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(ctx context.Context, cmd fees.GetFeeCreditCmd) (*wallet.Bill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.FetchFeeCreditBill(ctx, accountKey.PrivKeyHash)
}

// FetchFeeCreditBill returns fee credit bill for given unitID
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) FetchFeeCreditBill(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
	fcb, err := w.backend.FetchFeeCreditBill(ctx, unitID)
	if err != nil {
		if errors.Is(err, client.ErrMissingFeeCreditBill) {
			return nil, nil
		}
		return nil, err
	}
	return fcb, nil
}

// collectDust sends dust transfer for every bill for given account in wallet and records metadata.
// Returns immediately without error if there's already 1 or 0 bills.
// Once the dust transfers get confirmed on the ledger then swap transfer is broadcast and metadata cleared.
// If blocking is true then the function blocks until swap has been completed or timed out,
// if blocking is false then the function returns after sending the dc transfers.
func (w *Wallet) collectDust(ctx context.Context, blocking bool, accountIndex uint64) error {
	log.Info("starting dust collection for account=", accountIndex, " blocking=", blocking)
	roundNr, err := w.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	pubKey, err := w.am.GetPublicKey(accountIndex)
	if err != nil {
		return err
	}
	billResponse, err := w.backend.ListBills(pubKey, true)
	if err != nil {
		return err
	}
	if len(billResponse.Bills) < 2 {
		log.Info("Account ", accountIndex, " has less than 2 bills, skipping dust collection")
		return nil
	}
	if len(billResponse.Bills) > maxBillsForDustCollection {
		log.Info("Account ", accountIndex, " has more than ", maxBillsForDustCollection, " bills, dust collection will take only the first ", maxBillsForDustCollection, " bills")
		billResponse.Bills = billResponse.Bills[0:maxBillsForDustCollection]
	}
	var bills []*Bill
	for _, b := range billResponse.Bills {
		proof, err := w.backend.GetProof(b.Id)
		if err != nil {
			return err
		}
		bills = append(bills, convertBill(proof.Bills[0]))
	}
	var expectedSwaps []expectedSwap
	dcBillGroups := groupDcBills(bills)
	if len(dcBillGroups) > 0 {
		for _, v := range dcBillGroups {
			if roundNr >= v.dcTimeout {
				swapTimeout := roundNr + swapTimeoutBlockCount
				billIds := getBillIds(v.dcBills)
				err = w.swapDcBills(ctx, v.dcBills, v.dcNonce, billIds, swapTimeout, accountIndex)
				if err != nil {
					return err
				}
				expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: v.dcNonce, timeout: swapTimeout, dcSum: v.valueSum})
				w.dcWg.AddExpectedSwaps(expectedSwaps)
			} else {
				// expecting to receive swap during dcTimeout
				expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: v.dcNonce, timeout: v.dcTimeout, dcSum: v.valueSum})
				w.dcWg.AddExpectedSwaps(expectedSwaps)
				err = w.doSwap(ctx, accountIndex, v.dcTimeout)
				if err != nil {
					return err
				}
			}
		}
	} else {
		k, err := w.am.GetAccountKey(accountIndex)
		if err != nil {
			return err
		}

		dcTimeout := roundNr + dcTimeoutBlockCount
		dcNonce := calculateDcNonce(bills)
		var dcValueSum uint64
		for _, b := range bills {
			dcValueSum += b.Value
			tx, err := tx_builder.NewDustTx(k, w.SystemID(), &wallet.Bill{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}, dcNonce, dcTimeout)
			if err != nil {
				return err
			}
			log.Info("sending dust transfer tx for bill=", b.Id, " account=", accountIndex)
			err = w.SendTransaction(ctx, tx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
			if err != nil {
				return err
			}
		}
		expectedSwaps = append(expectedSwaps, expectedSwap{dcNonce: dcNonce, timeout: dcTimeout, dcSum: dcValueSum})
		w.dcWg.AddExpectedSwaps(expectedSwaps)

		if err = w.doSwap(ctx, accountIndex, dcTimeout); err != nil {
			return err
		}
	}

	if blocking {
		if err = w.confirmSwap(ctx); err != nil {
			log.Error("failed to confirm swap tx", err)
			return err
		}
	}

	log.Info("finished waiting for blocking collect dust on account=", accountIndex)

	return nil
}

func (w *Wallet) doSwap(ctx context.Context, accountIndex, timeout uint64) error {
	roundNr, err := w.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	pubKey, err := w.am.GetPublicKey(accountIndex)
	if err != nil {
		return err
	}
	log.Info("waiting for swap confirmation(s)...")
	for roundNr <= timeout+1 {
		billResponse, err := w.backend.ListBills(pubKey, true)
		if err != nil {
			return err
		}
		var bills []*Bill
		for _, b := range billResponse.Bills {
			proof, err := w.backend.GetProof(b.Id)
			if err != nil {
				return err
			}
			bills = append(bills, convertBill(proof.Bills[0]))
		}
		dcBillGroups := groupDcBills(bills)
		if len(dcBillGroups) > 0 {
			for k, v := range dcBillGroups {
				s := w.dcWg.getExpectedSwap(k)
				if s.dcNonce != nil && roundNr >= v.dcTimeout || v.valueSum >= s.dcSum {
					swapTimeout := roundNr + swapTimeoutBlockCount
					billIds := getBillIds(v.dcBills)
					err = w.swapDcBills(ctx, v.dcBills, v.dcNonce, billIds, swapTimeout, accountIndex)
					if err != nil {
						return err
					}
					w.dcWg.UpdateTimeout(v.dcNonce, swapTimeout)
					return nil
				}
			}
		}
		select {
		case <-time.After(500 * time.Millisecond):
			roundNr, err = w.GetRoundNumber(ctx)
			if err != nil {
				return err
			}
			continue
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

func (w *Wallet) confirmSwap(ctx context.Context) error {
	roundNr, err := w.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	log.Info("waiting for swap confirmation(s)...")
	swapTimeout := roundNr + swapTimeoutBlockCount
	for roundNr <= swapTimeout {
		if len(w.dcWg.swaps) == 0 {
			return nil
		}
		blockBytes, err := w.AlphabillClient.GetBlock(ctx, roundNr)
		if err != nil {
			return fmt.Errorf("failed to download block: %w", err)
		}
		block := &types.Block{}
		if err := cbor.Unmarshal(blockBytes, block); err != nil {
			return fmt.Errorf("failed to unmarshal block: %w", err)
		}
		for _, tx := range block.Transactions {
			if err := w.dcWg.DecrementSwaps(string(tx.TransactionOrder.UnitID())); err != nil {
				return err
			}
		}
		if len(w.dcWg.swaps) == 0 {
			return nil
		}
		select {
		// wait for some time before retrying to fetch new block
		case <-time.After(500 * time.Millisecond):
			roundNr, err = w.GetRoundNumber(ctx)
			if err != nil {
				return err
			}
			continue
		case <-ctx.Done():
			return nil
		}
	}
	w.dcWg.ResetWaitGroup()

	return nil
}

func (w *Wallet) swapDcBills(ctx context.Context, dcBills []*Bill, dcNonce []byte, billIds [][]byte, timeout uint64, accountIndex uint64) error {
	k, err := w.am.GetAccountKey(accountIndex)
	if err != nil {
		return err
	}
	fcb, err := w.GetFeeCreditBill(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
	if err != nil {
		return err
	}
	if fcb.GetValue() < tx_builder.MaxFee {
		return ErrInsufficientFeeCredit
	}

	var bpBills []*wallet.Bill
	for _, b := range dcBills {
		bpBills = append(bpBills, &wallet.Bill{Id: b.GetID(), Value: b.Value, TxProof: b.TxProof})
	}

	swapTx, err := tx_builder.NewSwapTx(k, w.SystemID(), bpBills, dcNonce, billIds, timeout)
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("sending swap tx: nonce=%s timeout=%d", hexutil.Encode(dcNonce), timeout))
	err = w.SendTransaction(context.Background(), swapTx, &wallet.SendOpts{RetryOnFullTxBuffer: true})
	if err != nil {
		return err
	}
	return nil
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *Wallet) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*types.TxProof, error) {
	return w.TxPublisher.SendTx(ctx, tx, senderPubKey)
}

func (c *SendCmd) isValid() error {
	if len(c.ReceiverPubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return ErrInvalidPubKey
	}
	return nil
}

func (c *AddFeeCmd) isValid() error {
	if c.Amount == 0 {
		return ErrInvalidCreateFeeCreditAmount
	}
	return nil
}

func createMoneyWallet(mnemonic string, am account.Manager) error {
	// load accounts from account manager
	accountKeys, err := am.GetAccountKeys()
	if err != nil {
		return fmt.Errorf("failed to check does account have any keys: %w", err)
	}
	// create keys in account manager if not exists
	if len(accountKeys) == 0 {
		// creating keys also adds the first account
		if err = am.CreateKeys(mnemonic); err != nil {
			return fmt.Errorf("failed to create keys for the account: %w", err)
		}
		// reload accounts after adding the first account
		accountKeys, err = am.GetAccountKeys()
		if err != nil {
			return fmt.Errorf("failed to read account keys: %w", err)
		}
		if len(accountKeys) == 0 {
			return errors.New("failed to create key for the first account")
		}
	}

	return nil
}

func calculateDcNonce(bills []*Bill) []byte {
	var billIds [][]byte
	for _, b := range bills {
		billIds = append(billIds, b.GetID())
	}

	// sort billIds in ascending order
	sort.Slice(billIds, func(i, j int) bool {
		return bytes.Compare(billIds[i], billIds[j]) < 0
	})

	hasher := crypto.SHA256.New()
	for _, billId := range billIds {
		hasher.Write(billId)
	}
	return hasher.Sum(nil)
}

// groupDcBills groups bills together by dc nonce
func groupDcBills(bills []*Bill) map[string]*dcBillGroup {
	m := map[string]*dcBillGroup{}
	for _, b := range bills {
		if b.IsDcBill {
			k := string(b.DcNonce)
			billContainer, exists := m[k]
			if !exists {
				billContainer = &dcBillGroup{}
				m[k] = billContainer
			}
			billContainer.valueSum += b.Value
			billContainer.dcBills = append(billContainer.dcBills, b)
			billContainer.dcNonce = b.DcNonce
			billContainer.dcTimeout = b.DcTimeout
		}
	}
	return m
}

func getBillIds(bills []*Bill) [][]byte {
	var billIds [][]byte
	for _, b := range bills {
		billIds = append(billIds, b.GetID())
	}
	return billIds
}

// TODO unused functions?
//// newBillFromVM converts ListBillVM to Bill structs
//func newBillFromVM(b *backendmoney.ListBillVM) *bp.Bill {
//	return &bp.Bill{
//		Id:       b.Id,
//		Value:    b.Value,
//		TxHash:   b.TxHash,
//		IsDcBill: b.IsDCBill,
//	}
//}
//
//// newBill creates new Bill struct from given BlockProof for Transfer and Split transactions.
//func newBill(proof *types.TxProof) (*Bill, error) {
//	blockProof, err := NewBlockProof(proof.TxRecord, proof.TxProof, proof.BlockNumber)
//	if err != nil {
//		return nil, err
//	}
//	switch tx := gtx.(type) {
//	case money.Transfer:
//		return &Bill{
//			Id:         tx.UnitID(),
//			Value:      tx.TargetValue(),
//			TxHash:     tx.Hash(crypto.SHA256),
//			BlockProof: blockProof,
//		}, nil
//	case money.Split:
//		return &Bill{
//			Id:         txutil.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
//			Value:      tx.Amount(),
//			TxHash:     tx.Hash(crypto.SHA256),
//			BlockProof: blockProof,
//		}, nil
//	default:
//		return nil, errors.New("cannot convert unsupported tx type to Bill struct")
//	}
//}
//
//func convertBills(billsList []*backendmoney.ListBillVM) ([]*bp.Bill, error) {
//	var bills []*bp.Bill
//	for _, b := range billsList {
//		bill := newBillFromVM(b)
//		bills = append(bills, bill)
//	}
//	return bills, nil
//}

// converts proto bp.Bill to money.Bill domain struct
func convertBill(b *wallet.Bill) *Bill {
	if b.IsDcBill {
		attrs := &money.TransferDCAttributes{}
		if err := b.TxProof.TxRecord.TransactionOrder.UnmarshalAttributes(attrs); err != nil {
			return nil
		}
		return &Bill{
			Id:        util.BytesToUint256(b.Id),
			Value:     b.Value,
			TxHash:    b.TxHash,
			IsDcBill:  b.IsDcBill,
			DcNonce:   attrs.Nonce,
			DcTimeout: b.TxProof.TxRecord.TransactionOrder.Timeout(),
			TxProof:   b.TxProof,
		}
	}
	return &Bill{
		Id:       util.BytesToUint256(b.Id),
		Value:    b.Value,
		TxHash:   b.TxHash,
		IsDcBill: b.IsDcBill,
		TxProof:  b.TxProof,
	}
}
