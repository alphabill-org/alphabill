package block

import (
	"bytes"
	"crypto"
	"io/ioutil"
	"path/filepath"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/encoding/protojson"
)

// Verify validates struct and verifies proofs.
func (x *Bills) Verify(txConverter TxConverter, verifiers map[string]abcrypto.Verifier) error {
	for _, bill := range x.Bills {
		err := bill.Verify(bill.Id, txConverter, verifiers)
		if err != nil {
			return err
		}
	}
	return nil
}

// Verify validates struct and verifies proof.
func (x *Bill) Verify(unitID []byte, txConverter TxConverter, verifiers map[string]abcrypto.Verifier) error {
	gtx, err := txConverter.ConvertTx(x.TxProof.Tx)
	if err != nil {
		return err
	}
	if !bytes.Equal(x.TxHash, gtx.Hash(crypto.SHA256)) {
		return errors.New("bill txHash is not equal to actual transaction hash")
	}
	return x.TxProof.Verify(unitID, gtx, verifiers, crypto.SHA256)
}

func (x *TxProof) Verify(unitID []byte, gtx txsystem.GenericTransaction, verifiers map[string]abcrypto.Verifier, hashAlgo crypto.Hash) error {
	return x.Proof.Verify(uint256.NewInt(0).SetBytes(unitID), gtx, verifiers, hashAlgo)
}

func ReadBillsFile(path string) (*Bills, error) {
	b, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	res := &Bills{}
	err = protojson.Unmarshal(b, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func WriteBillsFile(path string, res *Bills) error {
	b, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(res)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, b, 0600) // -rw-------
}
