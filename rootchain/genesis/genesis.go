package genesis

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/rootchain/unicitytree"
)

var ErrEncryptionPubKeyIsNil = errors.New("encryption public key is nil")
var ErrSignerIsNil = errors.New("signer is nil")

type (
	rootGenesisConf struct {
		peerID                string
		encryptionPubKeyBytes []byte
		signer                crypto.Signer
		totalValidators       uint32
		blockRateMs           uint32
		consensusTimeoutMs    uint32
		hashAlgorithm         gocrypto.Hash
	}

	Option func(c *rootGenesisConf)

	UnicitySealFunc func(rootHash []byte) (*types.UnicitySeal, error)
)

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
	if c.totalValidators < 1 {
		return genesis.ErrInvalidNumberOfRootValidators
	}
	if c.consensusTimeoutMs < genesis.MinConsensusTimeout {
		return fmt.Errorf("invalid consensus timeout, must be at least %v", genesis.MinConsensusTimeout)
	}
	if c.blockRateMs < genesis.MinBlockRateMs {
		return fmt.Errorf("invalid block rate, must be at least %v", genesis.MinBlockRateMs)
	}
	// Timeout must be bigger than round min block-rate+2s
	if c.blockRateMs+genesis.MinConsensusTimeout > c.consensusTimeoutMs {
		return fmt.Errorf("invalid timeout for block rate, must be at least %d", c.blockRateMs+genesis.MinConsensusTimeout)
	}
	return nil
}

func WithTotalNodes(rootValidators uint32) Option {
	return func(c *rootGenesisConf) {
		c.totalValidators = rootValidators
	}
}

func WithBlockRate(rate uint32) Option {
	return func(c *rootGenesisConf) {
		c.blockRateMs = rate
	}
}

func WithConsensusTimeout(timeoutMs uint32) Option {
	return func(c *rootGenesisConf) {
		c.consensusTimeoutMs = timeoutMs
	}
}

// WithHashAlgorithm set custom hash algorithm (unused for now, remove?)
func WithHashAlgorithm(hashAlgorithm gocrypto.Hash) Option {
	return func(c *rootGenesisConf) {
		c.hashAlgorithm = hashAlgorithm
	}
}

func createUnicityCertificates(utData []*types.UnicityTreeData, hash gocrypto.Hash, sealFn UnicitySealFunc) ([]byte, map[types.SystemID]*types.UnicityCertificate, error) {
	// calculate unicity tree
	ut, err := unicitytree.New(hash, utData)
	if err != nil {
		return nil, nil, fmt.Errorf("unicity tree calculation failed: %w", err)
	}
	// create seal
	rootHash := ut.GetRootHash()
	seal, err := sealFn(rootHash)
	if err != nil {
		return nil, nil, fmt.Errorf("unicity seal generation failed: %w", err)
	}
	certs := make(map[types.SystemID]*types.UnicityCertificate)
	// extract certificates
	for _, d := range utData {
		utCert, err := ut.GetCertificate(d.SystemIdentifier)
		if err != nil {
			return nil, nil, fmt.Errorf("get unicity tree certificate error: %w", err)
		}
		uc := &types.UnicityCertificate{
			InputRecord: d.InputRecord,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				SystemIdentifier:         utCert.SystemIdentifier,
				HashSteps:                utCert.HashSteps,
				PartitionDescriptionHash: utCert.PartitionDescriptionHash,
			},
			UnicitySeal: seal,
		}
		certs[d.SystemIdentifier] = uc
	}
	return rootHash, certs, nil
}

func NewPartitionRecordFromNodes(nodes []*genesis.PartitionNode) ([]*genesis.PartitionRecord, error) {
	var partitionNodesMap = make(map[types.SystemID][]*genesis.PartitionNode)
	for _, n := range nodes {
		if err := n.IsValid(); err != nil {
			return nil, fmt.Errorf("partition node %s validation failed: %w", n.NodeIdentifier, err)
		}
		si := n.BlockCertificationRequest.SystemIdentifier
		partitionNodesMap[si] = append(partitionNodesMap[si], n)
	}

	var partitionRecords []*genesis.PartitionRecord
	for _, partitionNodes := range partitionNodesMap {
		pr, err := newPartitionRecord(partitionNodes)
		if err != nil {
			return nil, fmt.Errorf("partition record generation error: %w", err)
		}
		partitionRecords = append(partitionRecords, pr)
	}
	return partitionRecords, nil
}

