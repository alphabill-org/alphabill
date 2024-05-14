package wvm

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
)

/*
AB functions to verify objects etc
*/
func addAlphabillModule(ctx context.Context, rt wazero.Runtime, _ Observability) error {
	_, err := rt.NewHostModuleBuilder("ab").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(digest_sha256), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("digest_sha256").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(verifyTxProof), []api.ValueType{api.ValueTypeI64, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI32}).Export("verify_tx_proof").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(amountTransferred), []api.ValueType{api.ValueTypeI64, api.ValueTypeI64, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("amount_transferred").
		Instantiate(ctx)
	return err
}

func verifyTxProof(vec *VmContext, mod api.Module, stack []uint64) error {
	// args: handle of txProof, handle of txRec
	proof, err := getVar[*types.TxProof](vec.curPrg.vars, stack[0])
	if err != nil {
		return fmt.Errorf("tx proof: %w", err)
	}
	txRec, err := getVar[*types.TransactionRecord](vec.curPrg.vars, stack[1])
	if err != nil {
		return fmt.Errorf("tx record: %w", err)
	}
	tb, err := vec.curPrg.env.TrustBase(0)
	if err != nil {
		return fmt.Errorf("acquiring trust base: %w", err)
	}
	if err := types.VerifyTxProof(proof, txRec, tb, crypto.SHA256); err != nil {
		vec.log.Debug(fmt.Sprintf("%s.verifyTxProof: %v", mod.Name(), err))
		stack[0] = 1
	} else {
		stack[0] = 0
	}
	return nil
}

func digest_sha256(vec *VmContext, mod api.Module, stack []uint64) error {
	data := read(mod, stack[0])
	addr, err := vec.writeToMemory(mod, hash.Sum256(data))
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
  - [1] receiver_pkh: address of PubKey hash to which the money has been transferred to;
  - [2] ref_no: address (if given, ie not zero) of the reference number transfer(s)
    must have in order to be counted;
*/
func amountTransferred(vec *VmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(stack[0])
	if err != nil {
		return fmt.Errorf("reading input data: %w", err)
	}
	var txs []types.TxRecordProof
	if err := types.Cbor.Unmarshal(data, &txs); err != nil {
		return fmt.Errorf("decoding data as slice of tx proofs: %w", err)
	}

	tb, err := vec.curPrg.env.TrustBase(0)
	if err != nil {
		return fmt.Errorf("acquiring trust base: %w", err)
	}

	pkh := read(mod, stack[1])

	var ref_no []byte
	if addrRefNo := stack[2]; addrRefNo != 0 {
		ref_no = read(mod, addrRefNo)
	}

	sum, err := amountTransferredSum(tb, txs, pkh, ref_no)
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
func amountTransferredSum(trustBase types.RootTrustBase, proofs []types.TxRecordProof, receiverPK []byte, refNo []byte) (uint64, error) {
	var total uint64
	var rErr error
	for i, v := range proofs {
		sum, err := transferredSum(trustBase, v.TxRecord, v.TxProof, receiverPK, refNo)
		if err != nil {
			rErr = errors.Join(rErr, fmt.Errorf("record[%d]: %w", i, err))
		}
		total += sum
	}
	return total, nil
}

/*
transferredSum returns the sum transferred to the "receiverPKH" by given transaction
record.
Arguments:
  - receiverPKH: public key hash of the recipient - currently only ptpkh template is
    supported as owner condition;
  - refNo: reference number of the transaction, if nil then ignored, otherwise must match
    (use not nil zero length slice to get sum of transfers without reference number);

unknown / invalid transactions are ignored (not error)?
*/
func transferredSum(trustBase types.RootTrustBase, txRec *types.TransactionRecord, txProof *types.TxProof, receiverPKH []byte, refNo []byte) (uint64, error) {
	if txRec == nil || txProof == nil || txRec.TransactionOrder == nil {
		return 0, errors.New("invalid input: either proof, tx record or tx order is unassigned")
	}

	txo := txRec.TransactionOrder
	if txo.SystemID() != money.DefaultSystemID {
		return 0, fmt.Errorf("expected partition id %d got %d", money.DefaultSystemID, txo.SystemID())
	}
	if refNo != nil && (txo.Payload == nil || txo.Payload.ClientMetadata == nil || !bytes.Equal(refNo, txo.Payload.ClientMetadata.ReferenceNumber)) {
		return 0, errors.New("reference number mismatch")
	}

	var sum uint64
	switch txo.PayloadType() {
	case money.PayloadTypeTransfer:
		attr := money.TransferAttributes{}
		if err := txo.UnmarshalAttributes(&attr); err != nil {
			return 0, fmt.Errorf("decoding transfer attributes: %w", err)
		}
		ownerPKH, err := templates.ExtractPubKeyHashFromP2pkhPredicate(attr.NewBearer)
		if err != nil {
			return 0, fmt.Errorf("extracting bearer pkh: %w", err)
		}
		if !bytes.Equal(ownerPKH, receiverPKH) {
			return 0, nil
		}
		sum = attr.TargetValue
	case money.PayloadTypeSplit:
		attr := money.SplitAttributes{}
		if err := txo.UnmarshalAttributes(&attr); err != nil {
			return 0, fmt.Errorf("decoding split attributes: %w", err)
		}
		for _, v := range attr.TargetUnits {
			ownerPKH, err := templates.ExtractPubKeyHashFromP2pkhPredicate(v.OwnerCondition)
			if err != nil {
				return 0, fmt.Errorf("extracting owner pkh: %w", err)
			}
			if !bytes.Equal(ownerPKH, receiverPKH) {
				return 0, nil
			}
			sum += v.Amount
		}
	default:
		return 0, nil
	}

	// potentially costly operation so we do it last
	if err := types.VerifyTxProof(txProof, txRec, trustBase, crypto.SHA256); err != nil {
		return 0, fmt.Errorf("verification of transaction: %w", err)
	}

	return sum, nil
}
