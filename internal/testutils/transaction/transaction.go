package testtransaction

import (
	"math/rand"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func RandomBillTransfer() *transaction.Transaction {
	tx := randomTx()

	bt := &transaction.BillTransfer{
		NewBearer: randomBytes(3),
		// #nosec G404
		TargetValue: rand.Uint64(),
		Backlink:    randomBytes(3),
	}
	anypb.MarshalFrom(tx.TransactionAttributes, bt, proto.MarshalOptions{})
	return tx
}

func RandomBillSplit() *transaction.Transaction {
	tx := randomTx()

	bt := &transaction.BillSplit{
		// #nosec G404
		Amount:       rand.Uint64(),
		TargetBearer: randomBytes(3),
		// #nosec G404
		RemainingValue: rand.Uint64(),
		Backlink:       randomBytes(3),
	}
	anypb.MarshalFrom(tx.TransactionAttributes, bt, proto.MarshalOptions{})
	return tx
}

func randomTx() *transaction.Transaction {
	return &transaction.Transaction{
		TransactionAttributes: new(anypb.Any),
		UnitId:                randomBytes(3),
		Timeout:               0,
		OwnerProof:            randomBytes(3),
	}
}

func randomBytes(len int) []byte {
	bytes := make([]byte, len)
	// #nosec G404
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}