func NewRootGenesis(
	nodeID string,
	s crypto.Signer,
	encPubKey []byte,
	partitions []*genesis.PartitionRecord,
	opts ...Option,
) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	c := &rootGenesisConf{
		peerID:                nodeID,
		signer:                s,
		encryptionPubKeyBytes: encPubKey,
		totalValidators:       1,
		blockRateMs:           genesis.DefaultBlockRateMs,
		consensusTimeoutMs:    genesis.DefaultConsensusTimeout,
		hashAlgorithm:         gocrypto.SHA256,
	}
	for _, option := range opts {
		option(c)
	}
	if err := c.isValid(); err != nil {
		return nil, nil, fmt.Errorf("consensus parameters validation failed: %w", err)
	}
	verifier, err := s.Verifier()
	if err != nil {
		return nil, nil, fmt.Errorf("verifier error, %w", err)
	}
	// make sure that there are no duplicate system id's in provided partition records
	if err = genesis.CheckPartitionSystemIdentifiersUnique(partitions); err != nil {
		return nil, nil, fmt.Errorf("partition genesis records not unique: %w", err)
	}
	// iterate over all partitions and make sure that all requests are matching and every node is represented
	ucData := make([]*types.UnicityTreeData, len(partitions))
	// remember system description records hashes and system id for verification
	sdrhs := make(map[types.SystemID][]byte, len(partitions))
	for i, partition := range partitions {
		// Check that partition is valid: required fields sent and no duplicate node, all requests with same system id
		if err = partition.IsValid(); err != nil {
			return nil, nil, fmt.Errorf("invalid partition record: %w", err)
		}
		sdrh := partition.PartitionDescription.Hash(c.hashAlgorithm)
		// if partition is valid then conversion cannot fail
		sdrhs[partition.PartitionDescription.SystemIdentifier] = sdrh
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		ucData[i] = &types.UnicityTreeData{
			SystemIdentifier:         partition.PartitionDescription.SystemIdentifier,
			InputRecord:              partition.Validators[0].BlockCertificationRequest.InputRecord,
			PartitionDescriptionHash: sdrh,
		}
	}
	// if all requests match then consensus is present
	sealFn := func(rootHash []byte) (*types.UnicitySeal, error) {
		roundMeta := &abtypes.RoundInfo{
			RoundNumber:       genesis.RootRound,
			Epoch:             0,
			Timestamp:         types.GenesisTime,
			ParentRoundNumber: 0,
			CurrentRootHash:   rootHash,
		}
		uSeal := &types.UnicitySeal{
			Version:              1,
			RootChainRoundNumber: genesis.RootRound,
			Timestamp:            types.GenesisTime,
			PreviousHash:         roundMeta.Hash(gocrypto.SHA256),
			Hash:                 rootHash,
		}
		return uSeal, uSeal.Sign(c.peerID, c.signer)
	}
	// calculate unicity tree
	rootHash, certs, err := createUnicityCertificates(ucData, c.hashAlgorithm, sealFn)
	if err != nil {
		return nil, nil, fmt.Errorf("unicity certificate generation failed: %w", err)
	}
	// create "temporary" trust base to verify self signature
	tb, err := types.NewTrustBaseGenesis(
		[]*types.NodeInfo{types.NewNodeInfo(nodeID, 1, verifier)},
		rootHash,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create trust base: %w", err)
	}
	for sysId, uc := range certs {
		// check the certificate
		// ignore error, we just put it there and if not, then verify will fail anyway
		srdh := sdrhs[sysId]
		if err = uc.Verify(tb, c.hashAlgorithm, sysId, srdh); err != nil {
			// should never happen.
			return nil, nil, fmt.Errorf("generated unicity certificate validation failed: %w", err)
		}
		certs[sysId] = uc
	}

	genesisPartitions := make([]*genesis.GenesisPartitionRecord, len(partitions))
	rootPublicKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, nil, fmt.Errorf("root public key marshal error: %w", err)
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
		certificate, f := certs[partition.PartitionDescription.SystemIdentifier]
		if !f {
			return nil, nil, fmt.Errorf("missing UnicityCertificate for partition %s", partition.PartitionDescription.SystemIdentifier)
		}
		genesisPartitions[i] = &genesis.GenesisPartitionRecord{
			Nodes:                partition.Validators,
			Certificate:          certificate,
			PartitionDescription: partition.PartitionDescription,
		}
	}
	// sort genesis partition by system id
	sort.Slice(genesisPartitions, func(i, j int) bool {
		return genesisPartitions[i].PartitionDescription.SystemIdentifier < genesisPartitions[j].PartitionDescription.SystemIdentifier
	})
	// Sign the consensus and append signature
	consensusParams := &genesis.ConsensusParams{
		TotalRootValidators: c.totalValidators,
		BlockRateMs:         c.blockRateMs,
		ConsensusTimeoutMs:  c.consensusTimeoutMs,
		HashAlgorithm:       uint32(c.hashAlgorithm),
		Signatures:          make(map[string][]byte),
	}
	if err = consensusParams.Sign(c.peerID, c.signer); err != nil {
		return nil, nil, fmt.Errorf("consensus parameter sign error: %w", err)
	}
	genesisRoot := &genesis.GenesisRootRecord{
		RootValidators: rootValidatorInfo,
		Consensus:      consensusParams,
	}
	rootGenesis := &genesis.RootGenesis{
		Root:       genesisRoot,
		Partitions: genesisPartitions,
	}
	if err = rootGenesis.IsValid(); err != nil {
		return nil, nil, fmt.Errorf("root genesis validation failed: %w", err)
	}
	partitionGenesis := partitionGenesisFromRoot(rootGenesis)
	return rootGenesis, partitionGenesis, nil
}

