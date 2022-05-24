package main

import (
	"context"
	"flag"
	"log"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

/*
Example usage
start shard node:
$ start-vd.sh

run script:
$ go run scripts/verifiable_data/vd_register_data.go --data-hash 0x67588d4d37bf6f4d6c63ce4bda38da2b869012b1bc131db07aa1d2b5bfd810dd
*/
func main() {
	// parse command line parameters
	dataHashHex := flag.String("data-hash", "", "SHA256 hash (hex, prefixed with '0x') of the data to verify")
	timeout := flag.Uint64("timeout", 1000, "transaction timeout (block height)")
	uri := flag.String("alphabill-uri", "localhost:9543", "alphabill node uri where to send the transaction")
	flag.Parse()

	// verify command line parameters
	if *dataHashHex == "" {
		log.Fatal("hash of data is required")
	}
	if *timeout <= 0 {
		log.Fatal("timeout is required")
	}
	if *uri == "" {
		log.Fatal("alphabill-uri is required")
	}

	dataHash, err := uint256.FromHex(*dataHashHex)
	if err != nil {
		log.Fatal(err)
	}
	bytes32 := dataHash.Bytes32()
	dataId := bytes32[:]

	// create tx
	tx, err := createRegisterDataTx(dataId, *timeout)
	if err != nil {
		log.Fatal(err)
	}

	// send tx
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

func createRegisterDataTx(hash []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:                hash,
		SystemId:              []byte{0, 0, 0, 1},
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
	}
	return tx, nil
}
