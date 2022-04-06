package rootchain

import (
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
)

// NewGenesis creates a new genesis for the root chain and each partition.
func NewGenesis(partitions []*genesis.PartitionRecord, signer crypto.Signer) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	// initiate state
	state, err := newStateFromPartitionRecords(partitions, signer, gocrypto.SHA256)
	if err != nil {
		return nil, nil, err
	}

	// verify that we have consensus between the partition nodes.
	for _, p := range partitions {
		id := string(p.SystemDescriptionRecord.SystemIdentifier)
		if !state.checkConsensus(id) {
			return nil, nil, errors.Errorf("partition %X has not reached a consensus", id)
		}
	}

	// create unicity certificates
	if _, err = state.createUnicityCertificates(); err != nil {
		return nil, nil, err
	}

	genesisPartitions := make([]*genesis.GenesisPartitionRecord, len(partitions))
	partitionGenesis := make([]*genesis.PartitionGenesis, len(partitions))
	rootPublicKey, verifier, err := GetPublicKeyAndVerifier(signer)

	// generate genesis structs
	for i, p := range partitions {
		id := string(p.SystemDescriptionRecord.SystemIdentifier)
		certificate := state.latestUnicityCertificates.get(id)
		genesisPartitions[i] = &genesis.GenesisPartitionRecord{
			Nodes:                   p.Validators,
			Certificate:             certificate,
			SystemDescriptionRecord: p.SystemDescriptionRecord,
		}

		partitionGenesis[i] = &genesis.PartitionGenesis{
			SystemDescriptionRecord: p.SystemDescriptionRecord,
			Certificate:             certificate,
			TrustBase:               rootPublicKey,
		}
	}

	rootGenesis := &genesis.RootGenesis{
		Partitions:    genesisPartitions,
		TrustBase:     rootPublicKey,
		HashAlgorithm: uint32(state.hashAlgorithm),
	}
	if err := rootGenesis.IsValid(verifier, gocrypto.SHA256); err != nil {
		return nil, nil, err
	}
	return rootGenesis, partitionGenesis, nil
}
