package explorer

import (
	"crypto"
	"encoding/hex"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/types"
)

func CreateTxExplorer(blockNo uint64, txRecord *types.TransactionRecord) (*TxExplorer , error){
	if (txRecord == nil){
		return nil, fmt.Errorf("transaction record is nil");
	}
	hashHex := hex.EncodeToString(txRecord.Hash(crypto.SHA256))
	txExplorer := &TxExplorer{
		Hash:             hashHex,
		BlockNumber:      blockNo,
		Timeout:          txRecord.TransactionOrder.Timeout(),
		PayloadType:      txRecord.TransactionOrder.PayloadType(),
		Status:           &txRecord.ServerMetadata.SuccessIndicator,
		TargetUnits:      []types.UnitID{},
		TransactionOrder: txRecord.TransactionOrder,
		Fee:              txRecord.ServerMetadata.GetActualFee(),
	}
	txExplorer.TargetUnits = txRecord.ServerMetadata.TargetUnits
	return txExplorer , nil
}