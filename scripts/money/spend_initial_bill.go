package main

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

/*
Example usage
go run scripts/money/spend_initial_bill.go --pubkey 0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3 --alphabill-uri localhost:26766 --bill-id 1 --bill-value 1000000000000000000 --timeout 10
*/
func main() {
	// parse command line parameters
	pubKeyHex := flag.String("pubkey", "", "public key of the new bill owner")
	billIdUint := flag.Uint64("bill-id", 0, "bill id of the spendable bill")
	billValue := flag.Uint64("bill-value", 0, "bill value of the spendable bill")
	timeout := flag.Uint64("timeout", 0, "transaction timeout (block number)")
	uri := flag.String("alphabill-uri", "", "alphabill node uri where to send the transaction")
	flag.Parse()

	// verify command line parameters
	if *pubKeyHex == "" {
		log.Fatal("pubkey is required")
	}
	if *billIdUint == 0 {
		log.Fatal("bill-id is required")
	}
	if *billValue == 0 {
		log.Fatal("bill-value is required")
	}
	if *timeout == 0 {
		log.Fatal("timeout is required")
	}
	if *uri == "" {
		log.Fatal("alphabill-uri is required")
	}

	// process command line parameters
	pubKey, err := hexutil.Decode(*pubKeyHex)
	if err != nil {
		log.Fatal(err)
	}
	bytes32 := uint256.NewInt(*billIdUint).Bytes32()
	billID := bytes32[:]

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, *uri, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
	txClient := alphabill.NewAlphabillServiceClient(conn)
	res, err := txClient.GetRoundNumber(ctx, &emptypb.Empty{})
	if err != nil {
		log.Fatal(err)
	}
	absoluteTimeout := res.RoundNumber + *timeout

	feeAmount := uint64(3)
	fcrID := util.SameShardIDBytes(billID, hash.Sum256(pubKey))

	// create transferFC
	transferFC, err := createTransferFC(feeAmount, billID, fcrID, res.RoundNumber, absoluteTimeout)
	if err != nil {
		log.Fatal(err)
	}
	transferFCBytes, err := cbor.Marshal(transferFC)
	if err != nil {
		log.Fatal(err)
	}
	protoTransferFC := &alphabill.Transaction{Order: transferFCBytes}

	// send transferFC
	_, err = txClient.ProcessTransaction(ctx, protoTransferFC)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("sent transferFC transaction")

	// wait for transferFC proof
	transferFCProof, err := waitForConfirmation(ctx, txClient, transferFC, res.RoundNumber, absoluteTimeout)
	if err != nil {
		log.Fatalf("failed to confirm transferFC transaction %v", err)
	} else {
		log.Println("confirmed transferFC transaction")
	}

	// create addFC
	addFC, err := createAddFC(fcrID, script.PredicateAlwaysTrue(), transferFCProof.TxRecord, transferFCProof.TxProof, absoluteTimeout, feeAmount)
	if err != nil {
		log.Fatal(err)
	}
	addFCBytes, err := cbor.Marshal(addFC)
	if err != nil {
		log.Fatal(err)
	}
	protoAddFC := &alphabill.Transaction{Order: addFCBytes}

	// send addFC
	_, err = txClient.ProcessTransaction(ctx, protoAddFC)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("sent addFC transaction")

	// wait for addFC confirmation
	_, err = waitForConfirmation(ctx, txClient, addFC, res.RoundNumber, absoluteTimeout)
	if err != nil {
		log.Fatalf("failed to confirm addFC transaction %v", err)
	} else {
		log.Println("confirmed addFC transaction")
	}

	// create transfer tx
	tx, err := createTransferTx(pubKey, billID, *billValue-feeAmount, fcrID, absoluteTimeout, transferFC.Hash(crypto.SHA256))
	if err != nil {
		log.Fatal(err)
	}
	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		log.Fatal(err)
	}
	protoTransferTx := &alphabill.Transaction{Order: txBytes}

	// get round number for timeout
	res, err = txClient.GetRoundNumber(ctx, &emptypb.Empty{})
	if err != nil {
		log.Fatal(err)
	}
	absoluteTimeout = res.RoundNumber + *timeout

	// send transfer tx
	if _, err := txClient.ProcessTransaction(ctx, protoTransferTx); err != nil {
		log.Fatal(err)
	}
	log.Println("successfully sent initial bill transfer transaction")
}

func createTransferFC(feeAmount uint64, unitID []byte, targetUnitID []byte, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&transactions.TransferFeeCreditAttributes{
			Amount:                 feeAmount,
			TargetSystemIdentifier: []byte{0, 0, 0, 0},
			TargetRecordID:         targetUnitID,
			EarliestAdditionTime:   t1,
			LatestAdditionTime:     t2,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       []byte{0, 0, 0, 0},
			Type:           transactions.PayloadTypeTransferFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: t2, MaxTransactionFee: 1},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}
	return tx, nil
}

func createAddFC(unitID []byte, ownerCondition []byte, transferFC *types.TransactionRecord, transferFCProof *types.TxProof, timeout uint64, maxFee uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&transactions.AddFeeCreditAttributes{
			FeeCreditTransfer:       transferFC,
			FeeCreditTransferProof:  transferFCProof,
			FeeCreditOwnerCondition: ownerCondition,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       []byte{0, 0, 0, 0},
			Type:           transactions.PayloadTypeAddFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: timeout, MaxTransactionFee: maxFee},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}, nil
}

func createTransferTx(pubKey []byte, unitID []byte, billValue uint64, fcrID []byte, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&billtx.TransferAttributes{
			NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
			TargetValue: billValue,
			Backlink:    backlink,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   []byte{0, 0, 0, 0},
			Type:       billtx.PayloadTypeTransfer,
			UnitID:     unitID,
			Attributes: attr,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: 1,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}, nil
}

func waitForConfirmation(ctx context.Context, abClient alphabill.AlphabillServiceClient, pendingTx *types.TransactionOrder, latestRoundNumber, timeout uint64) (*wallet.Proof, error) {
	for latestRoundNumber <= timeout {
		res, err := abClient.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: latestRoundNumber})
		if err != nil {
			return nil, err
		}
		blockBytes := res.Block
		if blockBytes == nil || (len(blockBytes) == 1 && blockBytes[0] == 0xf6) { // 0xf6 cbor Null
			// block might be empty, check latest round number
			res, err := abClient.GetRoundNumber(ctx, &emptypb.Empty{})
			if err != nil {
				return nil, err
			}
			if res.RoundNumber > latestRoundNumber {
				latestRoundNumber++
			} else {
				// wait for some time before retrying to fetch new block
				select {
				case <-time.After(time.Second):
					continue
				case <-ctx.Done():
					return nil, nil
				}
			}
		} else {
			block := &types.Block{}
			if err := cbor.Unmarshal(blockBytes, block); err != nil {
				return nil, fmt.Errorf("failed to unmarshal block: %w", err)
			}
			for i, tx := range block.Transactions {
				if bytes.Equal(tx.TransactionOrder.UnitID(), pendingTx.UnitID()) {
					return wallet.NewTxProof(i, block, crypto.SHA256)
				}
			}
			latestRoundNumber++
		}
	}
	return nil, errors.New("error tx failed to confirm")
}
