package explorer

import (
	"crypto"
	"encoding/hex"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/types"
)
func CreateBlockExplorer(b *types.Block) (*BlockExplorer , error){
	if (b == nil){
		return nil, fmt.Errorf("block is nil");
	}
	var txHashes []string

		for _, tx := range b.Transactions {
			hash := tx.Hash(crypto.SHA256) // crypto.SHA256?
			hashHex:= hex.EncodeToString(hash);
			txHashes = append(txHashes, hashHex)
		}

		header := &HeaderExplorer{
			Timestamp:         b.UnicityCertificate.UnicitySeal.Timestamp,
			BlockHash:         b.UnicityCertificate.InputRecord.BlockHash,
			PreviousBlockHash: b.Header.PreviousBlockHash,
			ProposerID:        b.GetProposerID(),
		}
		blockExplorer := &BlockExplorer{
			SystemID:        &b.Header.SystemID,
			RoundNumber:     b.GetRoundNumber(),
			Header:          header,
			TxHashes:        txHashes,
			SummaryValue:    b.UnicityCertificate.InputRecord.SummaryValue,
			SumOfEarnedFees: b.UnicityCertificate.InputRecord.SumOfEarnedFees,
		}
	return blockExplorer , nil
}

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