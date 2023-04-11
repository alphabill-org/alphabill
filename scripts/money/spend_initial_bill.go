package main

import (
	"bytes"
	"context"
	"crypto"
	"flag"
	"log"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
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
go run scripts/money/spend_initial_bill.go --pubkey 0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3 --alphabill-uri localhost:9543 --bill-id 1 --bill-value 1000000 --timeout 10
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

	tx, err := createTransferTx(pubKey, billID, *billValue, nil, absoluteTimeout, nil)
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

func createTransferTx(pubKey []byte, unitID []byte, billValue uint64, fcrID []byte, timeout uint64, backlink []byte) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:                unitID,
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           timeout,
			MaxFee:            1,
			FeeCreditRecordId: fcrID,
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &billtx.TransferAttributes{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    backlink,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func waitForConfirmation(ctx context.Context, abClient alphabill.AlphabillServiceClient, pendingTx *txsystem.Transaction, latestRoundNumber, timeout uint64) (*block.BlockProof, error) {
	txConverter := money.NewTxConverter([]byte{0, 0, 0, 0})
	for latestRoundNumber <= timeout {
		res, err := abClient.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: latestRoundNumber})
		if err != nil {
			return nil, err
		}
		if res.Block == nil {
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
			for _, tx := range res.Block.Transactions {
				if bytes.Equal(tx.UnitId, pendingTx.UnitId) {
					genericBlock, err := res.Block.ToGenericBlock(txConverter)
					if err != nil {
						return nil, err
					}
					proof, err := block.NewPrimaryProof(genericBlock, tx.UnitId, crypto.SHA256)
					if err != nil {
						return nil, err
					}
					return proof, nil
				}
			}
			latestRoundNumber++
		}
	}
	return nil, errors.New("error tx failed to confirm")
}
