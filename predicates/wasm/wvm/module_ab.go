package wvm

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
)

/*
AB functions to verify objects etc
*/
func addAlphabillModule(ctx context.Context, rt wazero.Runtime, _ Observability) error {
	_, err := rt.NewHostModuleBuilder("ab").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(digestSHA256), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("digest_sha256").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(amountTransferred), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("amount_transferred").
		NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(txSignedByPKH), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}).Export("tx_signed_by_pkh").
		Instantiate(ctx)
	return err
}

/*
txSignedByPKH attempts to verify transaction's OwnerProof against
P2PKH predicate.

Arguments (stack):
  - 0: handle of the tx order;
  - 1: handle of the PKH
  - 2: handle of the OwnerProof;

Returns:
  - 0: true, ie the txOrder was signed by the PubKey hash;
  - 1: false, ie the OwnerProof and PKH appear to be valid arguments for a
    P2PKH but the predicate evaluates to false;
  - 2: error, most likely the OwnerProof or PKH is not valid argument for P2PKH;
  - 3: error, argument stack[0] is not valid tx handle;
  - 4: error, failure to get PKH;
  - 5: error, failure to get owner proof;
  - 6: error, PKH is nil;
  - 7: error, owner proof is nil;
*/
func txSignedByPKH(ctx context.Context, mod api.Module, stack []uint64) {
	vec := extractVMContext(ctx)

	txo, err := getVar[*types.TransactionOrder](vec.curPrg.vars, api.DecodeU32(stack[0]))
	if err != nil {
		vec.log.DebugContext(ctx, "argument is not valid tx order handle", logger.Error(err))
		stack[0] = 3
		return
	}

	pkh, err := vec.getBytesVariable(api.DecodeU32(stack[1]))
	if err != nil {
		vec.log.DebugContext(ctx, "reading pkh", logger.Error(err))
		stack[0] = 4
		return
	}
	if pkh == nil {
		stack[0] = 6
		return
	}

	proof, err := vec.getBytesVariable(api.DecodeU32(stack[2]))
	if err != nil {
		vec.log.DebugContext(ctx, "extracting owner proof", logger.Error(err))
		stack[0] = 5
		return
	}
	if proof == nil {
		stack[0] = 7
		return
	}

	predicate := templates.NewP2pkh256BytesFromKeyHash(pkh)
	env := txoEvalCtx{EvalEnvironment: vec.curPrg.env, exArgument: txo.AuthProofSigBytes}
	switch ok, err := vec.engines(ctx, predicate, proof, txo, env); {
	case err != nil:
		vec.log.DebugContext(ctx, "failed to verify OwnerProof against p2pkh", logger.Error(err))
		stack[0] = 2
	case ok:
		stack[0] = 0
	default:
		stack[0] = 1
	}
}

func digestSHA256(vec *vmContext, mod api.Module, stack []uint64) error {
	data := read(mod, stack[0])
	h := sha256.Sum256(data)
	addr, err := vec.writeToMemory(mod, h[:])
	if err != nil {
		return fmt.Errorf("allocating memory for digest result: %w", err)
	}
	stack[0] = addr
	return nil
}

/*
Given raw BLOB of transaction proofs return amount on "money" transferred to
given receiver, optionally matching reference number too.
Arguments in "stack":

  - [0] txProofs: handle to raw CBOR containing array of tx record proofs,
    produced by ie CLI wallet save proof flag;
  - [1] receiver_pkh: handle of PubKey hash to which the money has been transferred to;
  - [2] ref_no: address (if given, ie not zero) of the reference number transfer(s)
    must have in order to be counted;
*/
func amountTransferred(vec *vmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(api.DecodeU32(stack[0]))
	if err != nil {
		return fmt.Errorf("reading input data: %w", err)
	}
	var txs []*types.TxRecordProof
	if err := types.Cbor.Unmarshal(data, &txs); err != nil {
		return fmt.Errorf("decoding data as slice of tx proofs: %w", err)
	}

	pkh, err := vec.getBytesVariable(api.DecodeU32(stack[1]))
	if err != nil {
		return fmt.Errorf("reading input PKH: %w", err)
	}

	var refNo []byte
	if addrRefNo := stack[2]; addrRefNo != 0 {
		refNo = read(mod, addrRefNo)
	}

	sum, err := amountTransferredSum(txs, pkh, refNo, trustBaseLoader(vec.curPrg.env.TrustBase))
	if err != nil {
		vec.log.Debug(fmt.Sprintf("AmountTransferredSum(%d) returned %d and some error(s): %v", len(txs), sum, err))
	}
	stack[0] = sum
	return nil
}

