package rootchain

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

var ErrEncryptionPubKeyIsNil = errors.New("encryption public key is nil")
var ErrQuorumThresholdOnlyDistributed = errors.New("Quorum threshold must only be less than total nodes in root chain")

type (
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

func (c *rootGenesisConf) QuorumThreshold() *uint32 {
	if c.quorumThreshold == 1 {
		return nil
	}
	return &c.quorumThreshold
}

func (c *rootGenesisConf) ConsensusTimeoutMs() *uint32 {
	if c.totalValidators == 1 {
		return nil
	}
	return &c.consensusTimeoutMs
}

func (c *rootGenesisConf) isValid() error {
	if c.peerID == "" {
		return genesis.ErrNodeIdentifierIsEmpty
	}
	if c.signer == nil {
		return ErrSignerIsNil
	}
	if len(c.encryptionPubKeyBytes) == 0 {
		return ErrEncryptionPubKeyIsNil
	}
	if c.totalValidators > 1 && c.totalValidators < genesis.MinDistributedRootValidators {
		return genesis.ErrInvalidNumberOfRootValidators
	}
	if c.totalValidators < c.quorumThreshold {
		return ErrQuorumThresholdOnlyDistributed
	}
	return nil
}

func WithPeerID(peerID string) GenesisOption {
	return func(c *rootGenesisConf) {
		c.peerID = peerID
	}
}

func WithSigningKey(signer crypto.Signer) GenesisOption {
	return func(c *rootGenesisConf) {
		c.signer = signer
	}
}

func WithEncryptionPubKey(encryptionPubKey []byte) GenesisOption {
	return func(c *rootGenesisConf) {
		c.encryptionPubKeyBytes = encryptionPubKey
	}
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

/*
// NewGenesisFromPartitionNodes creates a new genesis for the root chain and partitions.
func NewGenesisFromPartitionNodes(nodes []*genesis.PartitionNode, opts ...GenesisOption) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	c := &rootGenesisConf{
		totalValidators:    1,
		blockRateMs:        900,
		consensusTimeoutMs: 0,
		quorumThreshold:    0,
		hashAlgorithm:      gocrypto.SHA256,
	}

	for _, option := range opts {
		option(c)
	}

	if err := c.isValid(); err != nil {
		return nil, nil, err
	}

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
	return NewGenesis(partitionRecords, consensus, rootSigner, encPubKey)
}
*/
func NewRootGenesis(partitions []*genesis.PartitionRecord, opts ...GenesisOption) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	c := &rootGenesisConf{
		totalValidators:    1,
		blockRateMs:        900,
		consensusTimeoutMs: 0,
		quorumThreshold:    0,
		hashAlgorithm:      gocrypto.SHA256,
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
	for _, p := range partitions {
		id := string(p.SystemDescriptionRecord.SystemIdentifier)
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
	for i, p := range partitions {
		id := string(p.SystemDescriptionRecord.SystemIdentifier)
		certificate := state.latestUnicityCertificates.get(id)
		genesisPartitions[i] = &genesis.GenesisPartitionRecord{
			Nodes:                   p.Validators,
			Certificate:             certificate,
			SystemDescriptionRecord: p.SystemDescriptionRecord,
		}

		var keys = make([]*genesis.PublicKeyInfo, len(p.Validators))
		for j, v := range p.Validators {
			keys[j] = &genesis.PublicKeyInfo{
				NodeIdentifier:      v.NodeIdentifier,
				SigningPublicKey:    v.SigningPublicKey,
				EncryptionPublicKey: v.EncryptionPublicKey,
			}
		}

		partitionGenesis[i] = &genesis.PartitionGenesis{
			SystemDescriptionRecord: p.SystemDescriptionRecord,
			Certificate:             certificate,
			RootValidators:          rootValidatorInfo,
			Keys:                    keys,
			Params:                  p.Validators[0].Params,
		}
	}
	// Sign the consensus and append signature
	consensusParams := &genesis.ConsensusParams{
		TotalRootValidators: c.totalValidators,
		BlockRateMs:         c.blockRateMs,
		ConsensusTimeoutMs:  c.ConsensusTimeoutMs(),
		QuorumThreshold:     c.QuorumThreshold(),
		HashAlgorithm:       uint32(c.hashAlgorithm),
	}
	alg := gocrypto.Hash(c.hashAlgorithm)
	hash := consensusParams.Hash(alg)
	sig, err := c.signer.SignHash(hash)
	if err != nil {
		return nil, nil, err
	}
	consensusParams.Signatures = make(map[string][]byte)
	consensusParams.Signatures[c.peerID] = sig
	rootClusterRecord := &genesis.GenesisRootCluster{
		RootValidators: rootValidatorInfo,
		Consensus:      consensusParams,
	}
	rootGenesis := &genesis.RootGenesis{
		RootCluster: rootClusterRecord,
		Partitions:  genesisPartitions,
	}

	if err := rootGenesis.IsValid(c.peerID, verifier); err != nil {
		return nil, nil, err
	}
	return rootGenesis, partitionGenesis, nil
}

/*
// NewGenesis creates a new genesis for the root chain and each partition.
func NewGenesis(partitions []*genesis.PartitionRecord, consensus *genesis.ConsensusParams,
	rootSigner crypto.Signer, encPubKey crypto.Verifier) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	if consensus == nil {
		return nil, nil, errors.New("Missing root consensus parameters")
	}

	// initiate State
	state, err := NewStateFromPartitionRecords(partitions, rootSigner, gocrypto.SHA256)
	if err != nil {
		return nil, nil, err
	}

	encPubKeyBytes, err := encPubKey.MarshalPublicKey()
	if err != nil {
		return nil, nil, err
	}

	// verify that we have consensus between the partition nodes.
	for _, p := range partitions {
		id := string(p.SystemDescriptionRecord.SystemIdentifier)
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

	// todo: verify the conversion, this might not be correct
	rootId, err := peer.IDFromPublicKey(string(encPubKeyBytes))
	if err != nil {
		return nil, nil, err
	}
	// Add local root node info to partition record
	var rootValidatorInfo = make([]*genesis.PublicKeyInfo, 1)
	rootValidatorInfo[0] = &genesis.PublicKeyInfo{
		NodeIdentifier:      string(rootId),
		SigningPublicKey:    rootPublicKey,
		EncryptionPublicKey: encPubKeyBytes,
	}
	// generate genesis structs
	for i, p := range partitions {
		id := string(p.SystemDescriptionRecord.SystemIdentifier)
		certificate := state.latestUnicityCertificates.get(id)
		genesisPartitions[i] = &genesis.GenesisPartitionRecord{
			Nodes:                   p.Validators,
			Certificate:             certificate,
			SystemDescriptionRecord: p.SystemDescriptionRecord,
		}

		var keys = make([]*genesis.PublicKeyInfo, len(p.Validators))
		for j, v := range p.Validators {
			keys[j] = &genesis.PublicKeyInfo{
				NodeIdentifier:      v.NodeIdentifier,
				SigningPublicKey:    v.SigningPublicKey,
				EncryptionPublicKey: v.EncryptionPublicKey,
			}
		}

		partitionGenesis[i] = &genesis.PartitionGenesis{
			SystemDescriptionRecord: p.SystemDescriptionRecord,
			Certificate:             certificate,
			RootValidators:          rootValidatorInfo,
			Keys:                    keys,
			Params:                  p.Validators[0].Params,
		}
	}
	// Sign the consensus and append signature
	alg := gocrypto.Hash(consensus.HashAlgorithm)
	sig, err := rootSigner.SignBytes(consensus.Hash(alg))
	if err != nil {
		return nil, nil, err
	}
	consensus.Signatures[string(rootId)] = sig
	rootGenesisRecord := &genesis.GenesisRootCluster{
		RootValidators: rootValidatorInfo,
		Consensus:      consensus,
	}
	rootGenesis := &genesis.RootGenesis{
		RootCluster: rootGenesisRecord,
		Partitions:  genesisPartitions,
	}

	//		TrustBase:     rootPublicKey,
	//		HashAlgorithm: uint32(state.hashAlgorithm),
	if err := rootGenesis.IsValid(string(rootId), verifier); err != nil {
		return nil, nil, err
	}
	return rootGenesis, partitionGenesis, nil
}
*/
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
