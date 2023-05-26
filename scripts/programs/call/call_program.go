package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"

	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/program"
	"github.com/alphabill-org/alphabill/scripts/programs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

/*
Example usage
start shard node:
$ ./setup-testab.sh -m 0 -t 0 -d 0 -p 3 && ./start.sh -r -p programs

run script:
$ go run scripts/programs/call/call_program.go --id "counter" --func "count"
*/
func main() {
	// parse command line parameters
	id := flag.String("id", "", "string to be used as program identifier")
	fn := flag.String("func", "", "function to call")
	timeout := flag.Uint64("timeout", 1000, "transaction timeout (block number)")
	uri := flag.String("alphabill-uri", "localhost:29766", "alphabill node uri where to send the transaction")
	flag.Parse()

	if id == nil || len(*id) < 1 {
		log.Fatal("program-id is required")
	}
	if *timeout <= 0 {
		log.Fatal("timeout is required")
	}
	if *uri == "" {
		log.Fatal("alphabill-uri is required")
	}
	progID := sha256.New().Sum([]byte(*id))
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, *uri, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err = conn.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
	txClient := alphabill.NewAlphabillServiceClient(conn)
	blockNr, err := txClient.GetRoundNumber(ctx, &emptypb.Empty{})
	if err != nil {
		log.Fatal(err)
	}
	absoluteTimeout := blockNr.RoundNumber + *timeout

	// create tx
	txCallOrder, err := programs.NewProgramTransaction(progID,
		programs.WithClientMetadata(&txsystem.ClientMetadata{
			Timeout: absoluteTimeout,
		}),
		programs.WithAttributes(&program.PCallAttributes{
			Function: *fn,
			Input:    programs.Uint64ToLEBytes(1),
		}))
	if err != nil {
		log.Fatal(fmt.Sprintf("failed to create program call tx, %v", err))
	}
	// send tx
	if _, err = txClient.ProcessTransaction(ctx, txCallOrder); err != nil {
		log.Fatal(err)
	}
	log.Println("successfully sent transaction")
}
