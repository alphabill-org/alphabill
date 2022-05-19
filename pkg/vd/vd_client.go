package verifiable_data

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	rtx "gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/abclient"

	"github.com/holiman/uint256"
)

type (
	vdClient struct {
		abClient abclient.ABClient
	}

	AlphabillClientConfig struct {
		Uri          string
		WaitForReady bool
	}
)

func New(_ context.Context, abConf *AlphabillClientConfig) *vdClient {
	return &vdClient{
		abClient: abclient.New(abclient.AlphabillClientConfig{
			Uri:          abConf.Uri,
			WaitForReady: abConf.WaitForReady,
		}),
	}
}

func (v *vdClient) RegisterFileHash(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Failed to open the file: ", err)
		return err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		fmt.Println("Error reading the file: ", err)
		return err
	}

	hash := hasher.Sum(nil)
	fmt.Printf("Hash of file '%s': %x", filePath, hash)
	return v.registerHashTx(hash)
}

func (v *vdClient) RegisterHash(hash string) error {
	dataHash, err := uint256.FromHex(hash)
	if err != nil {
		return err
	}
	bytes32 := dataHash.Bytes32()
	return v.registerHashTx(bytes32[:])
}

func (v *vdClient) registerHashTx(hash []byte) error {
	tx, err := createRegisterDataTx(hash, 100) // TODO make timeout configurable?
	defer v.abClient.Shutdown()
	if err != nil {
		return err
	}
	resp, err := v.abClient.SendTransaction(tx)
	if err != nil {
		return err
	}
	fmt.Printf("Response: %s\n", resp.String())
	return nil
}

func createRegisterDataTx(hash []byte, timeout uint64) (*transaction.Transaction, error) {
	tx := &transaction.Transaction{
		UnitId:                hash,
		SystemId:              []byte{0, 0, 0, 1},
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &rtx.RegisterData{}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}
