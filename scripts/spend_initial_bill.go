package main

import (
	"context"
	"flag"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"log"
)

/*
Example usage
go run spend_initial_bill.go --pubkey 0x0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0 --alphabill-uri localhost:9543 --bill-id 1 --bill-value 1000000 --timeout 100
*/
func main() {
	// parse command line parameters
	pubKeyHex := flag.String("pubkey", "", "public key of the new bill owner")
	billIdUint := flag.Uint64("bill-id", 0, "bill id of the spendable bill")
	billValue := flag.Uint64("bill-value", 0, "bill value of the spendable bill")
	timeout := flag.Uint64("timeout", 0, "transaction timeout (block height)")
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
	billId := uint256.NewInt(*billIdUint).Bytes()

	// create tx
	tx, err := createTransferTx(pubKey, billId, *billValue, *timeout)
	if err != nil {
		log.Fatal(err)
	}

	// send tx
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, *uri, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
	txClient := transaction.NewTransactionsClient(conn)
	txResponse, err := txClient.ProcessTransaction(ctx, tx)
	if err != nil {
		log.Fatal(err)
	}
	if txResponse.Ok {
		log.Println("successfully sent transaction")
	} else {
		log.Fatalf("faild to send transaction %v", txResponse.Message)
	}
}

func createTransferTx(pubKey []byte, billId []byte, billValue uint64, timeout uint64) (*transaction.Transaction, error) {
	tx := &transaction.Transaction{
		UnitId:                billId,
		SystemId:              []byte{0},
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &transaction.BillTransfer{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    nil,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}
