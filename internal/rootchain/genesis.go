package rootchain

import (
	gocrypto "crypto"
	rstore "github.com/alphabill-org/alphabill/internal/rootchain/store"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

// NewGenesisFromPartitionNodes creates a new genesis for the root chain and partitions.
func NewGenesisFromPartitionNodes(nodes []*genesis.PartitionNode, rootSigner crypto.Signer, encPubKey crypto.Verifier) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	var partitionNodesMap = make(map[string][]*genesis.PartitionNode)
	for _, n := range nodes {
		if err := n.IsValid(); err != nil {
			return nil, nil, err
		}
		si := string(n.GetBlockCertificationRequest().GetSystemIdentifier())
		partitionNodesMap[si] = append(partitionNodesMap[si], n)
	}

	var partitionRecords []*genesis.PartitionRecord
	for _, partitionNodes := range partitionNodesMap {
		pr, err := newPartitionRecord(partitionNodes)
		if err != nil {
			return nil, nil, err
		}
		partitionRecords = append(partitionRecords, pr)
	}
	return NewGenesis(partitionRecords, rootSigner, encPubKey)
}

// NewGenesis creates a new genesis for the root chain and each partition.
func NewGenesis(partitions []*genesis.PartitionRecord, rootSigner crypto.Signer, encPubKey crypto.Verifier) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	// initiate State
	state, err := NewStateFromPartitionRecords(partitions, rootSigner, gocrypto.SHA256, rstore.NewInMemoryRootChainStore())
	if err != nil {
		return nil, nil, err
	}

	encPubKeyBytes, err := encPubKey.MarshalPublicKey()
	if err != nil {
		return nil, nil, err
	}

	// verify that we have consensus between the partition nodes.
	for _, partition := range partitions {
		id := p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)
		if !state.checkConsensus(state.incomingRequests[id]) {
			return nil, nil, errors.Errorf("partition %X has not reached a consensus", id)
		}
	}

	// create unicity certificates
	if _, err = state.CreateUnicityCertificates(); err != nil {
		return nil, nil, err
	}

	genesisPartitions := make([]*genesis.GenesisPartitionRecord, len(partitions))
	partitionGenesis := make([]*genesis.PartitionGenesis, len(partitions))
	rootPublicKey, verifier, err := GetPublicKeyAndVerifier(rootSigner)

	// generate genesis structs
	for i, partition := range partitions {
		id := p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)
		certificate := state.store.GetUC(id)
		genesisPartitions[i] = &genesis.GenesisPartitionRecord{
			Nodes:                   partition.Validators,
			Certificate:             certificate,
			SystemDescriptionRecord: partition.SystemDescriptionRecord,
		}

		var keys = make([]*genesis.PublicKeyInfo, len(partition.Validators))
		for j, v := range partition.Validators {
			keys[j] = &genesis.PublicKeyInfo{
				NodeIdentifier:      v.NodeIdentifier,
				SigningPublicKey:    v.SigningPublicKey,
				EncryptionPublicKey: v.EncryptionPublicKey,
			}
		}

		partitionGenesis[i] = &genesis.PartitionGenesis{
			SystemDescriptionRecord: partition.SystemDescriptionRecord,
			Certificate:             certificate,
			TrustBase:               rootPublicKey,
			EncryptionKey:           encPubKeyBytes,
			Keys:                    keys,
			Params:                  partition.Validators[0].Params,
		}
	}

	rootGenesis := &genesis.RootGenesis{
		Partitions:    genesisPartitions,
		TrustBase:     rootPublicKey,
		HashAlgorithm: uint32(state.hashAlgorithm),
	}
	if err := rootGenesis.IsValid(verifier); err != nil {
		return nil, nil, err
	}
	return rootGenesis, partitionGenesis, nil
}

func newPartitionRecord(nodes []*genesis.PartitionNode) (*genesis.PartitionRecord, error) {
	// validate nodes
	for _, n := range nodes {
		if err := n.IsValid(); err != nil {
			return nil, err
		}
	}
	// create partition record
	pr := &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: nodes[0].BlockCertificationRequest.SystemIdentifier,
			T2Timeout:        nodes[0].T2Timeout,
		},
		Validators: nodes,
	}

	// validate partition record
	if err := pr.IsValid(); err != nil {
		return nil, err
	}
	return pr, nil
}
