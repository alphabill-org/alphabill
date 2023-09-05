package main

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"math/big"
	"time"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fxamacker/cbor/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ProcessingDetails struct {
	_            struct{} `cbor:",toarray"`
	ErrorDetails string
	ReturnData   []byte
	ContractAddr common.Address
	Logs         []*statedb.LogEntry
}

// Example contract - use https://remix.ethereum.org to compile
/*
// SPDX-License-Identifier: GPL-3.0

pragma solidity >=0.7.0 <0.9.0;

contract Counter {

uint256 value=0;

event Increment(
    uint256 indexed newValue
);

function increment() public returns(uint256) {
    value++;
    emit Increment(value);
    return value;
}

function get() public view returns (uint256) {
    return value;
}
}
*/
// Output
// Compiled binary: 60806040526000805534801561001457600080fd5b506101b1806100246000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80636d4ce63c1461003b578063d09de08a14610059575b600080fd5b610043610077565b60405161005091906100e9565b60405180910390f35b610061610080565b60405161006e91906100e9565b60405180910390f35b60008054905090565b600080600081548092919061009490610133565b91905055506000547f51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a8160405160405180910390a2600054905090565b6000819050919050565b6100e3816100d0565b82525050565b60006020820190506100fe60008301846100da565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061013e826100d0565b91507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff82036101705761016f610104565b5b60018201905091905056fea2646970667358221220282d6a3942d6b9e98489e30b415c3ce915daa6dbe393483126b48e0add741bb364736f6c63430008120033
// function ID's:
// - "get()": "6d4ce63c",
// - "increment()": "d09de08a"
/*
Start EVM node and root
./setup-testab.sh -e 3 && ./start.sh -p evm -r
- deploy
go run scripts/evm/evm.go --max-gas 1000000  --from 0000000000000000000000000000000000000000 --data (hex contract)
- call
go run scripts/evm/evm.go --max-gas 10000 --from 0000000000000000000000000000000000000000 --to (contract addr) --nonce 1  --data (hex functionID+arg)

uint256 - 0000000000000000000000000000000000000000000000000000000000000004
run script:
- deploy contract
$ go run scripts/evm/evm.go --from 0x67588d4d37bf6f4d6c63 --nonce x --data 76474545df
- call contract
$ go run scripts/evm/evm.go --from 0x67588d4d37bf6f4d6c63 --to 0x67588dfg4d37bf6f4d6c63  --nonce x --data functionID
- transfer
$ go run scripts/evm/evm.go --from 0x67588d4d37bf6f4d6c63 --to 0x67588dfg4d37bf6f4d6c63 --nonce x --value in-wei
other parameters:
--timeout - tx timeout, default 1000
--value - value to transfer can be added to all calls, default 0
--uri - default localhost:29766
--max-gas - max gas units the user is willing to pay
--nonce - nonce, +1 per tx execution per account (from addr)
*/
func main() {
	// parse command line parameters
	fromStr := flag.String("from", "", "20 byte from-address (hex, prefixed with '0x')")
	toStr := flag.String("to", "", "20 byte to-address (hex, prefixed with '0x')")
	dataStr := flag.String("data", "", "hex, prefixed with '0x'")
	nonce := flag.Uint64("nonce", 0, "nonce, +1 for every executed Tx")
	val := flag.Uint64("value", 0, "value to transfer")
	gas := flag.Uint64("max-gas", 0, "max gas user is willing to spend on the transaction")
	timeout := flag.Uint64("timeout", 5, "transaction timeout (block number)")
	uri := flag.String("alphabill-uri", "localhost:29766", "alphabill node uri where to send the transaction")
	flag.Parse()

	// verify command line parameters
	if *fromStr == "" {
		log.Fatal("from address is required")
	}
	if *timeout <= 0 {
		log.Fatal("timeout is required")
	}
	if *uri == "" {
		log.Fatal("alphabill-uri is required")
	}
	from, err := addrStrToBytes(*fromStr)
	if err != nil {
		log.Fatal(err)
	}
	to, err := addrStrToBytes(*toStr)
	if err != nil {
		log.Fatal(err)
	}
	var data []byte = nil
	if *dataStr != "" {
		data, err = hex.DecodeString(*dataStr)
		if err != nil {
			log.Fatal("data hex decode error: %w", err)
		}
	}
	if *val > math.MaxInt64 {
		log.Fatal("value too big")
	}
	value := int64(*val)

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
	blockNr, err := txClient.GetRoundNumber(ctx, &emptypb.Empty{})
	if err != nil {
		log.Fatal(err)
	}
	absoluteTimeout := blockNr.RoundNumber + *timeout
	// create tx
	evmAttr := &evm.TxAttributes{
		From:  from,
		To:    to,
		Data:  data,
		Value: big.NewInt(value),
		Gas:   *gas,
		Nonce: *nonce,
	}
	attrBytes, err := cbor.Marshal(evmAttr)
	if err != nil {
		log.Fatal("evm attributes serialization failed")
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           evm.PayloadTypeEVMCall,
			SystemID:       evm.DefaultEvmTxSystemIdentifier,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: &types.ClientMetadata{Timeout: absoluteTimeout},
			Attributes:     attrBytes,
		},
		OwnerProof: nil,
	}

	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		log.Fatal(err)
	}

	// send tx
	if _, err = txClient.ProcessTransaction(ctx, &alphabill.Transaction{Order: txBytes}); err != nil {
		log.Fatal(err)
	}
	// wait for tx confirmation
	var proof *wallet.Proof
	proof, err = waitForConfirmation(ctx, txClient, tx, blockNr.RoundNumber, absoluteTimeout)
	if err != nil {
		log.Fatalf("failed to confirm evm transaction %v", err)
	} else {
		log.Printf("evm transaction was executed, status: %v", getStatusFromMetadata(proof.TxRecord.ServerMetadata.SuccessIndicator))
		log.Printf("fee: %v", proof.TxRecord.ServerMetadata.ActualFee)
		if proof.TxRecord.ServerMetadata.ProcessingDetails != nil {
			var details ProcessingDetails
			if err = cbor.Unmarshal(proof.TxRecord.ServerMetadata.ProcessingDetails, &details); err != nil {
				log.Fatal("evm processing result de-serialization failed: %w", err)
			}
			if proof.TxRecord.ServerMetadata.SuccessIndicator == types.TxStatusFailed {
				log.Printf("error details: %v", details.ErrorDetails)
				return
			}
			noContract := common.Address{} // content if no contract is deployed
			if details.ContractAddr != noContract {
				log.Printf("deployed contract address %x", details.ContractAddr)
			}
			for i, l := range details.Logs {
				log.Printf("log %v : %v", i, l)
			}
			if len(details.ReturnData) > 0 {
				log.Printf("return data: %x", details.ReturnData)
			}
		}
	}
}

