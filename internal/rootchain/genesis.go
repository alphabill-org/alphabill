package rootchain

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

const (
	GenesisTime = 1668208271000 // 11.11.2022 @ 11:11:11
)

type (
	RootNodeInfo struct {
		peerID    string
		signer    crypto.Signer
		encPubKey []byte
	}
	rootGenesisConf struct {
		peerID                string
		encryptionPubKeyBytes []byte
		signer                crypto.Signer
		totalValidators       uint32
		blockRateMs           uint32
		consensusTimeoutMs    uint32
		quorumThreshold       uint32
		hashAlgorithm         gocrypto.Hash
	}

	GenesisOption func(c *rootGenesisConf)
)

func (c *rootGenesisConf) QuorumThreshold() uint32 {
	if c.quorumThreshold == 0 {
		return genesis.GetMinQuorumThreshold(c.totalValidators)
	}
	return c.quorumThreshold
}

func (c *rootGenesisConf) ConsensusTimeoutMs() uint32 {
	return c.consensusTimeoutMs
}

func (c *rootGenesisConf) isValid() error {
	if c.peerID == "" {
		return genesis.ErrNodeIdentifierIsEmpty
	}
	if c.signer == nil {
		return ErrSignerIsNil
	}
	if len(c.encryptionPubKeyBytes) == 0 {
		return fmt.Errorf("encryption public key is nil")
	}
	if c.totalValidators < 1 {
		return fmt.Errorf("total root validators set to 0")
	}
	if c.totalValidators < c.quorumThreshold {
		return fmt.Errorf("quorum threshold set higher %v than total root nodes %v",
			c.quorumThreshold, c.totalValidators)
	}
	return nil
}

func WithTotalNodes(rootValidators uint32) GenesisOption {
	return func(c *rootGenesisConf) {
		c.totalValidators = rootValidators
	}
}

func WithBlockRate(rate uint32) GenesisOption {
	return func(c *rootGenesisConf) {
		c.blockRateMs = rate
	}
}

func WithConsensusTimeout(timeoutMs uint32) GenesisOption {
	return func(c *rootGenesisConf) {
		c.consensusTimeoutMs = timeoutMs
	}
}

func WithQuorumThreshold(threshold uint32) GenesisOption {
	return func(c *rootGenesisConf) {
		c.quorumThreshold = threshold
	}
}

// WithHashAlgorithm set custom hash algorithm (unused for now, remove?)
func WithHashAlgorithm(hashAlgorithm gocrypto.Hash) GenesisOption {
	return func(c *rootGenesisConf) {
		c.hashAlgorithm = hashAlgorithm
	}
}

func NewPartitionRecordFromNodes(nodes []*genesis.PartitionNode) ([]*genesis.PartitionRecord, error) {
	var partitionNodesMap = make(map[string][]*genesis.PartitionNode)
	for _, n := range nodes {
		if err := n.IsValid(); err != nil {
			return nil, err
		}
		si := string(n.GetBlockCertificationRequest().GetSystemIdentifier())
		partitionNodesMap[si] = append(partitionNodesMap[si], n)
	}

	var partitionRecords []*genesis.PartitionRecord
	for _, partitionNodes := range partitionNodesMap {
		pr, err := newPartitionRecord(partitionNodes)
		if err != nil {
			return nil, err
		}
		partitionRecords = append(partitionRecords, pr)
	}
	return partitionRecords, nil
}

func NewRootGenesis(id string, s crypto.Signer, encPubKey []byte, partitions []*genesis.PartitionRecord,
	opts ...GenesisOption) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {

	c := &rootGenesisConf{
		peerID:                id,
		signer:                s,
		encryptionPubKeyBytes: encPubKey,
		totalValidators:       1,
		blockRateMs:           genesis.DefaultBlockRateMs,
		consensusTimeoutMs:    genesis.DefaultConsensusTimeout,
		quorumThreshold:       0,
		hashAlgorithm:         gocrypto.SHA256,
	}

	for _, option := range opts {
		option(c)
	}

	if err := c.isValid(); err != nil {
		return nil, nil, err
	}
	// initiate State
	state, err := NewStateFromPartitionRecords(partitions, c.peerID, c.signer, gocrypto.SHA256)
	if err != nil {
		return nil, nil, err
	}

	// verify that we have consensus between the partition nodes.
	for _, partition := range partitions {
		id := p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)
		logger.Debug("Checking consensus for '%X'")
		if !state.checkConsensus(id) {
			return nil, nil, errors.Errorf("partition %X has not reached a consensus", id)
		}
	}

	// create unicity certificates
	if _, err = state.CreateUnicityCertificates(GenesisTime); err != nil {
		return nil, nil, err
	}

	genesisPartitions := make([]*genesis.GenesisPartitionRecord, len(partitions))
	partitionGenesis := make([]*genesis.PartitionGenesis, len(partitions))
	rootPublicKey, verifier, err := GetPublicKeyAndVerifier(c.signer)
	if err != nil {
		return nil, nil, err
	}

	// Add local root node info to partition record
	var rootValidatorInfo = make([]*genesis.PublicKeyInfo, 1)
	rootValidatorInfo[0] = &genesis.PublicKeyInfo{
		NodeIdentifier:      c.peerID,
		SigningPublicKey:    rootPublicKey,
		EncryptionPublicKey: c.encryptionPubKeyBytes,
	}
	// generate genesis structs
	for i, partition := range partitions {
		id := p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)
		certificate, err := state.GetLatestUnicityCertificate(id)
		if err != nil {
			return nil, nil, err
		}
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
			RootValidators:          rootValidatorInfo,
			Keys:                    keys,
			Params:                  partition.Validators[0].Params,
		}
	}
	// Sign the consensus and append signature
	consensusParams := &genesis.ConsensusParams{
		TotalRootValidators: c.totalValidators,
		BlockRateMs:         c.blockRateMs,
		ConsensusTimeoutMs:  c.ConsensusTimeoutMs(),
		QuorumThreshold:     c.QuorumThreshold(),
		HashAlgorithm:       uint32(c.hashAlgorithm),
		Signatures:          make(map[string][]byte),
	}
	err = consensusParams.Sign(c.peerID, c.signer)
	if err != nil {
		return nil, nil, err
	}
	genesisRoot := &genesis.GenesisRootRecord{
		RootValidators: rootValidatorInfo,
		Consensus:      consensusParams,
	}
	rootGenesis := &genesis.RootGenesis{
		Root:       genesisRoot,
		Partitions: genesisPartitions,
	}

	if err := rootGenesis.IsValid(); err != nil {
		return nil, nil, err
	}
	// verify that the signing public key is present in root validator info
	pubKeyInfo := rootGenesis.Root.FindPubKeyById(c.peerID)
	if pubKeyInfo == nil {
		return nil, nil, fmt.Errorf("missing public key info for node id %v", c.peerID)
	}
	pubKeyBytes, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to extract public key bytes: %w", err)
	}
	// Compare keys
	if !bytes.Equal(pubKeyBytes, pubKeyInfo.SigningPublicKey) {
		return nil, nil, fmt.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, pubKeyInfo.SigningPublicKey)
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