func partitionGenesisFromRoot(rg *genesis.RootGenesis) []*genesis.PartitionGenesis {
	partitionGenesis := make([]*genesis.PartitionGenesis, len(rg.Partitions))
	for i, partition := range rg.Partitions {
		var keys = make([]*genesis.PublicKeyInfo, len(partition.Nodes))
		for j, v := range partition.Nodes {
			keys[j] = &genesis.PublicKeyInfo{
				NodeIdentifier:      v.NodeIdentifier,
				SigningPublicKey:    v.SigningPublicKey,
				EncryptionPublicKey: v.EncryptionPublicKey,
			}
		}
		partitionGenesis[i] = &genesis.PartitionGenesis{
			PartitionDescription: partition.PartitionDescription,
			Certificate:          partition.Certificate,
			RootValidators:       rg.Root.RootValidators,
			Keys:                 keys,
			Params:               partition.Nodes[0].Params,
		}
	}
	return partitionGenesis
}

func newPartitionRecord(nodes []*genesis.PartitionNode) (*genesis.PartitionRecord, error) {
	// validate nodes
	for _, n := range nodes {
		if err := n.IsValid(); err != nil {
			return nil, fmt.Errorf("partition node %s genesis validation failed %w", n.NodeIdentifier, err)
		}
	}
	// all nodes expected to have the same PDR so we just take the first
	pr := &genesis.PartitionRecord{
		PartitionDescription: &nodes[0].PartitionDescription,
		Validators:           nodes,
	}

	// validate partition record
	if err := pr.IsValid(); err != nil {
		return nil, fmt.Errorf("genesis partition record validation failed: %w", err)
	}
	return pr, nil
}

