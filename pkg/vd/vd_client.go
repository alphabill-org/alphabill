package verifiable_data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/client"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/pkg/errors"
)

type (
	VDClient struct {
		abClient client.ABClient
	}

	AlphabillClientConfig struct {
		Uri          string
		WaitForReady bool
	}
)

const timeoutDelta = 100 // TODO make timeout configurable?

func New(_ context.Context, abConf *AlphabillClientConfig) (*VDClient, error) {
	return &VDClient{
		abClient: client.New(client.AlphabillClientConfig{
			Uri:          abConf.Uri,
			WaitForReady: abConf.WaitForReady,
		}),
	}, nil
}

func (v *VDClient) RegisterFileHash(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open the file %s", filePath)
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return errors.Wrapf(err, "failed to read the file %s", filePath)
	}

	hash := hasher.Sum(nil)
	log.Debug("Hash of file '", filePath, "': ", hash)
	return v.registerHashTx(hash)
}

func (v *VDClient) RegisterHashBytes(bytes []byte) error {
	return v.registerHashTx(bytes)
}

func (v *VDClient) RegisterHash(hash string) error {
	bytes, err := hexStringToBytes(hash)
	if err != nil {
		return err
	}
	return v.registerHashTx(bytes)
}

func (v *VDClient) registerHashTx(hash []byte) error {
	defer func() {
		err := v.abClient.Shutdown()
		if err != nil {
			log.Error(err)
		}
	}()
	if err := validateHash(hash); err != nil {
		return err
	}

	maxBlockNumber, err := v.abClient.GetMaxBlockNumber()
	if err != nil {
		return err
	}

	tx, err := createRegisterDataTx(hash, maxBlockNumber+timeoutDelta)
	if err != nil {
		return err
	}
	resp, err := v.abClient.SendTransaction(tx)
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return errors.New(fmt.Sprintf("error while submitting the hash: %s", resp.GetMessage()))
	}
	log.Info("Hash successfully submitted")
	return nil
}

func validateHash(hash []byte) error {
	if len(hash) != sha256.Size {
		return errors.New(fmt.Sprintf("invalid hash length, expected %d bytes, got %d", sha256.Size, len(hash)))
	}
	return nil
}

func createRegisterDataTx(hash []byte, timeout uint64) (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		UnitId:   hash,
		SystemId: []byte{0, 0, 0, 1},
		Timeout:  timeout,
	}
	return tx, nil
}

func hexStringToBytes(hexString string) ([]byte, error) {
	bs, err := hex.DecodeString(strings.TrimPrefix(hexString, "0x"))
	if err != nil {
		return nil, err
	}
	return bs, nil
}
