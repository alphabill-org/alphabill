package partition

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	pg "github.com/alphabill-org/alphabill/partition/genesis"
	"github.com/alphabill-org/alphabill/state"
	"github.com/libp2p/go-libp2p/core/peer"
)

var ErrStateIsNil = errors.New("state is nil")
var ErrSignerIsNil = errors.New("signer is nil")
var ErrEncryptionPubKeyIsNil = errors.New("encryption public key is nil")

type (
	genesisConf struct {
		peerID                peer.ID
		hashAlgorithm         gocrypto.Hash
		signer                crypto.Signer
		encryptionPubKeyBytes []byte
		params                []byte
	}

	GenesisOption func(c *genesisConf)
)

func (c *genesisConf) isValid() error {
	if c.peerID == "" {
		return genesis.ErrNodeIdentifierIsEmpty
	}
	if c.signer == nil {
		return ErrSignerIsNil
	}
	if len(c.encryptionPubKeyBytes) == 0 {
		return ErrEncryptionPubKeyIsNil
	}

	return nil
}

func WithPeerID(peerID peer.ID) GenesisOption {
	return func(c *genesisConf) {
		c.peerID = peerID
	}
}

func WithHashAlgorithm(hashAlgorithm gocrypto.Hash) GenesisOption {
	return func(c *genesisConf) {
		c.hashAlgorithm = hashAlgorithm
	}
}

func WithSigningKey(signer crypto.Signer) GenesisOption {
	return func(c *genesisConf) {
		c.signer = signer
	}
}

func WithEncryptionPubKey(encryptionPubKey []byte) GenesisOption {
	return func(c *genesisConf) {
		c.encryptionPubKeyBytes = encryptionPubKey
	}
}

func WithParams(params []byte) GenesisOption {
	return func(c *genesisConf) {
		c.params = params
	}
}

// NewNodeGenesis creates a new genesis.PartitionNode from the given inputs.
// Must contain PeerID, signer, and public encryption key configuration.
//
// This function must be called by all partition nodes in the network.
func NewNodeGenesis(state *state.State, pdr types.PartitionDescriptionRecord, opts ...GenesisOption) (*genesis.PartitionNode, error) {
	if state == nil {
		return nil, ErrStateIsNil
	}
	c := &genesisConf{
		hashAlgorithm: gocrypto.SHA256,
	}

	for _, option := range opts {
		option(c)
	}

	if err := c.isValid(); err != nil {
		return nil, err
	}

	zeroHash := make([]byte, c.hashAlgorithm.Size())
	summaryValue, hash, err := state.CalculateRoot()
	if err != nil {
		return nil, err
	}
	if hash == nil {
		hash = zeroHash
	}
	// calculate block hash
	gIR := &types.InputRecord{
		PreviousHash: zeroHash, // extend zero hash
		Hash:         hash,
		RoundNumber:  pg.PartitionRoundNumber,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}
	// create genesis block
	gBlock := &types.Block{
		Header: &types.Header{
			SystemID:          pdr.SystemIdentifier,
			ProposerID:        "genesis",
			PreviousBlockHash: zeroHash,
		},
		Transactions: make([]*types.TransactionRecord, 0),
		UnicityCertificate: &types.UnicityCertificate{
			InputRecord: gIR,
		},
	}
	// calculate first block hash
	blockHash, err := gBlock.Hash(c.hashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("calculating genesis block hash: %w", err)
	}
	gIR.BlockHash = blockHash
	id := c.peerID.String()
	// Protocol request
	blockCertificationRequest := &certification.BlockCertificationRequest{
		Partition:       pdr.SystemIdentifier,
		NodeIdentifier:  id,
		Leader:          id,
		InputRecord:     gIR,
		RootRoundNumber: pg.RootRoundNumber,
	}
	if err := blockCertificationRequest.Sign(c.signer); err != nil {
		return nil, err
	}

	verifier, err := c.signer.Verifier()
	if err != nil {
		return nil, err
	}

	if err := blockCertificationRequest.IsValid(verifier); err != nil {
		return nil, err
	}

	signingPubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}

	// partition node
	node := &genesis.PartitionNode{
		Version:                   1,
		NodeIdentifier:            id,
		SigningPublicKey:          signingPubKey,
		EncryptionPublicKey:       c.encryptionPubKeyBytes,
		BlockCertificationRequest: blockCertificationRequest,
		Params:                    c.params,
		PartitionDescription:      pdr,
	}
	if err := node.IsValid(); err != nil {
		return nil, err
	}
	return node, nil
}
