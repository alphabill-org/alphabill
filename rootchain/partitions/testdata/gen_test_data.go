package testdata

import (
	"fmt"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	pg "github.com/alphabill-org/alphabill/partition/genesis"
	rg "github.com/alphabill-org/alphabill/rootchain/genesis"
)

func GenData() {
	rootSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	if err != nil {
		panic(err)
	}
	rootPubKey := getPubKey(rootSigner)
	rootNodeID := "test_root_node"

	writeRootGenesisFile("root-genesis-A.json", "A", rootNodeID, rootSigner, rootPubKey)
	writeRootGenesisFile("root-genesis-B.json", "B", rootNodeID, rootSigner, rootPubKey)
	writeRootGenesisFile("root-genesis-C.json", "C", rootNodeID, rootSigner, rootPubKey)
}

func writeRootGenesisFile(filename string, partitionNodeID string, rootNodeID string, rootSigner *abcrypto.InMemorySecp256K1Signer, rootPubKey []byte) {
	partitionID := types.PartitionID(1)
	partitionSigner, err := abcrypto.NewInMemorySecp256K1Signer()
	if err != nil {
		panic(err)
	}
	partition := createPartition(partitionID, partitionNodeID, partitionSigner)
	rootGenesis, _, err := rg.NewRootGenesis(rootNodeID, rootSigner, rootPubKey, []*genesis.PartitionRecord{partition})
	if err != nil {
		panic(err)
	}
	err = util.WriteJsonFile(fmt.Sprintf("./rootchain/partitions/testdata/%s", filename), rootGenesis)
	if err != nil {
		panic(err)
	}
}

func createPartition(partitionID types.PartitionID, nodeID string, partitionSigner abcrypto.Signer) *genesis.PartitionRecord {
	req := createInputRequest(partitionID, nodeID, partitionSigner)
	pubKey := getPubKey(partitionSigner)

	pdr := &types.PartitionDescriptionRecord{Version: 1,
		NetworkIdentifier:   5,
		PartitionIdentifier: partitionID,
		TypeIdLen:           8,
		UnitIdLen:           256,
		T2Timeout:           2500 * time.Millisecond,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         money.NewBillID(nil, make([]byte, 32)),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}
	return &genesis.PartitionRecord{
		PartitionDescription: pdr,
		Validators: []*genesis.PartitionNode{
			{
				Version:                    1,
				NodeIdentifier:             nodeID,
				SigningPublicKey:           pubKey,
				EncryptionPublicKey:        pubKey,
				BlockCertificationRequest:  req,
				PartitionDescriptionRecord: *pdr,
			},
		},
	}
}

func createInputRequest(partitionID types.PartitionID, nodeID string, partitionSigner abcrypto.Signer) *certification.BlockCertificationRequest {
	req := &certification.BlockCertificationRequest{
		Partition:      partitionID,
		NodeIdentifier: nodeID,
		InputRecord: &types.InputRecord{Version: 1,
			PreviousHash: make([]byte, 32),
			Hash:         make([]byte, 32),
			BlockHash:    make([]byte, 32),
			SummaryValue: []byte{1, 0, 0},
			RoundNumber:  pg.PartitionRoundNumber,
		},
	}

	err := req.Sign(partitionSigner)
	if err != nil {
		panic(err)
	}
	return req
}

func getPubKey(signer abcrypto.Signer) []byte {
	verifier, err := signer.Verifier()
	if err != nil {
		panic(err)
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		panic(err)
	}
	return pubKey
}
