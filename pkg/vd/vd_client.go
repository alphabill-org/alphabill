package verifiable_data

import (
	"context"
	"crypto/sha256"
	"io"
	"os"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"

	"github.com/pkg/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/abclient"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"github.com/holiman/uint256"
)

type (
	VDClient struct {
		abClient abclient.ABClient
	}

	AlphabillClientConfig struct {
		Uri          string
		WaitForReady bool
	}
)

const timeoutDelta = 100 // TODO make timeout configurable?

func New(_ context.Context, abConf *AlphabillClientConfig) (*VDClient, error) {
	return &VDClient{
		abClient: abclient.New(abclient.AlphabillClientConfig{
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

func (v *VDClient) RegisterHash(hash string) error {
	dataHash, err := uint256.FromHex(hash)
	if err != nil {
		return err
	}
	bytes32 := dataHash.Bytes32()
	return v.registerHashTx(bytes32[:])
}

func (v *VDClient) registerHashTx(hash []byte) error {
	defer func() {
		err := v.abClient.Shutdown()
		if err != nil {
			log.Error(err)
		}
	}()
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
	log.Info("Response: ", resp.String())
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
