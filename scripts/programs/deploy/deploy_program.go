package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem/program"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

/*
Example usage
start shard node:
$ ./setup-testab.sh -m 0 -t 0 -d 0 -p 3 && ./start.sh -r -p programs

run script:
$ go run scripts/programs/deploy/deploy_program.go --id "counter" --wasm-file counter.wasm
*/
func main() {
	// parse command line parameters
	id := flag.String("id", "", "string to be used as program identifier")

	wasmPath := flag.String("wasm-file", "", "path to program wasm code")
	paramStr := flag.String("param", "", "programs params as hex string (hex, prefixed with '0x')")
	timeout := flag.Uint64("timeout", 1000, "transaction timeout (block number)")
	uri := flag.String("alphabill-uri", "localhost:29766", "alphabill node uri where to send the transaction")
	flag.Parse()

	if id == nil || len(*id) < 1 {
		log.Fatal("program-id is required")
	}
	// verify command line parameters
	if wasmPath == nil || *wasmPath == "" {
		log.Fatal("program-file is required")
	}
	if *timeout <= 0 {
		log.Fatal("timeout is required")
	}
	if *uri == "" {
		log.Fatal("alphabill-uri is required")
	}
	// verify command line parameters
	var progParams = make([]byte, 8)
	if *paramStr != "" {
		var err error
		progParams, err = hex.DecodeString(*paramStr)
		if err != nil {
			log.Fatal(err)
		}
	}
	progID := sha256.Sum256([]byte(*id))
	wasm, err := os.ReadFile(*wasmPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("program-file read failed, %v", err))
	}
	if len(wasm) < 1 {
		log.Fatal("program-file empty")
	}
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
	tx, err := createProgramDeployTx(progID[:], wasm, progParams, absoluteTimeout)
	if err != nil {
		log.Fatal(fmt.Sprintf("failed to create program deploy tx, %v", err))
	}
	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		log.Fatal(err)
	}
	protoTransferTx := &alphabill.Transaction{Order: txBytes}
	// send tx
	if _, err = txClient.ProcessTransaction(ctx, protoTransferTx); err != nil {
		log.Fatal(err)
	}
	log.Println("successfully sent transaction")
}

func createProgramDeployTx(unitID []byte, wasm []byte, params []byte, t1 uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&program.PDeployAttributes{
			ProgModule: wasm,
			ProgParams: params,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       program.DefaultProgramsSystemIdentifier,
			Type:           program.ProgramDeploy,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: t1, MaxTransactionFee: 1},
		},
	}
	return tx, nil
}