/*
The error return value is "for diagnostic" purposes, ie the func might return non-zero sum and non-nil error - in that
case the error describes reason why some transaction(s) were not counted. The ref number or receiver not matching are
not included in errors, only failures to determine whether the tx possibly could have been contributing to the sum...
*/
func amountTransferredSum(proofs []*types.TxRecordProof, receiverPKH []byte, refNo []byte, getTrustBase func(*types.TxProof) (types.RootTrustBase, error)) (uint64, error) {
	var total uint64
	var rErr error
	for i, v := range proofs {
		trustBase, err := getTrustBase(v.TxProof)
		if err != nil {
			rErr = errors.Join(rErr, fmt.Errorf("record[%d]: acquiring trust base: %w", i, err))
			continue
		}
		sum, err := transferredSum(trustBase, v, receiverPKH, refNo)
		if err != nil {
			rErr = errors.Join(rErr, fmt.Errorf("record[%d]: %w", i, err))
		}
		total += sum
	}
	return total, rErr
}

/*
transferredSum returns the sum transferred to the "receiverPKH" by given transaction
record.
Arguments:
  - receiverPKH: public key hash of the recipient - currently only p2pkh template is
    supported as owner predicate;
  - refNo: reference number of the transaction, if nil then ignored, otherwise must match
    (use not nil zero length slice to get sum of transfers without reference number);

unknown / invalid transactions are ignored (not error)?
*/
func transferredSum(trustBase types.RootTrustBase, txRecordProof *types.TxRecordProof, receiverPKH []byte, refNo []byte) (uint64, error) {
	if err := txRecordProof.IsValid(); err != nil {
		return 0, fmt.Errorf("invalid input: %w", err)
	}
	if trustBase == nil {
		return 0, errors.New("invalid input: trustbase is unassigned")
	}
	txo, err := txRecordProof.GetTransactionOrderV1()
	if err != nil {
		return 0, fmt.Errorf("decoding transaction order: %w", err)
	}
	if txo.PartitionID != money.DefaultPartitionID {
		return 0, fmt.Errorf("expected partition id %d got %d", money.DefaultPartitionID, txo.PartitionID)
	}
	if refNo != nil && !bytes.Equal(refNo, txo.ReferenceNumber()) {
		return 0, errors.New("reference number mismatch")
	}

	var sum uint64
	switch txo.Type {
	case money.TransactionTypeTransfer:
		attr := money.TransferAttributes{}
		if err := txo.UnmarshalAttributes(&attr); err != nil {
			return 0, fmt.Errorf("decoding transfer attributes: %w", err)
		}
		ownerPKH, err := templates.ExtractPubKeyHashFromP2pkhPredicate(attr.NewOwnerPredicate)
		if err != nil {
			return 0, fmt.Errorf("extracting bearer pkh: %w", err)
		}
		if !bytes.Equal(ownerPKH, receiverPKH) {
			return 0, nil
		}
		sum = attr.TargetValue
	case money.TransactionTypeSplit:
		attr := money.SplitAttributes{}
		if err := txo.UnmarshalAttributes(&attr); err != nil {
			return 0, fmt.Errorf("decoding split attributes: %w", err)
		}
		for _, v := range attr.TargetUnits {
			ownerPKH, err := templates.ExtractPubKeyHashFromP2pkhPredicate(v.OwnerPredicate)
			if err != nil {
				return 0, fmt.Errorf("extracting owner pkh: %w", err)
			}
			if !bytes.Equal(ownerPKH, receiverPKH) {
				continue
			}
			sum += v.Amount
		}
	default:
		return 0, nil
	}

	// potentially costly operation so we do it last
	if err := types.VerifyTxProof(txRecordProof, trustBase, crypto.SHA256); err != nil {
		return 0, fmt.Errorf("verification of transaction: %w", err)
	}

	return sum, nil
}

/*
trustBaseLoader returns function which loads the trustbase in effect on
the epoch of the tx proof
*/
func trustBaseLoader(getTrustBase func(epoch uint64) (types.RootTrustBase, error)) func(proof *types.TxProof) (types.RootTrustBase, error) {
	return func(proof *types.TxProof) (types.RootTrustBase, error) {
		uc, err := proof.GetUC()
		if err != nil {
			return nil, fmt.Errorf("reading UC: %w", err)
		}
		if uc.UnicitySeal == nil {
			return nil, fmt.Errorf("invalid UC: UnicitySeal is unassigned")
		}
		trustBase, err := getTrustBase(uc.UnicitySeal.Epoch)
		if err != nil {
			return nil, fmt.Errorf("acquiring trust base: %w", err)
		}
		return trustBase, nil
	}
}

type txoEvalCtx struct {
	EvalEnvironment
	exArgument func() ([]byte, error)
}

func (ec txoEvalCtx) ExtraArgument() ([]byte, error) {
	if ec.exArgument == nil {
		return nil, errors.New("extra arguments callback not assigned")
	}
	return ec.exArgument()
}
