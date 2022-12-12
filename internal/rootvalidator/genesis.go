package rootvalidator

import (
	"bytes"
	gocrypto "crypto"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
)

const (
	ErrEncryptionPubKeyIsNil          = "encryption public key is nil"
	ErrQuorumThresholdOnlyDistributed = "quorum threshold must only be less than total nodes in root chain"

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

	UnicitySealFunc func(rootHash []byte) (*certificates.UnicitySeal, error)
)

func (c *rootGenesisConf) QuorumThreshold() *uint32 {
	if c.quorumThreshold == 0 {
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
		return errors.New(ErrEncryptionPubKeyIsNil)
	}
	if c.totalValidators > 1 && c.totalValidators < genesis.MinDistributedRootValidators {
		return errors.New(genesis.ErrInvalidNumberOfRootValidators)
	}
	if c.totalValidators < c.quorumThreshold {
		return errors.New(ErrQuorumThresholdOnlyDistributed)
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

func createUnicityCertificates(utData []*unicitytree.Data, hash gocrypto.Hash, sealFn UnicitySealFunc) (map[p.SystemIdentifier]*certificates.UnicityCertificate, error) {
	// calculate unicity tree
	ut, err := unicitytree.New(hash.New(), utData)
	if err != nil {
		return nil, err
	}
	// create seal
	rootHash := ut.GetRootHash()
	seal, err := sealFn(rootHash)
	if err != nil {
		return nil, err
	}
	certs := make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	// extract certificates
	for _, d := range utData {
		utCert, err := ut.GetCertificate(d.SystemIdentifier)
		if err != nil {
			return nil, fmt.Errorf("get unicity tree certificate error: %w", err)
		}
		uc := &certificates.UnicityCertificate{
			InputRecord: d.InputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      utCert.SystemIdentifier,
				SiblingHashes:         utCert.SiblingHashes,
				SystemDescriptionHash: utCert.SystemDescriptionHash,
			},
			UnicitySeal: seal,
		}
		certs[p.SystemIdentifier(d.SystemIdentifier)] = uc
	}
	return certs, nil
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
		blockRateMs:           900,
		consensusTimeoutMs:    0,
		quorumThreshold:       0,
		hashAlgorithm:         gocrypto.SHA256,
	}

	for _, option := range opts {
		option(c)
	}

	if err := c.isValid(); err != nil {
		return nil, nil, err
	}
	ver, err := s.Verifier()
	if err != nil {
		return nil, nil, err
	}
	trustBase := map[string]crypto.Verifier{c.peerID: ver}
	// make sure that there are no duplicate system id's in provided partition records
	if err := genesis.CheckPartitionSystemIdentifiersUnique(partitions); err != nil {
		return nil, nil, err
	}
	// iterate over all partitions and make sure that all requests are matching and every node is represented
	ucData := make([]*unicitytree.Data, len(partitions))
	// remember system description records hashes and system id for verification
	sdrhs := make(map[p.SystemIdentifier][]byte, len(partitions))
	for i, partition := range partitions {
		// Check that partition is valid: required fields sent and no duplicate node, all requests with same system id
		if err := partition.IsValid(); err != nil {
			return nil, nil, errors.Errorf("invalid partition record: %v", err)
		}
		sdrh := partition.SystemDescriptionRecord.Hash(c.hashAlgorithm)
		sdrhs[p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)] = sdrh
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		ucData[i] = &unicitytree.Data{
			SystemIdentifier:            partition.SystemDescriptionRecord.SystemIdentifier,
			InputRecord:                 partition.Validators[0].BlockCertificationRequest.InputRecord,
			SystemDescriptionRecordHash: sdrh,
		}
	}
	// if all requests match then consensus is present
	sealFn := func(rootHash []byte) (*certificates.UnicitySeal, error) {
		uSeal := &certificates.UnicitySeal{
			RootChainRoundNumber: 1,
			PreviousHash:         make([]byte, c.hashAlgorithm.Size()),
			Hash:                 rootHash,
			RoundCreationTime:    GenesisTime,
		}
		return uSeal, uSeal.Sign(c.peerID, c.signer)
	}
	// calculate unicity tree
	certs, err := createUnicityCertificates(ucData, c.hashAlgorithm, sealFn)
	if err != nil {
		return nil, nil, err
	}
	for sysId, uc := range certs {
		// check the certificate
		// ignore error, we just put it there and if not, then verify will fail anyway
		srdh, _ := sdrhs[sysId]
		if err := uc.IsValid(trustBase, c.hashAlgorithm, sysId.Bytes(), srdh); err != nil {
			// should never happen.
			return nil, nil, fmt.Errorf("error invalid genese unicity certificate: %w", err)
		}
		certs[sysId] = uc
	}

	genesisPartitions := make([]*genesis.GenesisPartitionRecord, len(partitions))
	partitionGenesis := make([]*genesis.PartitionGenesis, len(partitions))
	rootPublicKey, err := ver.MarshalPublicKey()
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
		certificate, f := certs[id]
		if !f {
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

	if err := rootGenesis.IsValid(c.peerID, ver); err != nil {
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

func NewDistributedRootGenesis(rootGenesis []*genesis.RootGenesis) (*genesis.RootGenesis, []*genesis.PartitionGenesis, error) {
	if len(rootGenesis) < genesis.MinDistributedRootValidators {
		return nil, nil, errors.Errorf("distributed root chain genesis requires at least %v root validator genesis files", genesis.MinDistributedRootValidators)
	}
	// Take the first and start appending to it from the rest
	rg, rest := rootGenesis[0], rootGenesis[1:]
	consensusBytes := rg.Root.Consensus.Bytes()
	// Check and append
	for _, appendGen := range rest {
		// Check consensus parameters are same by comparing serialized bytes
		// Should probably write a compare method instead of comparing serialized struct
		if bytes.Compare(consensusBytes, appendGen.Root.Consensus.Bytes()) != 0 {
			return nil, nil, errors.New("not compatible root genesis files, consensus is different")
		}
		// Take a naive approach for start: append first, validate later
		// append root info
		rg.Root.RootValidators = append(rg.Root.RootValidators, appendGen.Root.RootValidators...)
		// append consensus signatures
		for k, v := range appendGen.Root.Consensus.Signatures {
			rg.Root.Consensus.Signatures[k] = v
		}
		// Make sure that they have same partitions and merge UC Seal signature
		if len(rg.Partitions) != len(appendGen.Partitions) {
			return nil, nil, errors.New("not compatible root genesis files, different number of partitions")
		}
		// Append UC Seal signatures
		for _, rgPart := range rg.Partitions {
			rgPartSdh := rgPart.Certificate.UnicityTreeCertificate.SystemDescriptionHash
			for _, appendPart := range appendGen.Partitions {
				if bytes.Compare(rgPartSdh, appendPart.Certificate.UnicityTreeCertificate.SystemDescriptionHash) == 0 {
					// copy partition UC Seal signatures
					for k, v := range appendPart.Certificate.UnicitySeal.Signatures {
						rgPart.Certificate.UnicitySeal.Signatures[k] = v
					}
					// There can be only one partition with same system description hash
					break
				}
			}
		}
	}
	// extract new partition genesis files
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
			SystemDescriptionRecord: partition.SystemDescriptionRecord,
			Certificate:             partition.Certificate,
			RootValidators:          rg.Root.RootValidators,
			Keys:                    keys,
			Params:                  partition.Nodes[0].Params,
		}
	}
	// verify result
	err := rg.Verify()
	if err != nil {
		return nil, nil, errors.Wrap(err, "root genesis combine failed")
	}
	return rg, partitionGenesis, nil
}