func MergeRootGenesisFiles(rootGenesis []*genesis.RootGenesis) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	// Take the first and start appending to it from the rest
	rg, rest := rootGenesis[0], rootGenesis[1:]
	if err := rg.IsValid(); err != nil {
		return nil, nil, fmt.Errorf("invalid root genesis input: %w", err)
	}
	consensusBytes := rg.Root.Consensus.Bytes()
	nodeIds := map[string]struct{}{}
	for _, v := range rg.Root.RootValidators {
		nodeIds[v.NodeIdentifier] = struct{}{}
	}
	// Check and append
	for _, appendGen := range rest {
		if err := appendGen.IsValid(); err != nil {
			return nil, nil, fmt.Errorf("invalid root genesis input: %w", err)
		}
		// Check consensus parameters are same by comparing serialized bytes
		// Should probably write a compare method instead of comparing serialized struct
		if !bytes.Equal(consensusBytes, appendGen.Root.Consensus.Bytes()) {
			return nil, nil, errors.New("not compatible root genesis files, consensus is different")
		}
		// append consensus signatures
		for k, v := range appendGen.Root.Consensus.Signatures {
			// skip, already present
			if _, found := rg.Root.Consensus.Signatures[k]; found {
				continue
			}
			rg.Root.Consensus.Signatures[k] = v
		}
		// Take a naive approach for start: append first, validate later
		// append root info
		for _, v := range appendGen.Root.RootValidators {
			if _, found := nodeIds[v.NodeIdentifier]; found {
				continue
			}
			rg.Root.RootValidators = append(rg.Root.RootValidators, v)
			nodeIds[v.NodeIdentifier] = struct{}{}
		}
		// Make sure that they have same the number of partitions
		if len(rg.Partitions) != len(appendGen.Partitions) {
			return nil, nil, errors.New("not compatible root genesis files, different number of partitions")
		}
		// Append to UC Seal signatures, assume partitions are in the same order
		for i, rgPart := range rg.Partitions {
			rgPartSdh := rgPart.Certificate.UnicityTreeCertificate.PartitionDescriptionHash
			appendPart := appendGen.Partitions[i]
			if !bytes.Equal(rgPartSdh, appendPart.Certificate.UnicityTreeCertificate.PartitionDescriptionHash) {
				return nil, nil, errors.New("not compatible genesis files, partitions are different")
			}
			// copy partition UC Seal signatures
			for k, v := range appendPart.Certificate.UnicitySeal.Signatures {
				rgPart.Certificate.UnicitySeal.Signatures[k] = v
			}
		}
	}
	// verify result
	if err := rg.IsValid(); err != nil {
		return nil, nil, fmt.Errorf("root genesis combine failed: %w", err)
	}
	// extract new partition genesis files
	partitionGenesis := partitionGenesisFromRoot(rg)
	return rg, partitionGenesis, nil
}

func RootGenesisAddSignature(rootGenesis *genesis.RootGenesis, id string, s crypto.Signer, encPubKey []byte) (*genesis.RootGenesis, error) {
	if rootGenesis == nil {
		return nil, fmt.Errorf("error, root genesis is nil")
	}
	if err := rootGenesis.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid root genesis: %w", err)
	}
	if uint32(len(rootGenesis.Root.Consensus.Signatures)) >= rootGenesis.Root.Consensus.TotalRootValidators {
		return nil, fmt.Errorf("genesis is already signed by maximum number of root nodes")
	}
	// check already signed by node
	if _, found := rootGenesis.Root.Consensus.Signatures[id]; found {
		return nil, fmt.Errorf("genesis is already signed by node id %v", id)
	}
	if err := rootGenesis.Root.Consensus.Sign(id, s); err != nil {
		return nil, fmt.Errorf("add signature failed: %w", err)
	}
	ver, err := s.Verifier()
	if err != nil {
		return nil, fmt.Errorf("get verifier failed: %w", err)
	}
	rootPublicKey, err := ver.MarshalPublicKey()
	if err != nil {
		return nil, fmt.Errorf("marshal public key failed: %w", err)
	}
	node := &genesis.PublicKeyInfo{
		NodeIdentifier:      id,
		SigningPublicKey:    rootPublicKey,
		EncryptionPublicKey: encPubKey,
	}
	rootGenesis.Root.RootValidators = append(rootGenesis.Root.RootValidators, node)
	// Update partition records
	for _, pr := range rootGenesis.Partitions {
		if err = pr.Certificate.UnicitySeal.Sign(id, s); err != nil {
			return nil, fmt.Errorf("failed to sign partition %X seal: %w", pr.PartitionDescription.SystemIdentifier, err)
		}
	}
	// make sure it what we signed is also valid
	if err = rootGenesis.IsValid(); err != nil {
		return nil, fmt.Errorf("root genesis validation failed: %w", err)
	}
	return rootGenesis, nil
}
