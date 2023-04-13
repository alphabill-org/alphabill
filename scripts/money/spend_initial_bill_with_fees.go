package main

import (
	"context"
	"crypto"
	"flag"
	"log"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

/*
Example usage
go run scripts/money/spend_initial_bill_with_fees.go --pubkey 0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3 --alphabill-uri localhost:9543 --bill-id 1 --bill-value 1000000 --timeout 10
*/
func main2() {
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

	txFee := uint64(1)
	feeAmount := uint64(2)
	fcrID := util.SameShardIDBytes(uint256.NewInt(0).SetBytes(billID), hash.Sum256(pubKey))

	// create transferFC
	transferFC, err := createTransferFC(feeAmount, billID, fcrID, res.RoundNumber, absoluteTimeout)
	if err != nil {
		log.Fatal(err)
	}
	// send transferFC
	_, err = txClient.ProcessTransaction(ctx, transferFC)
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
	transferFC.ServerMetadata = &txsystem.ServerMetadata{Fee: txFee} // add server metadata so that hash is correct
	addFC, err := createAddFC(fcrID, script.PredicateAlwaysTrue(), transferFC, transferFCProof, absoluteTimeout, feeAmount)
	if err != nil {
		log.Fatal(err)
	}
	// send addFC
	_, err = txClient.ProcessTransaction(ctx, addFC)
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
	transferFCWrapper, err := transactions.NewFeeCreditTx(transferFC)
	if err != nil {
		log.Fatalf("failed to wrap transferFC %v", err)
	}
	tx, err := createTransferTx(pubKey, billID, *billValue-feeAmount-txFee, fcrID, absoluteTimeout, transferFCWrapper.Hash(crypto.SHA256))
	if err != nil {
		log.Fatal(err)
	}
	// get round number for timeout
	res, err = txClient.GetRoundNumber(ctx, &emptypb.Empty{})
	if err != nil {
		log.Fatal(err)
	}
	absoluteTimeout = res.RoundNumber + *timeout

	// send transfer tx
	if _, err := txClient.ProcessTransaction(ctx, tx); err != nil {
		log.Fatal(err)
	}
	log.Println("successfully sent initial bill transfer transaction")
}

func createTransferFC(feeAmount uint64, unitID []byte, targetUnitID []byte, t1, t2 uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:                unitID,
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout: t2,
			MaxFee:  1,
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.TransferFeeCreditAttributes{
		Amount:                 feeAmount,
		TargetSystemIdentifier: []byte{0, 0, 0, 0},
		TargetRecordId:         targetUnitID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createAddFC(unitID []byte, ownerCondition []byte, transferFC *txsystem.Transaction, transferFCProof *block.BlockProof, timeout uint64, maxFee uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:                unitID,
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout: timeout,
			MaxFee:  maxFee,
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transactions.AddFeeCreditAttributes{
		FeeCreditTransfer:       transferFC,
		FeeCreditTransferProof:  transferFCProof,
		FeeCreditOwnerCondition: ownerCondition,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}