func getStatusFromMetadata(s types.TxStatus) string {
	if s == types.TxStatusSuccessful {
		return "success"
	}
	return "failed"
}

func addrStrToBytes(addrStr string) ([]byte, error) {
	if addrStr == "" {
		return nil, nil
	}
	addr, err := hex.DecodeString(addrStr)
	if err != nil {
		return nil, fmt.Errorf("addr string parsing failed: %w", err)
	}
	if len(addr) != 20 {
		return nil, fmt.Errorf("invalid address string")
	}
	return addr, nil
}

func waitForConfirmation(ctx context.Context, abClient alphabill.AlphabillServiceClient, pendingTx *types.TransactionOrder, latestRoundNumber, timeout uint64) (*wallet.Proof, error) {
	blockNo := latestRoundNumber
	for blockNo <= timeout {
		res, err := abClient.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: blockNo})
		if err != nil {
			return nil, err
		}
		blockBytes := res.Block
		if blockBytes == nil || len(blockBytes) == 1 && blockBytes[0] == 0xf6 { // 0xf6 cbor Null
			// wait for some time and try to fetch the same block number again
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return nil, nil
			}
		} else {
			block := &types.Block{}
			if err = cbor.Unmarshal(blockBytes, block); err != nil {
				return nil, fmt.Errorf("failed to unmarshal block: %w", err)
			}
			for i, tx := range block.Transactions {
				if bytes.Equal(tx.TransactionOrder.UnitID(), pendingTx.UnitID()) {
					return wallet.NewTxProof(i, block, crypto.SHA256)
				}
			}
			blockNo++
		}
	}
	return nil, errors.New("error tx failed to confirm")
}
