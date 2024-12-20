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
var ErrAuthPubKeyIsNil = errors.New("authentication public key is nil")

type (
	genesisConf struct {
		peerID        peer.ID
		hashAlgorithm gocrypto.Hash
		signer        crypto.Signer
		authPubKey    []byte
		params        []byte
	}

	GenesisOption func(c *genesisConf)
)

func (c *genesisConf) isValid() error {
	if c.peerID == "" {
		return genesis.ErrNodeIDIsEmpty
	}
	if c.signer == nil {
		return ErrSignerIsNil
	}
	if len(c.authPubKey) == 0 {
		return ErrAuthPubKeyIsNil
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

func WithSignPrivKey(signer crypto.Signer) GenesisOption {
	return func(c *genesisConf) {
		c.signer = signer
	}
}

func WithAuthPubKey(authPubKey []byte) GenesisOption {
	return func(c *genesisConf) {
		c.authPubKey = authPubKey
	}
}

func WithParams(params []byte) GenesisOption {
	return func(c *genesisConf) {
		c.params = params
	}
}

// NewNodeGenesis creates a new genesis.PartitionNode from the given inputs.
// Must contain PeerID, signer, and authentication key configuration.
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

	summaryValue, hash, err := state.CalculateRoot()
	if err != nil {
		return nil, err
	}
	// calculate block hash
	gIR := &types.InputRecord{
		Version:      1,
		PreviousHash: nil,
		Hash:         hash,
		RoundNumber:  pg.PartitionRoundNumber,
		SummaryValue: util.Uint64ToBytes(summaryValue),
		Timestamp:    types.GenesisTime,
	}
	// create genesis block
	ucBytes, err := types.Cbor.Marshal(&types.UnicityCertificate{
		Version:     1,
		InputRecord: gIR,
	})
	if err != nil {
		return nil, err
	}
	gBlock := &types.Block{
		Header: &types.Header{
			Version:           1,
			PartitionID:       pdr.PartitionID,
			ProposerID:        "genesis",
			PreviousBlockHash: hash,
		},
		Transactions:       make([]*types.TransactionRecord, 0),
		UnicityCertificate: ucBytes,
	}
	// calculate first block hash
	gIR, err = gBlock.CalculateBlockHash(c.hashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("calculating genesis block hash: %w", err)
	}
	id := c.peerID.String()
	// Protocol request
	blockCertificationRequest := &certification.BlockCertificationRequest{
		PartitionID: pdr.PartitionID,
		NodeID:      id,
		InputRecord: gIR,
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

	signKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}

	// partition node
	node := &genesis.PartitionNode{
		Version:                    1,
		NodeID:                     id,
		AuthKey:                    c.authPubKey,
		SignKey:                    signKey,
		BlockCertificationRequest:  blockCertificationRequest,
		PartitionDescriptionRecord: pdr,
		Params:                     c.params,
	}
	if err := node.IsValid(); err != nil {
		return nil, err
	}
	return node, nil
}
